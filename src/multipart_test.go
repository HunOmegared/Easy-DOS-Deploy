package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanDiskDrives(t *testing.T) {
	g := Geometry{1000, 255, 63} // 7.8 GB, LBA-assist
	p, err := PlanHDD(g)
	if err != nil {
		t.Fatal(err)
	}
	cyl := g.H * g.S
	ds := p.Drives()
	if len(ds) != 4 {
		t.Fatalf("%d drives: %+v", len(ds), p)
	}
	var end uint32
	for i, d := range ds {
		if d.Size > maxFAT16 || d.FatType != 16 || d.PartType != 0x06 {
			t.Fatalf("drive %d: %+v", i, d)
		}
		if i > 0 && (d.EBR%cyl != 0 || d.Start != d.EBR+g.S || d.EBR != end) {
			t.Fatalf("drive %d not on a cylinder: %+v", i, d)
		}
		end = d.Start + d.Size
		if end%cyl != 0 {
			t.Fatalf("drive %d does not end on a cylinder", i)
		}
	}
	if end != uint32(g.Sectors()) {
		t.Fatalf("not the whole disk: %d of %d", end, g.Sectors())
	}
	if p.ExtStart != ds[1].EBR || p.ExtStart+p.ExtSize != end {
		t.Fatalf("extended: %d+%d", p.ExtStart, p.ExtSize)
	}
	// one partition when it fits, as before
	if p, _ := PlanHDD(Geometry{1013, 32, 63}); len(p.Logical) != 0 {
		t.Fatal(p.Logical)
	}
	// chosen sizes: C: 500 MB, D: 1 GB, the rest in 2 GB drives (3 + 220 MB)
	p, _ = PlanDisk(g, []uint32{500 << 11, 1000 << 11})
	ds = p.Drives()
	if len(ds) != 6 || ds[0].Size < 500<<11 || ds[0].Size > 500<<11+cyl || ds[1].Size < 1000<<11 || ds[1].Size > 1000<<11+cyl {
		t.Fatalf("%+v", ds)
	}
	if ds[0].SPC != 16 || ds[1].SPC != 32 {
		t.Fatalf("clusters: %d %d", ds[0].SPC, ds[1].SPC)
	}
}

func TestMultiPartitionImage(t *testing.T) {
	src := makeDonor(t, MediaHDD, Geometry{200, 16, 63}, DOSMS)
	img := filepath.Join(t.TempDir(), "big.img")
	g := Geometry{1000, 255, 63}
	if _, err := CreateBootImage(BootOptions{Path: img, Kind: MediaHDD, Geom: g, Source: src}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(img)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	parts := diskParts(f)
	if len(parts) != 4 || !parts[0].Active || parts[0].Logical || !parts[3].Logical {
		t.Fatalf("%+v", parts)
	}
	plan, _ := PlanHDD(g)
	for i, d := range plan.Drives() {
		if parts[i].Start != d.Start || parts[i].Size != d.Size {
			t.Fatalf("drive %d: %+v, planned %+v", i, parts[i], d)
		}
		v := &Volume{f: f}
		if err := v.load(int64(d.Start) * 512); err != nil {
			t.Fatalf("drive %d: %v", i, err)
		}
		if v.FatType != 16 || v.totSecs != d.Size {
			t.Fatalf("drive %d: FAT%d %d", i, v.FatType, v.totSecs)
		}
		bs := make([]byte, 512)
		f.ReadAt(bs, int64(d.Start)*512)
		if want := d.Start - d.EBR; i > 0 && le32(bs[28:]) != want {
			t.Fatalf("drive %d hidden sectors %d", i, le32(bs[28:]))
		}
	}
	if n := HardDrives(img); n != 4 {
		t.Fatal(n)
	}
	di, err := ReadDiskInfo(img)
	if err != nil || di.Geom != g || di.Drives != 4 || di.DOS != DOSMS {
		t.Fatalf("%+v %v", di, err)
	}
	// writing to a card: the EBRs and boot sectors of D: E: F: are written,
	// their empty data areas are not
	m := openDiskMap(f)
	for _, d := range plan.Logical {
		if m.Skippable(int64(d.EBR)*512, 1<<20) {
			t.Fatal("EBR skipped")
		}
		if !m.Skippable(int64(d.Start+d.Size)*512-1<<20, 1<<20) {
			t.Fatal("empty end of a drive written")
		}
	}
	if m.Skippable(int64(plan.Start)*512, 512) {
		t.Fatal("boot sector skipped")
	}
}

func TestFixCDLetter(t *testing.T) {
	a := []string{"@ECHO OFF", ":CD", `LH C:\DOS\MSCDEX.EXE /D:MSCD001 /L:D`, `C:\DOS\MSCDEX.EXE /D:X /L:h`}
	got, from, to := fixCDLetter(a, 3)
	if from != 'D' || to != 'F' || got[2] != `LH C:\DOS\MSCDEX.EXE /D:MSCD001 /L:F` || got[3] != a[3] || a[2] == got[2] {
		t.Fatalf("%q %c %c", got, from, to)
	}
	if got, _, _ := fixCDLetter(a, 1); got[2] != a[2] {
		t.Fatal("changed with one drive")
	}
}

func TestOnlyC(t *testing.T) {
	src := makeDonor(t, MediaHDD, Geometry{200, 16, 63}, DOSMS)
	img := filepath.Join(t.TempDir(), "onlyc.img")
	if _, err := CreateBootImage(BootOptions{Path: img, Kind: MediaHDD, Geom: Geometry{1000, 255, 63}, Source: src, OnlyC: true}); err != nil {
		t.Fatal(err)
	}
	if n := HardDrives(img); n != 1 {
		t.Fatal(n)
	}
}

func TestStarterCard(t *testing.T) {
	src := makeDonor(t, MediaHDD, Geometry{200, 16, 63}, DOSMS)
	img := filepath.Join(t.TempDir(), "starter.img")
	if _, err := CreateBootImage(BootOptions{Path: img, Kind: MediaHDD, Geom: UniversalGeom, Source: src, AddGeom: true}); err != nil {
		t.Fatal(err)
	}
	v, err := OpenImage(img, false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	ents, _, _ := v.readDir(v.root())
	if ents[0].Name != "IO.SYS" || ents[0].first() != 2 {
		t.Fatal("IO.SYS is not first")
	}
	if b, err := v.ReadFile("GEOM.COM"); err != nil || len(b) != len(geomCOM) {
		t.Fatal("no GEOM.COM", err)
	}
	if b, err := v.ReadFile("MKFLOPPY.BAT"); err != nil || !strings.Contains(string(b), "FORMAT A: /S\r\n") || !strings.Contains(string(b), "COPY C:\\GEOM.COM A:\\") {
		t.Fatal("no MKFLOPPY.BAT", err)
	}
	// plain CHS: every BIOS limit
	if g := UniversalGeom; g.H > 16 || g.S > 63 || g.C > 1024 || g.Sectors()*512 > 528482304 {
		t.Fatal(g)
	}
}

func TestClassicTypes(t *testing.T) {
	src := makeDonor(t, MediaHDD, Geometry{200, 16, 63}, DOSMS)
	for _, c := range ClassicTypes {
		g := c.Image()
		p, err := PlanHDD(g)
		if err != nil || len(p.Logical) != 0 {
			t.Fatal(c, err)
		}
		img := filepath.Join(t.TempDir(), "classic.img")
		if _, err := CreateBootImage(BootOptions{Path: img, Kind: MediaHDD, Geom: g, Source: src, AddGeom: true}); err != nil {
			t.Fatal(c, err)
		}
		di, err := ReadDiskInfo(img)
		if err != nil || di.Geom != g || di.DOS != DOSMS {
			t.Fatalf("%v: %+v %v", c, di, err)
		}
	}
	if g := ClassicTypes[0].Image(); g != (Geometry{613, 4, 17}) {
		t.Fatal(g)
	}
}
