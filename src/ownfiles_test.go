package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShortNames(t *testing.T) {
	n := newShortNamer()
	for _, c := range []struct{ in, out string }{
		{"README.TXT", "README.TXT"},
		{"doom.exe", "DOOM.EXE"},
		{"Norton Commander", "NORTON~1"},
		{"Norton Commander 2", "NORTON~2"},
		{"index.html", "INDEX~1.HTM"},
		{"Árvíztűrő.txt", "ARVIZT~1.TXT"},
		{"a b.c", "AB~1.C"},
	} {
		if got, _ := n.name(c.in); got != c.out {
			t.Errorf("%s: %s, want %s", c.in, got, c.out)
		}
	}
	if d := DefaultDest(t.TempDir()); !strings.HasPrefix(d, `C:\`) || len(d) > 3+12 {
		t.Fatal(d)
	}
}

func TestCopyOwnFiles(t *testing.T) {
	src := t.TempDir()
	games := filepath.Join(src, "My Games")
	os.MkdirAll(filepath.Join(games, "Doom", "Saves"), 0755)
	os.WriteFile(filepath.Join(games, "Doom", "DOOM.EXE"), []byte("doom"), 0644)
	os.WriteFile(filepath.Join(games, "Doom", "Saves", "savegame0.dsg"), []byte("save"), 0644)
	os.WriteFile(filepath.Join(games, "readme.txt"), []byte("hello"), 0644)
	nc := filepath.Join(src, "nc.exe")
	os.WriteFile(nc, []byte("nc"), 0644)

	donor := makeDonor(t, MediaHDD, Geometry{200, 16, 63}, DOSMS)
	img := filepath.Join(t.TempDir(), "disk.img")
	if _, err := CreateBootImage(BootOptions{Path: img, Kind: MediaHDD, Geom: Geometry{1000, 128, 63}, Source: donor}); err != nil {
		t.Fatal(err)
	}
	items := []CopyItem{{games, `C:\GAMES`}, {nc, `D:\UTIL`}}
	if n, files, err := CopySize(items); err != nil || n != 4+4+5+2 || files != 4 {
		t.Fatal(n, files, err)
	}
	var steps int
	r, err := CopyOwnFiles(img, items, func(d, n int) { steps = d })
	if err != nil {
		t.Fatal(err)
	}
	if steps != 4 || !strings.Contains(strings.Join(r.Done, " "), "SAVEGA~1.DSG") {
		t.Fatal(steps, r.Done)
	}
	vols, done, err := openDrives(img)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	if b, err := vols[0].ReadFile(`GAMES\DOOM\DOOM.EXE`); err != nil || string(b) != "doom" {
		t.Fatal(err)
	}
	if b, err := vols[0].ReadFile(`GAMES\DOOM\SAVES\SAVEGA~1.DSG`); err != nil || string(b) != "save" {
		t.Fatal(err)
	}
	if b, err := vols[1].ReadFile(`UTIL\NC.EXE`); err != nil || string(b) != "nc" {
		t.Fatal(err)
	}
	// a drive the disk does not have
	if _, err := CopyOwnFiles(img, []CopyItem{{nc, `F:\`}}, nil); err == nil {
		t.Fatal("F: accepted")
	}
}

func TestRecipeRoundTrip(t *testing.T) {
	p := TemplateByID("games-ms").Make()
	p.Files = []CopyItem{{`D:\Games\Doom`, `C:\GAMES\DOOM`}}
	p.Build = &BuildSettings{Kind: MediaHDD, Geom: Geometry{1021, 128, 63}, OnlyC: true, System: "setup",
		SetupDisks: []string{`D:\dos\Disk1.img`, `D:\dos\Disk2.img`}, SetupOpt: &SetupOptions{Backup: CompDOS, DriveSpace: false},
		WriteMenu: true, Label: "DOS622", Output: `D:\out\integra.img`}
	path := filepath.Join(t.TempDir(), "x.bmp.json")
	if err := p.Save(path); err != nil {
		t.Fatal(err)
	}
	q, err := LoadProject(path)
	if err != nil {
		t.Fatal(err)
	}
	if q.Build == nil || q.Build.Geom != p.Build.Geom || !q.Build.OnlyC || q.Build.System != "setup" || len(q.Build.SetupDisks) != 2 ||
		q.Build.SetupOpt.Backup != CompDOS || q.Build.Output != p.Build.Output || len(q.Files) != 1 || q.Files[0].Dest != `C:\GAMES\DOOM` {
		t.Fatalf("%+v", q.Build)
	}
	// an older project without the recipe still loads
	old := filepath.Join(t.TempDir(), "old.bmp.json")
	os.WriteFile(old, []byte(`{"target":0,"items":[{"id":"DOS","title":"DOS"}]}`), 0644)
	if q, err := LoadProject(old); err != nil || q.Build != nil {
		t.Fatal(q, err)
	}
}
