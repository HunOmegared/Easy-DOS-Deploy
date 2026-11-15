package main

import (
	"fmt"
	"io"
	"strings"
)

// PartInfo describes one partition (or a whole partitionless disk) found
// on a disk that is about to be overwritten.
type PartInfo struct {
	Index  int  // 1-4; 0 for a disk without partition table
	Type   byte // partition type byte
	Start  uint32
	Size   uint32 // sectors
	FS     string
	Label  string
	Modern bool // NTFS, exFAT, GPT, Linux...: surely not a DOS disk
}

var modernTypes = map[byte]string{
	0xEE: "GPT", 0x83: "Linux", 0x82: "Linux swap", 0x8E: "Linux LVM",
	0xA5: "BSD", 0xA6: "OpenBSD", 0xA9: "NetBSD", 0xAF: "macOS HFS+", 0xFD: "Linux RAID",
}

// fsOf names the file system in a boot sector.
func fsOf(bs []byte) (fs, label string, modern bool) {
	if len(bs) < 512 || bs[510] != 0x55 || bs[511] != 0xAA {
		if len(bs) >= 11 && string(bs[3:11]) == "EXFAT   " {
			return "exFAT", "", true
		}
		return "", "", false
	}
	switch {
	case string(bs[3:11]) == "NTFS    ":
		return "NTFS", "", true
	case string(bs[3:11]) == "EXFAT   ":
		return "exFAT", "", true
	case string(bs[82:90]) == "FAT32   ":
		return "FAT32", strings.TrimSpace(string(bs[71:82])), false
	case string(bs[54:59]) == "FAT12" || string(bs[54:59]) == "FAT16" || (looksLikeBPB(bs) && bs[38] != 0x29):
		fs = "FAT"
		if bs[38] == 0x29 {
			fs = strings.TrimSpace(string(bs[54:62]))
			label = strings.TrimSpace(string(bs[43:54]))
		}
		return fs, label, false
	}
	return "", "", false
}

// InspectDisk lists what is on a disk. r must allow 512-byte reads at
// sector offsets (true for raw Windows disk handles).
func InspectDisk(r io.ReaderAt) ([]PartInfo, error) {
	s0 := make([]byte, 512)
	if _, err := r.ReadAt(s0, 0); err != nil {
		return nil, err
	}
	if fs, label, modern := fsOf(s0); fs != "" && !hasMBRWithFAT(s0) && (looksLikeBPB(s0) || modern) {
		return []PartInfo{{FS: fs, Label: label, Modern: modern}}, nil
	}
	if s0[510] != 0x55 || s0[511] != 0xAA {
		return nil, nil // empty or unknown
	}
	var out []PartInfo
	for i := 0; i < 4; i++ {
		e := s0[446+16*i:]
		t := e[4]
		if t == 0 {
			continue
		}
		p := PartInfo{Index: i + 1, Type: t, Start: le32(e[8:]), Size: le32(e[12:])}
		if name, ok := modernTypes[t]; ok {
			p.FS, p.Modern = name, true
		} else if t == 0x05 || t == 0x0F || t == 0x85 {
			p.FS = T("extended")
		} else if p.Start > 0 {
			bs := make([]byte, 512)
			if _, err := r.ReadAt(bs, int64(p.Start)*512); err == nil {
				p.FS, p.Label, p.Modern = fsOf(bs)
			}
		}
		if p.FS == "" {
			p.FS = fmt.Sprintf(T("type %02Xh"), t)
		}
		out = append(out, p)
	}
	return out, nil
}

// DescribeParts is a short human-readable list; modern reports whether any
// partition is surely not from a DOS machine.
func DescribeParts(ps []PartInfo) (text string, modern bool) {
	if len(ps) == 0 {
		return T("no partitions (empty or unknown content)"), false
	}
	var lines []string
	for _, p := range ps {
		var b strings.Builder
		if p.Index > 0 {
			b.WriteString(T("partition %d: ", p.Index))
		} else {
			b.WriteString(T("whole disk, no partition table: "))
		}
		b.WriteString(p.FS)
		if p.Size > 0 {
			b.WriteString(", " + humanSize(int64(p.Size)*512))
		}
		if p.Label != "" && p.Label != "NO NAME" {
			b.WriteString(", " + T("label: %s", p.Label))
		}
		lines = append(lines, b.String())
		modern = modern || p.Modern
	}
	return strings.Join(lines, "\n"), modern
}
