package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ---- test image formatter (independent of the code under test as far as
// practical: it lays out a volume the way FORMAT does) ----

type fmtSpec struct {
	totSecs  uint32
	spc      uint8
	rootEnts uint16
	fatType  int
	media    byte
	spt, hds uint16
}

func formatVolume(b []byte, s fmtSpec) {
	bs := b[:512]
	bs[0], bs[1], bs[2] = 0xEB, 0x3C, 0x90
	copy(bs[3:], "MSDOS5.0")
	binary.LittleEndian.PutUint16(bs[11:], 512)
	bs[13] = s.spc
	rsvd := uint16(1)
	if s.fatType == 32 {
		rsvd = 32
	}
	binary.LittleEndian.PutUint16(bs[14:], rsvd)
	bs[16] = 2
	binary.LittleEndian.PutUint16(bs[17:], s.rootEnts)
	if s.totSecs < 65536 && s.fatType != 32 {
		binary.LittleEndian.PutUint16(bs[19:], uint16(s.totSecs))
	} else {
		binary.LittleEndian.PutUint32(bs[32:], s.totSecs)
	}
	bs[21] = s.media
	binary.LittleEndian.PutUint16(bs[24:], s.spt)
	binary.LittleEndian.PutUint16(bs[26:], s.hds)
	// FAT size: iterate until stable
	rootSecs := (uint32(s.rootEnts)*32 + 511) / 512
	fatSz := uint32(1)
	for {
		data := s.totSecs - uint32(rsvd) - 2*fatSz - rootSecs
		nclus := data / uint32(s.spc)
		need := ((nclus+2)*uint32(s.fatType)/8 + 511) / 512
		if s.fatType == 12 {
			need = ((nclus+2)*3/2 + 1 + 511) / 512
		}
		if need <= fatSz {
			break
		}
		fatSz = need
	}
	if s.fatType == 32 {
		binary.LittleEndian.PutUint32(bs[36:], fatSz)
		binary.LittleEndian.PutUint32(bs[44:], 2)
		binary.LittleEndian.PutUint16(bs[48:], 1)
		fsi := b[512:1024]
		binary.LittleEndian.PutUint32(fsi, 0x41615252)
		binary.LittleEndian.PutUint32(fsi[484:], 0x61417272)
		binary.LittleEndian.PutUint32(fsi[488:], 1234)
		fsi[510], fsi[511] = 0x55, 0xAA
	} else {
		binary.LittleEndian.PutUint16(bs[22:], uint16(fatSz))
	}
	bs[510], bs[511] = 0x55, 0xAA
	for i := uint32(0); i < 2; i++ {
		f := b[(uint32(rsvd)+i*fatSz)*512:]
		switch s.fatType {
		case 12:
			f[0], f[1], f[2] = s.media, 0xFF, 0xFF
		case 16:
			f[0], f[1], f[2], f[3] = s.media, 0xFF, 0xFF, 0xFF
		case 32:
			binary.LittleEndian.PutUint32(f, 0x0FFFFF00|uint32(s.media))
			binary.LittleEndian.PutUint32(f[4:], 0x0FFFFFFF)
			binary.LittleEndian.PutUint32(f[8:], 0x0FFFFFFF) // root dir cluster
		}
	}
}

func writeImage(t *testing.T, name string, size int, mbrLBA uint32, s fmtSpec, ptype byte) string {
	t.Helper()
	b := make([]byte, size)
	if mbrLBA > 0 {
		e := b[446:]
		e[0] = 0x80
		e[4] = ptype
		binary.LittleEndian.PutUint32(e[8:], mbrLBA)
		binary.LittleEndian.PutUint32(e[12:], s.totSecs)
		b[510], b[511] = 0x55, 0xAA
		formatVolume(b[mbrLBA*512:], s)
	} else {
		formatVolume(b, s)
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func floppy(t *testing.T) string {
	return writeImage(t, "a.img", 1474560, 0, fmtSpec{2880, 1, 224, 12, 0xF0, 18, 2}, 0)
}

func hdd16(t *testing.T) string {
	// 64 MB disk, partition at LBA 63, FAT16 with 2 KB clusters
	tot := uint32(64<<20/512 - 63)
	return writeImage(t, "c.img", 64<<20, 63, fmtSpec{tot, 4, 512, 16, 0xF8, 63, 16}, 0x06)
}

func fat32(t *testing.T) string {
	// 300 MB superfloppy-style FAT32 with 512-byte clusters is too many
	// clusters for a test; use 4 KB clusters (8 sectors) => ~76k clusters.
	tot := uint32(300 << 20 / 512)
	return writeImage(t, "d.img", 300<<20, 0, fmtSpec{tot, 8, 0, 32, 0xF8, 63, 255}, 0)
}

func pattern(n int, seed byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7) + seed
	}
	return b
}

func TestFATReadWrite(t *testing.T) {
	for _, mk := range []struct {
		name string
		f    func(*testing.T) string
		typ  int
	}{{"floppy", floppy, 12}, {"hdd16", hdd16, 16}, {"fat32", fat32, 32}} {
		t.Run(mk.name, func(t *testing.T) {
			path := mk.f(t)
			v, err := OpenImage(path, true)
			if err != nil {
				t.Fatal(err)
			}
			if v.FatType != mk.typ {
				t.Fatalf("FAT%d, want FAT%d", v.FatType, mk.typ)
			}
			free0 := v.FreeBytes()
			files := map[string][]byte{
				"CONFIG.SYS":           []byte("FILES=30\r\n"),
				`MTCP\DHCP.EXE`:        pattern(40000, 1),
				`MTCP\MTCP.CFG`:        []byte("PACKETINT 0x60\r\n"),
				`DRIVERS\NE2000.COM`:   pattern(3333, 2),
				`A\B\C\DEEP.TXT`:       pattern(1, 3),
				"EMPTY.TXT":            {},
				`DRIVERS\SUB\ODD.BIN`:  pattern(513, 4),
				`DRIVERS\SUB\ODD2.BIN`: pattern(4097, 5),
			}
			for p, d := range files {
				if err := v.WriteFile(p, d); err != nil {
					t.Fatalf("write %s: %v", p, err)
				}
			}
			// many files in one subfolder forces the folder to grow
			for i := 0; i < 150; i++ {
				p := fmt.Sprintf(`MANY\F%03d.DAT`, i)
				files[p] = pattern(100+i, byte(i))
				if err := v.WriteFile(p, files[p]); err != nil {
					t.Fatalf("write %s: %v", p, err)
				}
			}
			// overwrite with a bigger and a smaller version
			files["CONFIG.SYS"] = pattern(9000, 9)
			v.WriteFile("CONFIG.SYS", files["CONFIG.SYS"])
			files[`MTCP\DHCP.EXE`] = pattern(10, 8)
			v.WriteFile(`MTCP\DHCP.EXE`, files[`MTCP\DHCP.EXE`])
			v.Close()

			// reopen and verify everything
			v, err = OpenImage(path, false)
			if err != nil {
				t.Fatal(err)
			}
			defer v.Close()
			var total int64
			for p, d := range files {
				got, err := v.ReadFile(strings.ToLower(p))
				if err != nil {
					t.Fatalf("read %s: %v", p, err)
				}
				if !bytes.Equal(got, d) {
					t.Fatalf("%s differs (%d vs %d bytes)", p, len(got), len(d))
				}
				cs := int64(v.clusSize())
				total += (int64(len(d)) + cs - 1) / cs * cs
			}
			// directories: MTCP DRIVERS A B C SUB MANY(grown)
			used := free0 - v.FreeBytes()
			if used < total {
				t.Fatalf("used %d < file data %d", used, total)
			}
			// no cluster may be shared between two files: walk all chains
			seen := map[uint32]string{}
			var walk func(d dir, prefix string)
			walk = func(d dir, prefix string) {
				ents, _, err := v.readDir(d)
				if err != nil {
					t.Fatal(err)
				}
				for _, e := range ents {
					if e.Name == "." || e.Name == ".." {
						continue
					}
					cs, err := v.chain(e.first())
					if err != nil {
						t.Fatal(err)
					}
					for _, c := range cs {
						if o, dup := seen[c]; dup {
							t.Fatalf("cluster %d used by %s and %s", c, o, prefix+e.Name)
						}
						seen[c] = prefix + e.Name
					}
					if e.isDir() {
						walk(dir{e.first()}, prefix+e.Name+`\`)
					}
				}
			}
			walk(v.root(), "")
			// dir chains are also in "seen"; for FAT32 the root chain too
			if v.FatType == 32 {
				cs, _ := v.chain(v.rootClus)
				for _, c := range cs {
					seen[c] = "ROOT"
				}
			}
			allocated := 0
			for c := uint32(2); c < v.nclus+2; c++ {
				if v.get(c) != 0 {
					allocated++
					if _, ok := seen[c]; !ok {
						t.Fatalf("cluster %d allocated but not used by any file (leak)", c)
					}
				}
			}
			if allocated != len(seen) {
				t.Fatalf("allocated %d, reachable %d", allocated, len(seen))
			}
			// both FAT copies identical
			a := make([]byte, v.fatSecs*v.bps)
			bb := make([]byte, v.fatSecs*v.bps)
			v.f.ReadAt(a, v.secOff(v.rsvd))
			v.f.ReadAt(bb, v.secOff(v.rsvd+v.fatSecs))
			if !bytes.Equal(a, bb) {
				t.Fatal("FAT copies differ")
			}
			// dot entries of a subfolder
			e, _ := v.find(v.root(), "MTCP")
			ents, _, _ := v.readDir(dir{e.first()})
			if ents[0].Name != "." || ents[0].first() != e.first() || ents[1].Name != ".." || ents[1].first() != 0 {
				t.Fatalf("bad dot entries: %+v %+v", ents[0].Name, ents[1].Name)
			}
		})
	}
}

func TestFATRootFullAndBadNames(t *testing.T) {
	v, err := OpenImage(floppy(t), true)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	for i := 0; i < 224; i++ {
		if err := v.WriteFile(fmt.Sprintf("R%d.TXT", i), []byte("x")); err != nil {
			t.Fatalf("file %d: %v", i, err)
		}
	}
	free := v.FreeBytes()
	if err := v.WriteFile("ONEMORE.TXT", pattern(5000, 1)); err == nil {
		t.Fatal("root should be full")
	}
	if v.FreeBytes() != free {
		t.Fatal("failed write leaked clusters")
	}
	for _, bad := range []string{"TOOLONGNAME.TXT", "A.TOOLONG", "SP ACE.TXT", "É.TXT", `X\Y+Z\A.TXT`} {
		if _, err := To83(splitPath(bad)[len(splitPath(bad))-1]); err == nil && !strings.Contains(bad, "+") {
			t.Fatalf("%s accepted", bad)
		}
	}
	if err := v.WriteFile(`X\Y+Z\A.TXT`, nil); err == nil {
		t.Fatal("bad folder name accepted")
	}
}

func TestFATDiskFull(t *testing.T) {
	v, err := OpenImage(floppy(t), true)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if err := v.WriteFile("BIG.BIN", pattern(1400*1024, 1)); err != nil {
		t.Fatal(err)
	}
	free := v.FreeBytes()
	if err := v.WriteFile("BIG2.BIN", pattern(200*1024, 1)); err != errFull {
		t.Fatalf("want errFull, got %v", err)
	}
	if v.FreeBytes() != free {
		t.Fatal("leak after full")
	}
	// replacing the big file with a big file must still work (frees first)
	if err := v.WriteFile("BIG.BIN", pattern(1400*1024, 2)); err != nil {
		t.Fatalf("replace when full: %v", err)
	}
	got, _ := v.ReadFile("BIG.BIN")
	if !bytes.Equal(got, pattern(1400*1024, 2)) {
		t.Fatal("content")
	}
}

func TestVHD(t *testing.T) {
	p := hdd16(t)
	b, _ := os.ReadFile(p)
	foot := make([]byte, 512)
	copy(foot, "conectix")
	binary.BigEndian.PutUint32(foot[60:], 2)
	os.WriteFile(p, append(b, foot...), 0644)
	v, err := OpenImage(p, true)
	if err != nil {
		t.Fatal(err)
	}
	v.WriteFile("X.TXT", []byte("hi"))
	v.Close()
	b2, _ := os.ReadFile(p)
	if !bytes.Equal(b2[len(b2)-512:], foot) {
		t.Fatal("footer damaged")
	}
	binary.BigEndian.PutUint32(foot[60:], 3)
	os.WriteFile(p, append(b, foot...), 0644)
	if _, err := OpenImage(p, true); err == nil {
		t.Fatal("dynamic VHD accepted")
	}
}

// ---- generation ----

func TestGenerateMSDOS(t *testing.T) {
	p := TemplateByID("net-ms").Make()
	o, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	cfg := strings.Join(o.Config, "\n")
	for _, want := range []string{"[MENU]\nMENUITEM=NET, DOS + hálózat + Easy Commander\nMENUITEM=DOS, DOS\nMENUDEFAULT=NET, 5", "[COMMON]\nFILES=30", "[NET]\nDEVICE=C:\\DOS\\HIMEM.SYS", "[DOS]"} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("missing %q in\n%s", want, cfg)
		}
	}
	auto := o.Autoexec
	if auto[0] != "@ECHO OFF" || !strings.HasPrefix(auto[1], "@REM") {
		t.Fatalf("autoexec head: %v", auto[:2])
	}
	a := strings.Join(auto, "\n")
	if !strings.Contains(a, `IF "%CONFIG%"=="NET" GOTO NET`) || !strings.Contains(a, ":NET\nSET MTCPCFG") || !strings.Contains(a, ":BMEND") {
		t.Fatal(a)
	}
	f := o.Files(CPASCII)["CONFIG.SYS"]
	if !bytes.Contains(f, []byte("\r\n")) || bytes.Contains(bytes.ReplaceAll(f, []byte("\r\n"), nil), []byte("\n")) {
		t.Fatal("not CRLF")
	}
}

func TestGenerateFreeDOS(t *testing.T) {
	p := TemplateByID("net-fd").Make()
	p.Items[0].Config = []string{`DEVICE=C:\X.SYS`}
	o, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	if o.ConfigName != "FDCONFIG.SYS" || o.AutoexecName != "FDAUTO.BAT" {
		t.Fatal(o.ConfigName, o.AutoexecName)
	}
	cfg := strings.Join(o.Config, "\n")
	for _, want := range []string{"MENUDEFAULT=1,5", "MENU 1 - DOS + hálózat + Easy Commander", "MENU 2 - DOS", "REM NET\n1?DEVICE=C:\\X.SYS", "SHELL=C:\\FREEDOS"} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("missing %q in\n%s", want, cfg)
		}
	}
	if !strings.Contains(strings.Join(o.Autoexec, "\n"), `IF "%CONFIG%"=="2" GOTO DOS`) {
		t.Fatal(o.Autoexec)
	}
}

func TestValidate(t *testing.T) {
	p := TemplateByID("net-ms").Make()
	p.Items[1].ID = "NET"
	if !HasFatal(p.Validate()) {
		t.Fatal("duplicate not caught")
	}
	p.Items[1].ID = "TOOLONGID"
	if !HasFatal(p.Validate()) {
		t.Fatal("long id")
	}
	p.Items[1].ID = "OK"
	p.Timeout = 200
	if !HasFatal(p.Validate()) {
		t.Fatal("timeout")
	}
	p.Timeout = -1
	p.MenuColor = "7,99"
	if !HasFatal(p.Validate()) {
		t.Fatal("color")
	}
}

func TestEncoding(t *testing.T) {
	s := "Árvíztűrő tükörfúrógép"
	if got := string(EncodeLine(s, CPASCII)); got != "Arvizturo tukorfurogep" {
		t.Fatal(got)
	}
	b := EncodeLine(s, CP852)
	if len(b) != len([]rune(s)) {
		t.Fatal("cp852 length")
	}
	back := DecodeFile(b, CP852)
	if back[0] != s {
		t.Fatal(back)
	}
	// CP437 has no ő/ű: they fall back to ASCII
	if got := DecodeFile(EncodeLine("őű á", CP437), CP437)[0]; got != "ou á" {
		t.Fatal(got)
	}
}

// ---- import ----

func roundTrip(t *testing.T, p *Project) {
	t.Helper()
	o, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	q, notes := Import(o.Config, o.Autoexec, p.Target)
	if len(notes) > 0 {
		t.Fatalf("notes: %v", notes)
	}
	q.Target, q.Codepage, q.Drivers, q.ConfigName, q.AutoexecName = p.Target, p.Codepage, p.Drivers, p.ConfigName, p.AutoexecName
	norm := func(x *Project) {
		for _, it := range append([]*Item{&x.Start, &x.End}, ptrs(x.Items)...) {
			if len(it.Config) == 0 {
				it.Config = nil
			}
			if len(it.Autoexec) == 0 {
				it.Autoexec = nil
			}
			it.Blocks = nil
		}
	}
	pp := p.Clone()
	norm(pp)
	norm(q)
	if !reflect.DeepEqual(pp, q) {
		t.Fatalf("round trip differs\nwant %+v\n got %+v", pp, q)
	}
}

func ptrs(it []Item) []*Item {
	out := make([]*Item, len(it))
	for i := range it {
		out[i] = &it[i]
	}
	return out
}

func TestRoundTripTemplates(t *testing.T) {
	for _, tp := range Templates {
		t.Run(tp.Name, func(t *testing.T) {
			p := tp.Make()
			roundTrip(t, p)
			p.Timeout = -1
			p.MenuColor = "15,1"
			p.End.Autoexec = []string{"ECHO vege"}
			p.End.Config = append(p.End.Config, "STACKS=9,256")
			if len(p.Items) > 1 {
				p.Default = 1
			}
			roundTrip(t, p)
		})
	}
}

func TestImportHandWrittenMSDOS(t *testing.T) {
	cfg := strings.Split(`[menu]
menuitem=Windows, Windows 3.11
menuitem=Games_With_EMS, Games
menuitem=dos
menudefault=Windows,10
menucolor=7, 1
submenu=More, more stuff

[common]
DEVICE=C:\DOS\HIMEM.SYS
DOS=HIGH,UMB

[Windows]
DEVICE=C:\DOS\EMM386.EXE NOEMS
include=net

[Games_With_EMS]
DEVICE=C:\DOS\EMM386.EXE RAM

[dos]

[net]
DEVICE=C:\NET\IFSHLP.SYS

[common]
LASTDRIVE=Z`, "\n")
	auto := strings.Split(`@echo off
path c:\dos
goto %config%
:windows
c:\net\net start
win
goto end
:Games_With_EMS
c:\mouse\mouse
:retry
goto end
:dos
:end
echo bye`, "\n")
	p, notes := Import(cfg, auto, TargetMSDOS)
	if len(p.Items) != 3 {
		t.Fatalf("%+v", p.Items)
	}
	if p.Items[0].ID != "WINDOWS" || p.Items[1].ID != "GAMES_WI" || p.Items[2].ID != "DOS" {
		t.Fatalf("ids %s %s %s", p.Items[0].ID, p.Items[1].ID, p.Items[2].ID)
	}
	if !reflect.DeepEqual(p.Items[0].Config, []string{`DEVICE=C:\DOS\EMM386.EXE NOEMS`, `DEVICE=C:\NET\IFSHLP.SYS`}) {
		t.Fatalf("include: %v", p.Items[0].Config)
	}
	if p.Timeout != 10 || p.Default != 0 || p.MenuColor != "7,1" {
		t.Fatal(p.Timeout, p.Default, p.MenuColor)
	}
	if !reflect.DeepEqual(p.End.Config, []string{"LASTDRIVE=Z"}) {
		t.Fatal(p.End.Config)
	}
	if !reflect.DeepEqual(p.Items[0].Autoexec, []string{`c:\net\net start`, "win"}) {
		t.Fatalf("win auto %q", p.Items[0].Autoexec)
	}
	if !reflect.DeepEqual(p.Items[1].Autoexec, []string{`c:\mouse\mouse`, ":retry"}) {
		t.Fatalf("games auto %q", p.Items[1].Autoexec)
	}
	if !reflect.DeepEqual(p.End.Autoexec, []string{"echo bye"}) {
		t.Fatalf("end %q", p.End.Autoexec)
	}
	if !reflect.DeepEqual(p.Start.Autoexec, []string{"@echo off", `path c:\dos`}) {
		t.Fatalf("start %q", p.Start.Autoexec)
	}
	joined := strings.Join(notes, "|")
	if !strings.Contains(joined, "SUBMENU") || !strings.Contains(joined, "GAMES_WI") {
		t.Fatalf("notes %v", notes)
	}
	if HasFatal(p.Validate()) {
		t.Fatal(p.Validate())
	}
}

func TestImportFreeDOSStock(t *testing.T) {
	// The shape of a FreeDOS 1.3 FDCONFIG.SYS
	cfg := strings.Split(`!COUNTRY=001,858,C:\FREEDOS\BIN\COUNTRY.SYS
!LASTDRIVE=Z
!BUFFERS=20
!FILES=40
!MENUCOLOR=7,0

MENUDEFAULT=2,5
MENU 1 - Load FreeDOS with JEMMEX, no EMS (most UMBs), max RAM free
MENU 2 - Load FreeDOS with JEMM386 (Expanded Memory)
MENU 3 - Load FreeDOS low with some drivers (Safe Mode)
MENU 4 - Load FreeDOS without drivers (Emergency Mode)

12?DOS=HIGH
12?DOS=UMB
12?DOSDATA=UMB
1?DEVICE=C:\FREEDOS\BIN\JEMMEX.EXE NOEMS X=TEST I=TEST NOVME NOINVLPG
234?DEVICE=C:\FREEDOS\BIN\HIMEMX.EXE
2?DEVICE=C:\FREEDOS\BIN\JEMM386.EXE X=TEST I=TEST I=B000-B7FF NOVME NOINVLPG
34?DOS=HIGH
1234?SHELL=C:\FREEDOS\BIN\COMMAND.COM C:\FREEDOS\BIN /E:1024 /P=C:\FDAUTO.BAT`, "\n")
	p, notes := Import(cfg, nil, TargetFreeDOS)
	if len(p.Items) != 4 || p.Default != 1 || p.Timeout != 5 || p.MenuColor != "7,0" {
		t.Fatalf("%d %d %d %q %v", len(p.Items), p.Default, p.Timeout, p.MenuColor, notes)
	}
	if p.Items[0].Title != "Load FreeDOS with JEMMEX, no EMS (most UMBs), max RAM free" {
		t.Fatal(p.Items[0].Title)
	}
	if len(p.Items[0].Config) != 5 || len(p.Items[3].Config) != 3 {
		t.Fatalf("%q\n%q", p.Items[0].Config, p.Items[3].Config)
	}
	if len(p.Start.Config) != 4 {
		t.Fatalf("start %q", p.Start.Config)
	}
	// generating it again must keep every line for every option
	o, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(o.Config, "\n"), "4?SHELL=") {
		t.Fatal(o.Config)
	}
}

func TestDeployToFolderAndImage(t *testing.T) {
	store := NewStore(t.TempDir())
	d := store.Find("ne2000")
	src := filepath.Join(t.TempDir(), "ne2000.com")
	os.WriteFile(src, pattern(1234, 1), 0644)
	if _, err := store.AddFile(d, src); err != nil {
		t.Fatal(err)
	}
	p := TemplateByID("net-ms").Make()
	if miss := MissingDrivers(p, store); len(miss) != 2 { // mtcp, ec
		t.Fatalf("missing %v", miss)
	}
	img := hdd16(t)
	r, err := WriteToImage(img, p, store, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(r.Done, r.Warnings)
	v, _ := OpenImage(img, false)
	got, _ := v.ReadFile(`DRIVERS\NE2000.COM`)
	cfgOnImg, _ := v.ReadFile("MTCP/MTCP.CFG")
	v.Close()
	if !bytes.Equal(got, pattern(1234, 1)) || !strings.Contains(string(cfgOnImg), "PACKETINT") {
		t.Fatal("driver files")
	}
	// second write: backups appear, MTCP.CFG kept
	p.Items[1].Title = "Csak DOS 2"
	r, err = WriteToImage(img, p, store, true)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(r.Done, "|")
	if !strings.Contains(joined, "CONFIG.BAK") || !strings.Contains(joined, "nem írtam felül") || !strings.Contains(joined, "AUTOEXEC.BAT (változatlan)") {
		t.Fatal(joined)
	}
	if !strings.Contains(strings.Join(r.Warnings, "|"), "IO.SYS") {
		t.Fatal("bootable warning missing")
	}
	// read it back as a project
	q, notes, err := ReadFromImage(img)
	if err != nil || len(notes) > 0 {
		t.Fatal(err, notes)
	}
	if q.Items[1].Title != "Csak DOS 2" || q.Target != TargetMSDOS {
		t.Fatal(q.Items)
	}
	// folder
	dir := t.TempDir()
	if _, err := WriteToFolder(dir, TemplateByID("net-fd").Make(), store, false); err != nil {
		t.Fatal(err)
	}
	q, notes, err = ReadFromFolder(dir)
	if err != nil || len(notes) > 0 || q.Target != TargetFreeDOS || q.AutoexecName != "FDAUTO.BAT" || len(q.Items) != 2 {
		t.Fatal(err, notes, q)
	}
}

func TestStoreZip(t *testing.T) {
	store := NewStore(t.TempDir())
	// build a zip with a nested zip like some FreeDOS packages
	inner := makeZip(t, map[string][]byte{"NE2000.COM": []byte("inner")})
	outer := makeZip(t, map[string][]byte{"SOURCE/CRYNWR/PKTD.ZIP": inner, "DOC/README": []byte("x")})
	p := filepath.Join(t.TempDir(), "crynwr.zip")
	os.WriteFile(p, outer, 0644)
	got, err := store.AddFile(store.Find("ne2000"), p)
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	b, _ := os.ReadFile(store.FilePath("ne2000", "NE2000.COM"))
	if string(b) != "inner" {
		t.Fatal(string(b))
	}
	m := makeZip(t, map[string][]byte{"BIN/DHCP.EXE": []byte("a"), "SOURCE/DHCP.EXE": []byte("b"), "BIN/PING.EXE": []byte("c")})
	os.WriteFile(p, m, 0644)
	got, _ = store.AddFile(store.Find("mtcp"), p)
	b, _ = os.ReadFile(store.FilePath("mtcp", "DHCP.EXE"))
	if string(b) != "a" || len(got) != 2 || store.Ready(store.Find("mtcp")) {
		t.Fatal(string(b), got)
	}
	res := store.Download(store.Find("ec"))
	if res.OK || !strings.Contains(res.Message, "tallózd") {
		t.Fatal(res)
	}
}
