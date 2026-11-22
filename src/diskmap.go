package main

import (
	"os"
)

// A partition of a disk image: a primary entry of the MBR, or a logical
// drive found through the chain of extended boot records (EBR).
type diskPart struct {
	Start, Size uint32 // LBA of the volume and its length in sectors
	Type        byte
	Active      bool
	Logical     bool
	Region      uint32 // first sector that belongs to it (its EBR for a logical drive)
}

func isExtendedType(t byte) bool { return t == 0x05 || t == 0x0F || t == 0x85 }

// diskParts lists the partitions of an image file with an MBR: the primary
// ones (without the extended container) and the logical drives, in drive
// letter order for DOS (the primary FAT partition first).
func diskParts(f *os.File) []diskPart {
	mbr := make([]byte, 512)
	if _, err := f.ReadAt(mbr, 0); err != nil || mbr[510] != 0x55 || mbr[511] != 0xAA {
		return nil
	}
	var prim, logical []diskPart
	for i := 0; i < 4; i++ {
		e := mbr[446+16*i:]
		t, start, size := e[4], le32(e[8:]), le32(e[12:])
		if t == 0 || size == 0 {
			continue
		}
		if !isExtendedType(t) {
			prim = append(prim, diskPart{Start: start, Size: size, Type: t, Active: e[0] == 0x80, Region: start})
			continue
		}
		// follow the EBR chain; a link is relative to the extended partition
		ebr, b := start, make([]byte, 512)
		for hop := 0; hop < 64; hop++ {
			if _, err := f.ReadAt(b, int64(ebr)*512); err != nil || b[510] != 0x55 || b[511] != 0xAA {
				break
			}
			if d := b[446:]; d[4] != 0 && le32(d[12:]) > 0 {
				logical = append(logical, diskPart{Start: ebr + le32(d[8:]), Size: le32(d[12:]), Type: d[4], Logical: true, Region: ebr})
			}
			l := b[462:]
			if !isExtendedType(l[4]) || le32(l[8:]) == 0 {
				break
			}
			ebr = start + le32(l[8:])
		}
	}
	return append(prim, logical...)
}

// diskMap tells which parts of a whole disk image carry no information, so
// that writing it to a card can leave them out: free clusters of every FAT
// volume and space outside all partitions.
type diskMap struct {
	regions []diskRegion
}

type diskRegion struct {
	from, to int64   // bytes
	v        *Volume // nil: not a FAT volume, always written
}

// openDiskMap reads the layout of an image; the volumes share f.
func openDiskMap(f *os.File) *diskMap {
	m := &diskMap{}
	parts := diskParts(f)
	if len(parts) == 0 {
		// a volume without a partition table
		if v, err := openVolume(f); err == nil && v.base == 0 {
			m.regions = append(m.regions, diskRegion{0, int64(v.totSecs) * int64(v.bps), v})
			return m
		}
		return nil
	}
	m.regions = append(m.regions, diskRegion{0, 512, nil}) // the MBR
	for _, p := range parts {
		r := diskRegion{from: int64(p.Region) * 512, to: int64(p.Start+p.Size) * 512}
		if isFATPartType(p.Type) {
			v := &Volume{f: f}
			if err := v.load(int64(p.Start) * 512); err == nil {
				r.v = v
			}
		}
		m.regions = append(m.regions, r)
	}
	return m
}

// Skippable reports whether bytes [off, off+n) of the image need not be
// written.
func (m *diskMap) Skippable(off, n int64) bool {
	end := off + n
	for _, r := range m.regions {
		a, b := max(off, r.from), min(end, r.to)
		if a >= b {
			continue
		}
		if r.v == nil || a < r.v.base || !r.v.Skippable(a, b-a) {
			return false
		}
	}
	return true
}

// HardDrives counts the DOS drives (FAT volumes) of an image.
func HardDrives(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	for _, p := range diskParts(f) {
		if isFATPartType(p.Type) {
			n++
		}
	}
	if n == 0 {
		if _, err := openVolume(f); err == nil {
			n = 1
		}
	}
	return n
}
