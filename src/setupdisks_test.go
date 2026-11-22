package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func setupDiskImages(t *testing.T) []string {
	dir := os.Getenv("EDD_SETUP_DISKS")
	if dir == "" {
		t.Skip("EDD_SETUP_DISKS not set")
	}
	imgs, _ := filepath.Glob(filepath.Join(dir, "*.img"))
	sort.Sort(sort.Reverse(sort.StringSlice(imgs))) // any order
	return imgs
}

func TestParsePacking(t *testing.T) {
	lst := "Microsoft MS-DOS 6.22\r\n=====\r\n\r\nThe left column lists the file name as it appears.\r\n" +
		"Setup Disk #1\r\n-------------\r\nATTRIB.EXE              ATTRIB.EXE\r\nMEM.EX_                 MEM.EXE\r\n\r\n" +
		"Setup Disk #2\r\n-------------\r\nMONOUMB.38_             MONOUMB.386\r\n"
	v, disk, exp := parsePacking([]byte(lst))
	if v != "Microsoft MS-DOS 6.22" || disk["MEM.EX_"] != 1 || exp["MEM.EX_"] != "MEM.EXE" || disk["MONOUMB.38_"] != 2 || exp["MONOUMB.38_"] != "MONOUMB.386" || len(disk) != 3 {
		t.Fatal(v, disk, exp)
	}
}

func TestSetupDisksInstall(t *testing.T) {
	imgs := setupDiskImages(t)
	s, err := ReadSetupDisks(imgs)
	if err != nil {
		t.Fatal(err)
	}
	if s.Version != "Microsoft MS-DOS 6.22" || len(s.Found) != 3 || len(s.Missing) != 0 || filepath.Base(s.Boot) != "Disk1.img" {
		t.Fatalf("%q %v %v %s", s.Version, s.Found, s.Missing, s.Boot)
	}
	files, miss := s.DOSFiles(DefaultSetupOptions())
	if len(miss) > 0 {
		t.Fatal(miss)
	}
	for _, n := range []string{"HIMEM.SYS", "EMM386.EXE", "MSCDEX.EXE", "MEM.EXE", "EDIT.COM", "QBASIC.EXE", "MSAV.EXE", "MSBACKUP.EXE", "UNDELETE.EXE", "MONOUMB.386", "COMMAND.COM", "VSAFE.COM"} {
		if files[`DOS\`+n] == nil {
			t.Errorf("%s missing", n)
		}
	}
	for _, n := range []string{"SETUP.EXE", "IO.SYS", "DRVSPACE.BIN", "MWAV.EXE", "MWBACKUP.EXE", "MSTOOLS.DLL", "DISK.1", "HIMEM.SY_"} {
		if files[`DOS\`+n] != nil {
			t.Errorf("%s installed", n)
		}
	}
	if string(files[`DOS\EMM386.EXE`][:2]) != "MZ" || len(files[`DOS\EMM386.EXE`]) != 120926 {
		t.Fatal("EMM386 not expanded")
	}
	t.Logf("%d files", len(files))
	all, _ := s.DOSFiles(SetupOptions{Backup: CompBoth, AntiVirus: CompWindows, Undelete: CompBoth, DriveSpace: true})
	for _, n := range []string{"MWAV.EXE", "MWBACKUP.EXE", "MSTOOLS.DLL", "DRVSPACE.EXE", "MSAVIRUS.LST", "VSAFE.COM"} {
		if all[`DOS\`+n] == nil {
			t.Errorf("%s missing (all)", n)
		}
	}
	if all[`DOS\MSAV.EXE`] != nil {
		t.Error("MSAV with Windows only")
	}
	// without disk 3: what is lost
	s3, err := ReadSetupDisks(imgs[1:]) // Disk2, Disk1
	if err != nil {
		t.Fatal(err)
	}
	if _, miss := s3.DOSFiles(DefaultSetupOptions()); len(miss) != 1 {
		t.Fatal(miss)
	}
	if _, err := ReadSetupDisks(imgs[:2]); err == nil {
		t.Fatal("no disk 1 accepted")
	}

	// a 4 GB card with MS-DOS installed from the disks
	img := filepath.Join(t.TempDir(), "dos.img")
	r, err := CreateBootImage(BootOptions{Path: img, Kind: MediaHDD, Geom: Geometry{1000, 128, 63}, Source: s.Boot, Extra: files, NoDriveSpace: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(r.Done)
	v, err := OpenImage(img, false)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	ents, _, _ := v.readDir(v.root())
	if ents[0].Name != "IO.SYS" || ents[1].Name != "MSDOS.SYS" || v.Exists("DRVSPACE.BIN") || !v.Exists("COMMAND.COM") {
		t.Fatal("system files")
	}
	if b, err := v.ReadFile(`DOS\HIMEM.SYS`); err != nil || len(b) != len(files[`DOS\HIMEM.SYS`]) {
		t.Fatal(err)
	}
	if HardDrives(img) != 2 {
		t.Fatal(HardDrives(img))
	}
}
