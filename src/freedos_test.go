package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A FreeDOS card image made without a system source: the program's MBR, the
// FreeDOS boot sector for the FAT type, and the files from the store.
func TestFreeDOSImage(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(filepath.Join(dir, "drivers"))
	put := func(id, name, data string) {
		os.MkdirAll(s.DriverDir(id), 0755)
		os.WriteFile(s.FilePath(id, name), []byte(data), 0644)
	}
	p := TemplateByID("everyday-fd").Make()
	files, missing := FreeDOSFiles(p, s)
	if len(missing) < 4 || len(files) != 0 { // kernel, FreeCOM, Jemm, CuteMouse, SHSUCDX
		t.Fatal(missing, files)
	}
	put(fdKernelID, "KERNEL.SYS", "kernel")
	put(fdCommandID, "COMMAND.COM", "shell")
	put("jemm", "JEMMEX.EXE", "jemm")
	put("ctmouse", "CTMOUSE.EXE", "mouse")
	put("shsucdx", "SHSUCDX.COM", "cdx")
	files, missing = FreeDOSFiles(p, s)
	if len(missing) != 0 {
		t.Fatal(missing)
	}
	for _, n := range []string{"KERNEL.SYS", "COMMAND.COM", `FREEDOS\BIN\COMMAND.COM`, `FREEDOS\BIN\JEMMEX.EXE`, `FREEDOS\BIN\CTMOUSE.EXE`, `FREEDOS\BIN\SHSUCDX.COM`} {
		if files[n] == nil {
			t.Fatal("missing", n, files)
		}
	}
	// plain DOS needs only Jemm
	if f, _ := FreeDOSFiles(TemplateByID("plain-fd").Make(), s); len(f) != 4 || f[`FREEDOS\BIN\CTMOUSE.EXE`] != nil {
		t.Fatal(f)
	}

	for _, g := range []Geometry{{1013, 32, 63}, {60, 16, 63}} { // FAT16 and FAT12
		img := filepath.Join(dir, "fd.img")
		r, err := CreateBootImage(BootOptions{Path: img, Kind: MediaHDD, Geom: g, FreeDOS: files})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Warnings) != 0 {
			t.Fatal(r.Warnings)
		}
		raw, _ := os.ReadFile(img)
		if !bytes.Equal(raw[:440], append(append([]byte{}, mbrCode...), make([]byte, 440-len(mbrCode))...)) {
			t.Fatal("MBR is not the program's own")
		}
		di, err := ReadDiskInfo(img)
		if err != nil || di.DOS != DOSFree {
			t.Fatal(di, err)
		}
		v, err := OpenImage(img, false)
		if err != nil {
			t.Fatal(err)
		}
		vbr := raw[63*512 : 64*512]
		code := fdBoot16
		if v.FatType == 12 {
			code = fdBoot12
		}
		if !bytes.Equal(vbr[0x3E:0x1FE], code[0x3E:0x1FE]) || string(vbr[3:11]) != "FRDOS4.1" || vbr[0x24] != 0x80 {
			t.Fatalf("FAT%d boot sector", v.FatType)
		}
		if b, _ := v.ReadFile(`FREEDOS\BIN\CTMOUSE.EXE`); string(b) != "mouse" || !v.Exists(`FREEDOS\TEMP`) {
			t.Fatal("files")
		}
		v.Close()
		// the menu goes on top, as the image window does it
		if _, err := WriteToImage(img, p, s, true); err != nil {
			t.Fatal(err)
		}
	}
	// without the kernel it does not start
	if _, err := CreateBootImage(BootOptions{Path: filepath.Join(dir, "x.img"), Kind: MediaHDD, Geom: Geometry{100, 16, 63}, FreeDOS: map[string][]byte{"COMMAND.COM": {1}}}); err == nil {
		t.Fatal("no kernel")
	}
}

func TestGeomFloppy(t *testing.T) {
	p := filepath.Join(t.TempDir(), "geom.img")
	if _, err := MakeGeomFloppy(p, map[string][]byte{"KERNEL.SYS": []byte("k")}); err == nil {
		t.Fatal("no shell")
	}
	if _, err := MakeGeomFloppy(p, map[string][]byte{"KERNEL.SYS": []byte("k"), "COMMAND.COM": []byte("c")}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if len(raw) != 1474560 || !bytes.Equal(raw[0x3E:0x1FE], fdBoot12[0x3E:0x1FE]) || raw[0x24] != 0 {
		t.Fatal("boot sector")
	}
	v, err := OpenImage(p, false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if g, _ := v.ReadFile("GEOM.COM"); !bytes.Equal(g, geomCOM) || !v.Exists("AUTOEXEC.BAT") || v.Exists("FREEDOS") {
		t.Fatal("files")
	}
	if di, _ := ReadDiskInfo(p); di.DOS != DOSFree {
		t.Fatal(di.DOS)
	}
}

// The image window says what is downloaded and what is browsed for.
func TestDriverNeeds(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "drivers"))
	p := TemplateByID("everyday-fd").Make()
	dl, br := DriverNeeds(p, s, true)
	want := "KERNEL.SYS COMMAND.COM JEMMEX.EXE CTMOUSE.EXE SHSUCDX.COM"
	if got := strings.Join(dl, " "); got != want || strings.Join(br, " ") != "OAKCDROM.SYS" {
		t.Fatalf("%q / %q", got, br)
	}
	if dl, br := DriverNeeds(p, s, false); len(dl) != 0 || len(br) != 1 {
		t.Fatal(dl, br)
	}
}

// An MS-DOS COMMAND.COM must never stand in for FreeCOM (it came in from an
// MS-DOS card backup once): it is refused, and one already there is set
// aside and counts as missing.
func TestFreeCOMCheck(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "drivers"))
	d := s.Find(fdCommandID)
	msdos := []byte("MS-DOS Version 6 command interpreter")
	if err := s.putFile(d, "COMMAND.COM", msdos); err == nil {
		t.Fatal("an MS-DOS COMMAND.COM was taken")
	}
	os.MkdirAll(s.DriverDir(d.ID), 0755)
	os.WriteFile(s.FilePath(d.ID, "COMMAND.COM"), msdos, 0644)
	if s.Ready(d) {
		t.Fatal("the MS-DOS COMMAND.COM counts as FreeCOM")
	}
	if _, err := os.Stat(s.FilePath(d.ID, "COMMAND.COM") + ".BAD"); err != nil {
		t.Fatal("not set aside")
	}
	if err := s.putFile(d, "COMMAND.COM", []byte("...FreeCOM version 0.86...")); err != nil || !s.Ready(d) {
		t.Fatal(err)
	}
	// and from a card backup: an MS-DOS donor's COMMAND.COM is not taken
	donor := makeDonor(t, MediaHDD, Geometry{100, 16, 63}, DOSMS)
	s2 := NewStore(filepath.Join(t.TempDir(), "d2"))
	if _, err := s2.extractFromImage(s2.Find(fdCommandID), donor); err == nil || s2.Ready(s2.Find(fdCommandID)) {
		t.Fatal("taken from an MS-DOS backup", err)
	}
}
