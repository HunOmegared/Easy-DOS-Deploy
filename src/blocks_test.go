package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func has(lines []string, sub string) bool {
	for _, l := range lines {
		if strings.Contains(l, sub) {
			return true
		}
	}
	return false
}

func TestBlocksMSDOS(t *testing.T) {
	b := BlockSet{Memory: "umb", Mouse: true, CDROM: true, Sound: true, PnP: true}
	cfg, auto, drv := b.Build(TargetMSDOS)
	for _, want := range []string{`DEVICE=C:\DOS\HIMEM.SYS`, `EMM386.EXE NOEMS`, "DOS=HIGH,UMB", `DEVICEHIGH=C:\DRIVERS\OAKCDROM.SYS /D:MSCD001`} {
		if !has(cfg, want) {
			t.Fatalf("config misses %q: %q", want, cfg)
		}
	}
	for _, want := range []string{`LH C:\DOS\MSCDEX.EXE /D:MSCD001 /L:D`, `LH C:\DRIVERS\CTMOUSE.EXE`, "SET BLASTER=A220 I5 D1 H5 P330 T6", `UNISOUND.COM /V70`} {
		if !has(auto, want) {
			t.Fatalf("autoexec misses %q: %q", want, auto)
		}
	}
	if strings.Join(drv, ",") != "oakcdrom,ctmouse,unisound" {
		t.Fatal(drv)
	}
	// no memory manager: no LH / DEVICEHIGH
	cfg, auto, _ = BlockSet{Memory: "none", Mouse: true, CDROM: true}.Build(TargetMSDOS)
	if has(cfg, "DEVICEHIGH") || has(auto, "LH ") || has(cfg, "HIMEM") {
		t.Fatal(cfg, auto)
	}
}

func TestBlocksFreeDOS(t *testing.T) {
	b := BlockSet{Memory: "ems", Mouse: true, CDROM: true, CDDriver: "udvd2", Net: true, EC: true}
	cfg, auto, drv := b.Build(TargetFreeDOS)
	if !has(cfg, `JEMMEX.EXE X=TEST`) || has(cfg, "NOEMS") || !has(cfg, `C:\FREEDOS\BIN\UDVD2.SYS`) {
		t.Fatal(cfg)
	}
	if !has(auto, `SHSUCDX.COM /D:MSCD001`) || !has(auto, `C:\FREEDOS\BIN\CTMOUSE.EXE`) || !has(auto, `NE2000.COM 0x60 3 0x300`) || !has(auto, `C:\EC\EC.EXE`) {
		t.Fatal(auto)
	}
	if strings.Join(drv, ",") != "ne2000,mtcp,mtcpcfg,ec" {
		t.Fatal(drv)
	}
}

func TestPresetsAndTemplates(t *testing.T) {
	for _, tp := range Templates {
		p := tp.Make()
		if HasFatal(p.Validate()) {
			t.Fatalf("%s: %v", tp.ID, p.Validate())
		}
		if _, err := Generate(p); err != nil {
			t.Fatal(tp.ID, err)
		}
	}
	p := DefaultProject(TargetMSDOS)
	if len(p.Items) != 1 || p.Items[0].ID != "DOS" || len(p.Drivers) != 0 {
		t.Fatalf("%+v", p)
	}
	g := TemplateByID("games-ms").Make()
	if g.Items[g.Default].ID != "GAMES" {
		t.Fatal(g.Default)
	}
	for _, pr := range Presets {
		if ValidID(pr.ID) != nil {
			t.Fatal(pr.ID)
		}
	}
}

func TestRebuildOnTargetChange(t *testing.T) {
	p := ProjectFromPresets(TargetMSDOS, 5, "FULL", "NET")
	p.Items[1].Autoexec = append(p.Items[1].Autoexec, "ECHO hand edit")
	p.Target = TargetFreeDOS
	kept := p.RebuildBlockItems(TargetMSDOS)
	if len(kept) != 1 || kept[0] != "NET" {
		t.Fatal(kept)
	}
	if !has(p.Items[0].Autoexec, `SHSUCDX`) || has(p.Items[1].Autoexec, "SHSUCDX") || !has(p.Items[1].Autoexec, "hand edit") {
		t.Fatal(p.Items)
	}
	if !p.Items[0].BlocksMatch(TargetFreeDOS, nil) || p.Items[1].BlocksMatch(TargetFreeDOS, nil) {
		t.Fatal("match")
	}
}

func TestDriverFromDiskImage(t *testing.T) {
	img := filepath.Join(t.TempDir(), "win98boot.img")
	if _, err := CreateBootImage(BootOptions{Path: img, Kind: MediaF144}); err != nil {
		t.Fatal(err)
	}
	v, _ := OpenImage(img, true)
	v.WriteFile(`EBD\OAKCDROM.SYS`, []byte("oak"))
	v.Close()
	s := NewStore(t.TempDir())
	got, err := s.AddFile(s.Find("oakcdrom"), img)
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	b, _ := os.ReadFile(s.FilePath("oakcdrom", "OAKCDROM.SYS"))
	if string(b) != "oak" || !s.Ready(s.Find("oakcdrom")) {
		t.Fatal(string(b))
	}
	if _, err := s.AddFile(s.Find("unisound"), img); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestCustomCatalogGetsNewEntries(t *testing.T) {
	s := NewStore(t.TempDir())
	s.Catalog = []Driver{{ID: "mine", Name: "Mine", Files: []DriverFile{{Name: "A.COM", Dest: `DRIVERS\A.COM`}}}}
	if err := s.SaveCatalog(); err != nil {
		t.Fatal(err)
	}
	s2 := NewStore(s.Dir)
	if s2.Find("mine") == nil || s2.Find("oakcdrom") == nil || s2.Find("unisound") == nil {
		t.Fatal("merge")
	}
}
