package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnDriver(t *testing.T) {
	dir := t.TempDir()
	st := &Store{Dir: filepath.Join(dir, "drivers"), Catalog: DefaultCatalog()}
	// a single .SYS file
	sys := filepath.Join(dir, "aspicd.sys")
	os.WriteFile(sys, []byte("driver"), 0644)
	o := OwnDriver{Name: "ASPI CD", Group: "cdrom", Source: sys, Main: "aspicd.sys"}
	o.Line, o.Config = SuggestOwnLine(o)
	if o.Line != `DEVICE=C:\DRIVERS\ASPICD.SYS` || !o.Config {
		t.Fatal(o.Line, o.Config)
	}
	d, err := st.AddOwnDriver(o)
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "own-aspicd" || !st.Ready(d) || d.Files[0].Dest != `DRIVERS\ASPICD.SYS` || !strings.Contains(d.Desc, "CD-ROM") {
		t.Fatalf("%+v", d)
	}
	// a zip with several files gets its own folder
	zp := filepath.Join(dir, "sbdrv.zip")
	f, _ := os.Create(zp)
	zw := zip.NewWriter(f)
	for _, n := range []string{"SB/readme.txt", "SB/sbmix.cfg", "SB/Sbmix.exe"} {
		w, _ := zw.Create(n)
		w.Write([]byte(n))
	}
	zw.Close()
	f.Close()
	names, err := ZipDriverFiles(zp)
	if err != nil || names[0] != "SBMIX.EXE" || len(names) != 3 {
		t.Fatal(names, err)
	}
	o2 := OwnDriver{Name: "SB mixer", Group: "sound", Source: zp, Main: names[0]}
	o2.Line, o2.Config = SuggestOwnLine(o2)
	if o2.Line != `C:\DRIVERS\SBMIX\SBMIX.EXE` || o2.Config {
		t.Fatal(o2.Line)
	}
	d2, err := st.AddOwnDriver(o2)
	if err != nil || len(d2.Files) != 3 || !st.Ready(d2) || d2.Files[2].Dest != `DRIVERS\SBMIX\SBMIX.CFG` {
		t.Fatalf("%+v %v", d2, err)
	}
	// the same name again: a new ID
	if d3, err := st.AddOwnDriver(o); err != nil || d3.ID != "own-aspicd-2" {
		t.Fatal(d3, err)
	}

	// into the menu: a block item keeps the line among its blocks
	p := TemplateByID("everyday-ms").Make()
	var items []int
	for i := range p.Items {
		items = append(items, i)
	}
	p.Items = append(p.Items, Item{ID: "HAND", Title: "Hand made", Autoexec: []string{"ECHO hi"}})
	items = append(items, len(p.Items)-1)
	p.AddOwnLine(d.ID, items, true, o.Line)
	for i, it := range p.Items {
		if !containsLine(it.Config, o.Line) {
			t.Fatalf("item %d: %v", i, it.Config)
		}
		if it.Blocks != nil && (len(it.Blocks.Own) != 1 || !it.BlocksMatch(p.Target, p.Machine)) {
			t.Fatalf("item %d blocks", i)
		}
	}
	// rebuilding for FreeDOS keeps the line
	from := p.Target
	p.Target = TargetFreeDOS
	p.RebuildBlockItems(from)
	if !containsLine(p.Items[0].Config, o.Line) || !containsDriver(p.Drivers, d.ID) {
		t.Fatal(p.Items[0].Config, p.Drivers)
	}
}

func containsLine(ls []string, l string) bool {
	for _, x := range ls {
		if x == l {
			return true
		}
	}
	return false
}

func containsDriver(ids []string, id string) bool { return containsLine(ids, id) }
