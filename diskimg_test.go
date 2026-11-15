package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// makeDonor builds a "bootable" source image with recognisable boot code.
func makeDonor(t *testing.T, kind MediaKind, g Geometry, dos DOSKind) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "donor.img")
	if _, err := CreateBootImage(BootOptions{Path: p, Kind: kind, Geom: g}); err != nil {
		t.Fatal(err)
	}
	f, _ := os.OpenFile(p, os.O_RDWR, 0)
	v, err := openVolume(f)
	if err != nil {
		t.Fatal(err)
	}
	// recognisable code bytes in the MBR and boot sector
	if v.base > 0 {
		f.WriteAt(bytes.Repeat([]byte{0xAB}, 400), 0)
	}
	f.WriteAt(bytes.Repeat([]byte{0xCD}, 0x1FE-0x3E), v.base+0x3E)
	f.WriteAt([]byte("DONOROEM"), v.base+3)
	for _, n := range systemFiles(dos) {
		if n == "DRVSPACE.BIN" {
			continue
		}
		v.WriteFile(n, pattern(30000+len(n), byte(len(n))))
	}
	v.WriteFile(`DOS\FORMAT.COM`, pattern(5000, 7))
	v.WriteFile(`DOS\SUB\X.TXT`, []byte("x"))
	v.mkdirAll([]string{"TEMP"}, true)
	v.WriteFile(`CONFIG.SYS`, []byte("FILES=30\r\n"))
	v.SetAttr(`DOS\FORMAT.COM`, 0x21)
	v.Close()
	return p
}

func TestPlanIntegraCard(t *testing.T) {
	// Amstrad Integra 486, 1 GB CF card in LBA mode: MSD shows 1013/32/63
	p, err := PlanHDD(Geometry{1013, 32, 63})
	if err != nil {
		t.Fatal(err)
	}
	if p.Start != 63 || p.Size != 1013*32*63-63 || p.FatType != 16 || p.SPC != 32 || p.PartType != 0x06 {
		t.Fatalf("%+v", p)
	}
	for _, g := range []Geometry{{2030, 16, 63}, {1024, 255, 63}, {10, 0, 63}, {10, 16, 64}} {
		if _, err := PlanHDD(g); err == nil {
			t.Fatalf("%v accepted", g)
		}
	}
	if p, _ := PlanHDD(Geometry{20, 16, 63}); p.FatType != 12 || p.PartType != 1 {
		t.Fatalf("small: %+v", p)
	}
	if p, _ := PlanHDD(Geometry{60, 16, 63}); p.FatType != 16 || p.PartType != 4 {
		t.Fatalf("<32MB: %+v", p)
	}
}

func TestCHS(t *testing.T) {
	g := Geometry{1013, 32, 63}
	if b := chs(63, g); b != [3]byte{1, 1, 0} {
		t.Fatal(b)
	}
	end := chs(1013*32*63-1, g) // cylinder 1012 = 0x3F4
	if end != [3]byte{31, 63 | 0x03<<6, 0xF4} {
		t.Fatalf("% x", end)
	}
}

func TestCreateCardImageFromBackup(t *testing.T) {
	g := Geometry{1013, 32, 63}
	donor := makeDonor(t, MediaHDD, Geometry{200, 16, 63}, DOSMS) // e.g. a 100 MB backup
	out := filepath.Join(t.TempDir(), "card.img")
	r, err := CreateBootImage(BootOptions{Path: out, Kind: MediaHDD, Geom: g, Total: 2046240, Source: donor, CopyAll: true, Label: "dos622"})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(r.Done, r.Warnings)
	b, _ := os.ReadFile(out)
	if len(b) != 2046240*512 {
		t.Fatalf("size %d", len(b))
	}
	if !bytes.Equal(b[:400], bytes.Repeat([]byte{0xAB}, 400)) {
		t.Fatal("MBR code not copied")
	}
	e := b[446:462]
	if e[0] != 0x80 || e[4] != 0x06 || le32(e[8:]) != 63 || le32(e[12:]) != 1013*2016-63 || b[510] != 0x55 {
		t.Fatalf("partition % x", e)
	}
	bs := b[63*512 : 64*512]
	if string(bs[3:11]) != "DONOROEM" || bs[0x3E] != 0xCD || bs[0x1FD] != 0xCD {
		t.Fatal("boot code not copied")
	}
	if le16(bs[24:]) != 63 || le16(bs[26:]) != 32 || le32(bs[28:]) != 63 || bs[36] != 0x80 || bs[13] != 32 || string(bs[43:54]) != "DOS622     " {
		t.Fatalf("BPB % x", bs[11:62])
	}
	v, err := OpenImage(out, false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if v.FatType != 16 {
		t.Fatal(v.FatType)
	}
	ents, _, _ := v.readDir(v.root())
	if ents[0].Name != "IO.SYS" || ents[1].Name != "MSDOS.SYS" || ents[0].attr() != 0x07 || ents[0].first() != 2 {
		t.Fatalf("root order: %s %s attr %x first %d", ents[0].Name, ents[1].Name, ents[0].attr(), ents[0].first())
	}
	cs, _ := v.chain(2)
	for i := 1; i < len(cs); i++ {
		if cs[i] != cs[i-1]+1 {
			t.Fatal("IO.SYS not contiguous")
		}
	}
	if l, err := v.List("TEMP"); err != nil || len(l) != 2 {
		t.Fatal("empty folder not copied", l, err)
	}
	got, err := v.ReadFile(`DOS\SUB\X.TXT`)
	if err != nil || string(got) != "x" {
		t.Fatal("copy all", err)
	}
	if e, _ := v.find(dir{mustDir(t, v, "DOS")}, "FORMAT.COM"); e.attr() != 0x21 {
		t.Fatalf("attr %x", e.attr())
	}
	// geometry can be read back, as from a card backup
	di, err := ReadDiskInfo(out)
	if err != nil || di.Geom != g || di.DOS != DOSMS || di.Total != 2046240 || di.PartStart != 63 {
		t.Fatalf("%+v %v", di, err)
	}
	// the free-area check
	if v.Skippable(0, 512) || v.Skippable(v.secOff(v.dataSec), 512) {
		t.Fatal("used area skippable")
	}
	if !v.Skippable(int64(1013*2016)*512, 512) || !v.Skippable(int64(1000*2016)*512, 1<<20) {
		t.Fatal("free area not skippable")
	}
}

func mustDir(t *testing.T, v *Volume, name string) uint32 {
	e, err := v.find(v.root(), name)
	if err != nil {
		t.Fatal(err)
	}
	return e.first()
}

func TestCreateFloppies(t *testing.T) {
	donor := makeDonor(t, MediaF144, Geometry{}, DOSFree)
	for k := MediaF144; k <= MediaF288; k++ {
		out := filepath.Join(t.TempDir(), "f.img")
		if _, err := CreateBootImage(BootOptions{Path: out, Kind: k, Source: donor}); err != nil {
			t.Fatal(k, err)
		}
		v, err := OpenImage(out, false)
		if err != nil {
			t.Fatal(k, err)
		}
		ff := floppyFormats[k]
		if v.FatType != 12 || v.totSecs != ff.sectors || v.fatSecs != uint32(ff.fatSecs) {
			t.Fatalf("%s: FAT%d %d %d", ff.name, v.FatType, v.totSecs, v.fatSecs)
		}
		// the FAT size DOS uses must be enough for the clusters
		if computeFATSecs(&bpbParams{totSecs: ff.sectors, spc: ff.spc, rootEnts: ff.rootEnts, fatType: 12}) > uint32(ff.fatSecs) {
			t.Fatalf("%s: FAT too small", ff.name)
		}
		if _, err := v.ReadFile("KERNEL.SYS"); err != nil {
			t.Fatal(err)
		}
		v.Close()
	}
	// FreeDOS FAT12 boot code must not go onto a FAT16 disk
	if _, err := CreateBootImage(BootOptions{Path: filepath.Join(t.TempDir(), "x.img"), Kind: MediaHDD, Geom: Geometry{100, 16, 63}, Source: donor}); err == nil {
		t.Fatal("FAT12 FreeDOS code accepted for FAT16")
	}
	// a floppy has no MBR code for a hard disk
	ms := makeDonor(t, MediaF144, Geometry{}, DOSMS)
	if _, err := CreateBootImage(BootOptions{Path: filepath.Join(t.TempDir(), "x.img"), Kind: MediaHDD, Geom: Geometry{100, 16, 63}, Source: ms}); err == nil {
		t.Fatal("floppy source accepted for hard disk")
	}
	// but an MS-DOS floppy is fine for a floppy
	out := filepath.Join(t.TempDir(), "m.img")
	if _, err := CreateBootImage(BootOptions{Path: out, Kind: MediaF720, Source: ms}); err != nil {
		t.Fatal(err)
	}
	di, _ := ReadDiskInfo(out)
	if di.DOS != DOSMS || di.Geom != (Geometry{80, 2, 9}) {
		t.Fatalf("%+v", di)
	}
}

func TestBlankAndMenuOnNewImage(t *testing.T) {
	out := filepath.Join(t.TempDir(), "blank.img")
	r, err := CreateBootImage(BootOptions{Path: out, Kind: MediaHDD, Geom: Geometry{120, 16, 63}})
	if err != nil || len(r.Warnings) == 0 {
		t.Fatal(err, r)
	}
	if _, err := WriteToImage(out, Templates[0].Make(), NewStore(t.TempDir()), false); err != nil {
		t.Fatal(err)
	}
	v, _ := OpenImage(out, false)
	defer v.Close()
	if !v.Exists("CONFIG.SYS") {
		t.Fatal("menu missing")
	}
}
