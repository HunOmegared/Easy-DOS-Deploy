package main

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"
)

// Geometry is the CHS layout that the BIOS of the target machine gives to
// DOS (INT 13h), for example 1013/32/63 for a 1 GB CF card in LBA mode.
type Geometry struct {
	C, H, S uint32
}

func (g Geometry) Sectors() int64 { return int64(g.C) * int64(g.H) * int64(g.S) }

// MediaKind is what CreateBootImage makes.
type MediaKind int

const (
	MediaHDD MediaKind = iota // hard disk / CF card with MBR and one partition
	MediaF144
	MediaF720
	MediaF12
	MediaF360
	MediaF288
)

type floppyFormat struct {
	name     string
	sectors  uint32
	spc      uint8
	rootEnts uint16
	media    byte
	fatSecs  uint16
	spt, hds uint16
}

// The parameters DOS FORMAT uses for the standard floppy sizes.
var floppyFormats = map[MediaKind]floppyFormat{
	MediaF144: {"1.44 MB", 2880, 1, 224, 0xF0, 9, 18, 2},
	MediaF720: {"720 KB", 1440, 2, 112, 0xF9, 3, 9, 2},
	MediaF12:  {"1.2 MB", 2400, 1, 224, 0xF9, 7, 15, 2},
	MediaF360: {"360 KB", 720, 2, 112, 0xFD, 2, 9, 2},
	MediaF288: {"2.88 MB", 5760, 2, 240, 0xF0, 9, 36, 2},
}

// MediaNames lists the kinds in the order of the MediaKind constants.
func MediaNames() []string {
	return []string{T("Hard disk / CF card"), "Floppy 1.44 MB", "Floppy 720 KB", "Floppy 1.2 MB", "Floppy 360 KB", "Floppy 2.88 MB"}
}

// BootOptions describe a new disk image.
type BootOptions struct {
	Path  string
	Kind  MediaKind
	Geom  Geometry // hard disk only
	Total int64    // hard disk: image size in sectors; 0 = Geom.Sectors()
	// Source is a bootable disk image (or a backup of a card) whose boot
	// code and system files are copied. Empty: a blank, non-bootable disk.
	Source  string
	CopyAll bool   // copy all other files of the source too
	Label   string // volume label, optional
	// Blank makes an all-zero hard disk image of the right size, for
	// partitioning and installing DOS in an emulator (DOS Setup, FDISK).
	Blank bool
	// FreeDOS, instead of a source: the files of a small FreeDOS system
	// (FreeDOSFiles); the boot code is the program's own.
	FreeDOS map[string][]byte
}

// DOS kinds found on a source disk.
type DOSKind int

const (
	DOSNone DOSKind = iota
	DOSMS           // IO.SYS + MSDOS.SYS
	DOSPC           // IBMBIO.COM + IBMDOS.COM
	DOSFree         // KERNEL.SYS
)

func (k DOSKind) String() string {
	switch k {
	case DOSMS:
		return "MS-DOS"
	case DOSPC:
		return "PC DOS"
	case DOSFree:
		return "FreeDOS"
	}
	return T("no DOS")
}

// systemFiles lists, in order, the files that make a disk bootable; the
// first two of MS-DOS/PC DOS must be the first root entries, the first one
// stored from the first data cluster on.
func systemFiles(k DOSKind) []string {
	switch k {
	case DOSMS:
		return []string{"IO.SYS", "MSDOS.SYS", "DRVSPACE.BIN", "DBLSPACE.BIN", "COMMAND.COM"}
	case DOSPC:
		return []string{"IBMBIO.COM", "IBMDOS.COM", "COMMAND.COM"}
	case DOSFree:
		return []string{"KERNEL.SYS", "COMMAND.COM"}
	}
	return nil
}

func detectDOS(v *Volume) DOSKind {
	switch {
	case v.Exists("IO.SYS") && v.Exists("MSDOS.SYS"):
		return DOSMS
	case v.Exists("IBMBIO.COM") && v.Exists("IBMDOS.COM"):
		return DOSPC
	case v.Exists("KERNEL.SYS"):
		return DOSFree
	}
	return DOSNone
}

// DiskInfo is what can be read from an existing image or card backup.
type DiskInfo struct {
	Geom        Geometry // as recorded by FDISK and FORMAT
	Total       int64    // sectors in the image
	Partitioned bool
	PartStart   uint32
	PartSize    uint32
	FatType     int
	DOS         DOSKind
	Desc        string
	Drives      int // DOS hard disk drives (C:, D: ...)
}

// imageSectors returns the usable size of an image file in sectors
// (without a VHD footer).
func imageSectors(f *os.File) (int64, error) {
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	size := st.Size()
	if size >= 1024 {
		foot := make([]byte, 8)
		if _, err := f.ReadAt(foot, size-512); err == nil && string(foot) == "conectix" {
			size -= 512
		}
	}
	return size / 512, nil
}

// ReadDiskInfo looks at an image: its geometry, partition and DOS.
func ReadDiskInfo(path string) (*DiskInfo, error) {
	v, err := OpenImage(path, false)
	if err != nil {
		return nil, err
	}
	defer v.Close()
	di := &DiskInfo{FatType: v.FatType, DOS: detectDOS(v), Desc: v.Desc}
	di.Total, _ = imageSectors(v.f)
	bs := make([]byte, 512)
	v.f.ReadAt(bs, v.base)
	di.Geom.H = le16(bs[26:])
	di.Geom.S = le16(bs[24:])
	if v.base == 0 {
		// floppy-like: cylinders from the size
		if di.Geom.H > 0 && di.Geom.S > 0 {
			di.Geom.C = uint32(int64(v.totSecs) / int64(di.Geom.H*di.Geom.S))
		}
		return di, nil
	}
	di.Partitioned = true
	mbr := make([]byte, 512)
	v.f.ReadAt(mbr, 0)
	// The partitions end at the end of a cylinder: the cylinders come from
	// the end of the last one (by LBA, which also works past cylinder 1023).
	var end uint32
	for i := 0; i < 4; i++ {
		e := mbr[446+16*i:]
		if e[4] == 0 || le32(e[12:]) == 0 {
			continue
		}
		if int64(le32(e[8:]))*512 == v.base {
			di.PartStart, di.PartSize = le32(e[8:]), le32(e[12:])
		}
		if h := uint32(e[5]) + 1; h > di.Geom.H {
			di.Geom.H = h
		}
		if x := le32(e[8:]) + le32(e[12:]); x > end {
			end = x
		}
	}
	if di.Geom.H > 0 && di.Geom.S > 0 {
		di.Geom.C = uint32((int64(end) + int64(di.Geom.H*di.Geom.S) - 1) / int64(di.Geom.H*di.Geom.S))
	}
	di.Drives = HardDrives(path)
	return di, nil
}

// PartPlan is one DOS drive of a hard disk plan.
type PartPlan struct {
	Start, Size uint32 // LBA and sectors of the volume
	FatType     int
	SPC         uint8
	PartType    byte
	EBR         uint32 // logical drive: LBA of its extended boot record
}

// HDDPlan is the partitioning of a hard disk, as DOS FDISK makes it: the
// primary partition (C:) from cylinder 0 head 1, and if the disk is bigger
// than one FAT16 partition can be, an extended partition with logical
// drives (D:, E: ...) up to the end of the last cylinder. Every drive ends
// at the end of a cylinder.
type HDDPlan struct {
	PartPlan            // C:
	Logical  []PartPlan // D:, E: ...
	ExtStart uint32     // extended partition (0: none)
	ExtSize  uint32
}

// Drives is C: and the logical drives.
func (p *HDDPlan) Drives() []PartPlan { return append([]PartPlan{p.PartPlan}, p.Logical...) }

func checkGeometry(g Geometry) error {
	if g.H == 256 {
		return fmt.Errorf("%s", T("256 heads: MS-DOS 6.x and Windows 9x cannot use a disk that the BIOS gives like this. In BIOS setup, set the drive to type User with fewer cylinders (\"From BIOS setup...\" tells how many), then check it again with GEOM.COM."))
	}
	if g.H < 1 || g.H > 255 || g.S < 1 || g.S > 63 || g.C < 1 {
		return fmt.Errorf("%s", T("Heads must be 1-255, sectors 1-63, cylinders at least 1."))
	}
	if g.C > 1024 {
		return fmt.Errorf("%s", T("DOS can only use 1024 cylinders. Use the geometry the BIOS gives in LBA/LARGE mode (more heads, fewer cylinders)."))
	}
	return nil
}

// fatFor chooses the FAT type, cluster size and partition type for a
// volume, like FDISK and FORMAT.
func fatFor(size uint32) (fat int, spc uint8, typ byte) {
	switch {
	case size < 32680: // FAT12 below 16 MB
		return 12, 8, 0x01
	case size <= 262144:
		spc = 4
	case size <= 524288:
		spc = 8
	case size <= 1048576:
		spc = 16
	case size <= 2097152:
		spc = 32
	default:
		spc = 64
	}
	if size < 65536 {
		return 16, spc, 0x04
	}
	return 16, spc, 0x06
}

func PlanHDD(g Geometry) (*HDDPlan, error) { return PlanDisk(g, nil) }

// PlanDisk plans a disk with the given drive sizes in sectors (C: first;
// each rounded up to whole cylinders, 0 or a missing size: as big as
// possible). The space left after the listed drives is given to more
// logical drives of at most 2 GB, so none of the disk is lost.
func PlanDisk(g Geometry, sizes []uint32) (*HDDPlan, error) {
	if err := checkGeometry(g); err != nil {
		return nil, err
	}
	cyl := g.H * g.S
	if g.Sectors() <= int64(g.S)+64 {
		return nil, fmt.Errorf("%s", T("The disk is too small."))
	}
	maxCyl := uint32((maxFAT16 + int64(g.S)) / int64(cyl)) // cylinders of one FAT16 volume
	if maxCyl < 1 {
		maxCyl = 1
	}
	cylsFor := func(i int, left uint32) uint32 {
		k := maxCyl
		if i < len(sizes) && sizes[i] > 0 {
			k = (sizes[i] + g.S + cyl - 1) / cyl
			if k > maxCyl {
				k = maxCyl
			}
		}
		if k > left {
			k = left
		}
		return k
	}
	p := &HDDPlan{}
	k := cylsFor(0, g.C)
	p.Start, p.Size = g.S, k*cyl-g.S
	p.FatType, p.SPC, p.PartType = fatFor(p.Size)
	at := k
	for i := 1; at < g.C; i++ {
		k := cylsFor(i, g.C-at)
		if k*cyl-g.S < 2048 { // less than 1 MB is not worth a drive
			break
		}
		d := PartPlan{EBR: at * cyl, Start: at*cyl + g.S, Size: k*cyl - g.S}
		d.FatType, d.SPC, d.PartType = fatFor(d.Size)
		p.Logical = append(p.Logical, d)
		at += k
	}
	if n := len(p.Logical); n > 0 {
		last := p.Logical[n-1]
		p.ExtStart = p.Logical[0].EBR
		p.ExtSize = last.Start + last.Size - p.ExtStart
	}
	return p, nil
}

func (p *HDDPlan) Describe() string {
	if len(p.Logical) == 0 {
		return T("Partition: %s, FAT%d, %d KB clusters", humanSize(int64(p.Size)*512), p.FatType, int(p.SPC)/2)
	}
	var parts []string
	for i, d := range p.Drives() {
		parts = append(parts, T("%c: %s (%d KB clusters)", 'C'+i, humanSize(int64(d.Size)*512), int(d.SPC)/2))
	}
	return T("Drives: %s", strings.Join(parts, ", "))
}

// partEntry fills a 16 byte partition table entry.
func partEntry(e []byte, active bool, typ byte, start, size, rel uint32, g Geometry) {
	if active {
		e[0] = 0x80
	}
	st, en := chs(start, g), chs(start+size-1, g)
	copy(e[1:4], st[:])
	e[4] = typ
	copy(e[5:8], en[:])
	binary.LittleEndian.PutUint32(e[8:], start-rel)
	binary.LittleEndian.PutUint32(e[12:], size)
}

// chs encodes an LBA as the 3 CHS bytes of a partition entry.
func chs(lba uint32, g Geometry) [3]byte {
	c := lba / (g.H * g.S)
	h := (lba / g.S) % g.H
	s := lba%g.S + 1
	if c > 1023 {
		c, h, s = 1023, g.H-1, g.S
	}
	return [3]byte{byte(h), byte(s) | byte(c>>8)<<6, byte(c)}
}

type bpbParams struct {
	totSecs  uint32
	spc      uint8
	rootEnts uint16
	media    byte
	fatSecs  uint32 // 0 = compute
	spt, hds uint16
	hidden   uint32
	drive    byte
	fatType  int
	label    string
}

func computeFATSecs(p *bpbParams) uint32 {
	rootSecs := (uint32(p.rootEnts)*32 + 511) / 512
	fat := uint32(1)
	for i := 0; i < 64; i++ {
		data := p.totSecs - 1 - 2*fat - rootSecs
		n := data/uint32(p.spc) + 2
		var need uint32
		if p.fatType == 12 {
			need = (n*3/2 + 1 + 511) / 512
		} else {
			need = (n*2 + 511) / 512
		}
		if need <= fat {
			break
		}
		fat = need
	}
	return fat
}

// minimal boot code for disks without a source: INT 18h ("no boot device")
var noBootCode = []byte{0xCD, 0x18, 0xF4, 0xEB, 0xFD}

// formatFAT writes a boot sector, empty FATs and root directory at off.
func formatFAT(f *os.File, off int64, p bpbParams, bootFrom []byte) error {
	if p.fatSecs == 0 {
		p.fatSecs = computeFATSecs(&p)
	}
	bs := make([]byte, 512)
	if bootFrom != nil {
		copy(bs[0:11], bootFrom[0:11]) // jump and OEM name
		copy(bs[0x3E:0x1FE], bootFrom[0x3E:0x1FE])
	} else {
		bs[0], bs[1], bs[2] = 0xEB, 0x3C, 0x90
		copy(bs[3:11], "DOSBMENU")
		copy(bs[0x3E:], noBootCode)
	}
	binary.LittleEndian.PutUint16(bs[11:], 512)
	bs[13] = p.spc
	binary.LittleEndian.PutUint16(bs[14:], 1)
	bs[16] = 2
	binary.LittleEndian.PutUint16(bs[17:], p.rootEnts)
	if p.totSecs < 65536 {
		binary.LittleEndian.PutUint16(bs[19:], uint16(p.totSecs))
	} else {
		binary.LittleEndian.PutUint32(bs[32:], p.totSecs)
	}
	bs[21] = p.media
	binary.LittleEndian.PutUint16(bs[22:], uint16(p.fatSecs))
	binary.LittleEndian.PutUint16(bs[24:], p.spt)
	binary.LittleEndian.PutUint16(bs[26:], p.hds)
	binary.LittleEndian.PutUint32(bs[28:], p.hidden)
	bs[36] = p.drive
	bs[38] = 0x29
	binary.LittleEndian.PutUint32(bs[39:], rand.New(rand.NewSource(time.Now().UnixNano())).Uint32())
	label := strings.ToUpper(p.label)
	if label == "" {
		label = "NO NAME"
	}
	copy(bs[43:54], fmt.Sprintf("%-11.11s", label))
	copy(bs[54:62], fmt.Sprintf("FAT%-5d", p.fatType))
	bs[510], bs[511] = 0x55, 0xAA
	if _, err := f.WriteAt(bs, off); err != nil {
		return err
	}
	fat := make([]byte, p.fatSecs*512)
	fat[0] = p.media
	fat[1], fat[2] = 0xFF, 0xFF
	if p.fatType == 16 {
		fat[3] = 0xFF
	}
	for i := uint32(0); i < 2; i++ {
		if _, err := f.WriteAt(fat, off+int64(1+i*p.fatSecs)*512); err != nil {
			return err
		}
	}
	root := make([]byte, (uint32(p.rootEnts)*32+511)/512*512)
	_, err := f.WriteAt(root, off+int64(1+2*p.fatSecs)*512)
	return err
}

// source holds what is taken from the source disk.
type source struct {
	v    *Volume
	dos  DOSKind
	mbr  []byte // nil if the source has no MBR
	boot []byte
}

func openSource(path string) (*source, error) {
	v, err := OpenImage(path, false)
	if err != nil {
		return nil, fmt.Errorf(T("system source: %v"), err)
	}
	s := &source{v: v, dos: detectDOS(v)}
	if s.dos == DOSNone {
		v.Close()
		return nil, fmt.Errorf("%s", T("The system source has no DOS system files (IO.SYS/MSDOS.SYS, IBMBIO.COM/IBMDOS.COM or KERNEL.SYS)."))
	}
	if v.FatType == 32 {
		v.Close()
		return nil, fmt.Errorf("%s", T("The system source is FAT32; its boot code does not fit a FAT12/FAT16 disk."))
	}
	s.boot = make([]byte, 512)
	v.f.ReadAt(s.boot, v.base)
	if v.base > 0 {
		s.mbr = make([]byte, 512)
		v.f.ReadAt(s.mbr, 0)
	}
	return s, nil
}

// CreateBootImage makes a new disk image. The report lists what was done.
func CreateBootImage(o BootOptions) (*Report, error) {
	r := &Report{}
	var src *source
	if o.Source != "" {
		var err error
		if src, err = openSource(o.Source); err != nil {
			return nil, err
		}
		defer src.v.Close()
	}
	var bp bpbParams
	var base int64
	var total int64
	var plan *HDDPlan
	if o.Kind == MediaHDD {
		var err error
		if plan, err = PlanHDD(o.Geom); err != nil {
			return nil, err
		}
		total = o.Total
		if total < o.Geom.Sectors() {
			total = o.Geom.Sectors()
		}
		base = int64(plan.Start) * 512
		bp = bpbParams{totSecs: plan.Size, spc: plan.SPC, rootEnts: 512, media: 0xF8,
			spt: uint16(o.Geom.S), hds: uint16(o.Geom.H), hidden: plan.Start, drive: 0x80, fatType: plan.FatType, label: o.Label}
		if src != nil && src.dos == DOSFree && src.v.FatType != plan.FatType {
			return nil, fmt.Errorf(T("FreeDOS boot code is different for FAT12 and FAT16: the source is FAT%d, the new disk FAT%d. Use a FreeDOS source with FAT%d."), src.v.FatType, plan.FatType, plan.FatType)
		}

	} else {
		ff, ok := floppyFormats[o.Kind]
		if !ok {
			return nil, fmt.Errorf("unknown media")
		}
		total = int64(ff.sectors)
		bp = bpbParams{totSecs: ff.sectors, spc: ff.spc, rootEnts: ff.rootEnts, media: ff.media, fatSecs: uint32(ff.fatSecs),
			spt: ff.spt, hds: ff.hds, drive: 0x00, fatType: 12, label: o.Label}
		if src != nil && src.dos == DOSFree && src.v.FatType != 12 {
			return nil, fmt.Errorf(T("FreeDOS boot code is different for FAT12 and FAT16: the source is FAT%d, the new disk FAT%d. Use a FreeDOS source with FAT%d."), src.v.FatType, 12, 12)
		}
	}

	if o.Blank {
		if o.Kind != MediaHDD {
			return nil, fmt.Errorf("unknown media")
		}
		f, err := os.OpenFile(o.Path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			return nil, err
		}
		err = f.Truncate(total * 512)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(o.Path)
			return nil, err
		}
		r.Done = append(r.Done, T("Empty image: %d sectors (%s), geometry %d/%d/%d.", total, humanSize(total*512), o.Geom.C, o.Geom.H, o.Geom.S))
		r.Warnings = append(r.Warnings, T("The image is empty: in 86Box set the hard disk to %d cylinders, %d heads, %d sectors, then partition and install DOS with its own Setup or FDISK and FORMAT /S.", o.Geom.C, o.Geom.H, o.Geom.S))
		return r, nil
	}
	f, err := os.OpenFile(o.Path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(o.Path)
		}
	}()
	if err := f.Truncate(total * 512); err != nil {
		return nil, err
	}
	if o.Kind == MediaHDD {
		mbr := make([]byte, 512)
		if src != nil && src.mbr != nil {
			copy(mbr[:440], src.mbr[:440]) // boot code; not the disk signature or the table
		} else {
			// the program's own boot code: load the active partition's boot sector
			copy(mbr, mbrCode)
			if src != nil {
				r.Done = append(r.Done, T("MBR: the program's own boot code (the source has no MBR)."))
			}
		}
		for i := 446; i < 510; i++ {
			mbr[i] = 0
		}
		partEntry(mbr[446:], true, plan.PartType, plan.Start, plan.Size, 0, o.Geom)
		if plan.ExtStart > 0 {
			partEntry(mbr[462:], false, 0x05, plan.ExtStart, plan.ExtSize, 0, o.Geom)
		}
		mbr[510], mbr[511] = 0x55, 0xAA
		if _, err := f.WriteAt(mbr, 0); err != nil {
			return nil, err
		}
		for i, d := range plan.Logical {
			ebr := make([]byte, 512)
			partEntry(ebr[446:], false, d.PartType, d.Start, d.Size, d.EBR, o.Geom)
			if i+1 < len(plan.Logical) {
				n := plan.Logical[i+1]
				partEntry(ebr[462:], false, 0x05, n.EBR, n.Start+n.Size-n.EBR, plan.ExtStart, o.Geom)
			}
			ebr[510], ebr[511] = 0x55, 0xAA
			if _, err := f.WriteAt(ebr, int64(d.EBR)*512); err != nil {
				return nil, err
			}
		}
		r.Done = append(r.Done, T("Geometry %d/%d/%d, %s.", o.Geom.C, o.Geom.H, o.Geom.S, plan.Describe()))
	} else {
		r.Done = append(r.Done, T("Floppy %s, FAT12.", floppyFormats[o.Kind].name))
	}
	var bootFrom []byte
	if src != nil {
		bootFrom = src.boot
	} else if o.FreeDOS != nil {
		if bootFrom, err = fdBootSector(bp.fatType); err != nil {
			return nil, err
		}
	}
	if err := formatFAT(f, base, bp, bootFrom); err != nil {
		return nil, err
	}
	if plan != nil {
		for _, d := range plan.Logical {
			lp := bp
			// FORMAT counts the hidden sectors of a logical drive from its EBR
			lp.totSecs, lp.spc, lp.fatType, lp.hidden, lp.label = d.Size, d.SPC, d.FatType, d.Start-d.EBR, ""
			if err := formatFAT(f, int64(d.Start)*512, lp, bootFrom); err != nil {
				return nil, err
			}
		}
	}
	f.Close()

	v, err := OpenImage(o.Path, true)
	if err != nil {
		return nil, fmt.Errorf(T("the new image cannot be opened: %v"), err)
	}
	defer v.Close()
	if src == nil && o.FreeDOS != nil {
		if o.FreeDOS["KERNEL.SYS"] == nil || o.FreeDOS["COMMAND.COM"] == nil {
			return nil, fmt.Errorf("%s", T("FreeDOS: the kernel (KERNEL.SYS) or the command shell (COMMAND.COM) is missing."))
		}
		names := make([]string, 0, len(o.FreeDOS))
		for n := range o.FreeDOS {
			names = append(names, n)
		}
		sort.Strings(names) // KERNEL.SYS and COMMAND.COM first in the root
		sort.SliceStable(names, func(i, j int) bool { return !strings.Contains(names[i], `\`) && strings.Contains(names[j], `\`) })
		for _, n := range names {
			if err := v.WriteFile(n, o.FreeDOS[n]); err != nil {
				return nil, err
			}
		}
		if o.Kind == MediaHDD {
			if _, err := v.mkdirAll([]string{"FREEDOS", "TEMP"}, true); err != nil {
				return nil, err
			}
		}
		r.Done = append(r.Done, T("System: FreeDOS (kernel, FreeCOM and the program's own boot code), %d files.", len(names)))
	}
	if src == nil {
		if o.FreeDOS == nil {
			r.Warnings = append(r.Warnings, T("No system source was given: the disk is formatted but does not boot."))
		}
		r.Done = append(r.Done, T("Free space: %s", humanSize(v.FreeBytes())))
		if err := v.Sync(); err != nil {
			return nil, err
		}
		ok = true
		return r, nil
	}
	copied := map[string]bool{}
	for i, name := range systemFiles(src.dos) {
		data, err := src.v.ReadFile(name)
		if err != nil {
			if i < 2 || (src.dos == DOSFree && i == 0) {
				return nil, fmt.Errorf(T("cannot read %s from the source: %v"), name, err)
			}
			continue
		}
		if err := v.WriteFile(name, data); err != nil {
			return nil, err
		}
		attr := byte(0x20)
		if name != "COMMAND.COM" && src.dos != DOSFree {
			attr = 0x07 // read-only, hidden, system
		}
		v.SetAttr(name, attr)
		copied[name] = true
	}
	r.Done = append(r.Done, T("System: %s (boot code and system files from %s).", src.dos, o.Source))
	if o.CopyAll {
		n := 0
		var skipped []string
		err := src.v.Walk(func(fi FileInfo) error {
			if copied[fi.Path] || fi.Attr&0x08 != 0 {
				return nil
			}
			if fi.Dir {
				// also empty folders (TEMP and the like)
				if _, err := v.mkdirAll(splitPath(fi.Path), true); err != nil {
					return fmt.Errorf("%s: %v", fi.Path, err)
				}
				return nil
			}
			data, err := src.v.ReadFile(fi.Path)
			if err != nil {
				skipped = append(skipped, DisplayName(fi.Path)+" ("+err.Error()+")")
				return nil
			}
			if err := v.WriteFile(fi.Path, data); err != nil {
				return fmt.Errorf("%s: %v", DisplayName(fi.Path), err)
			}
			if a := fi.Attr & 0x27; a != 0x20 {
				v.SetAttr(fi.Path, a)
			}
			n++
			return nil
		})
		if err != nil {
			return nil, err
		}
		r.Done = append(r.Done, T("%d other files copied from the source.", n))
		if len(skipped) > 0 {
			r.Warnings = append(r.Warnings, T("Not copied (unreadable): %s", strings.Join(skipped, ", ")))
		}
	}
	r.Done = append(r.Done, T("Free space: %s", humanSize(v.FreeBytes())))
	if err := v.Sync(); err != nil {
		return nil, err
	}
	ok = true
	return r, nil
}
