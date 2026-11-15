package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testLines(t *testing.T, name string) []string {
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return DecodeFile(b, CP852)
}

// A working MS-DOS 6.22 with a Sound Blaster 16, as MEMMAKER left it.
func TestFindMachineSB16(t *testing.T) {
	m := FindMachine(testLines(t, "sb16-config.sys"), testLines(t, "sb16-autoexec.bat"), nil)
	want := Group{
		Config: []string{`C:\SOUND\DRV\CTSB16.SYS /UNIT=0 /BLASTER=A:220 I:5 D:1 H:5`, `C:\SOUND\DRV\CTMMSYS.SYS`},
		Autoexec: []string{`SET SOUND=C:\SOUND`, `SET BLASTER=A220 I5 D1 H5 P330 T6`, `SET MIDI=SYNTH:1 MAP:E`,
			`C:\SOUND\DIAGNOSE /S`, `C:\SOUND\SB16SET /P /Q`},
	}
	if fmt.Sprint(m.Sound) != fmt.Sprint(want) {
		t.Fatalf("sound:\n%q\n%q", m.Sound, want)
	}
	if !m.Mouse.Empty() || !m.CDROM.Empty() || !m.Keyboard.Empty() {
		t.Fatalf("%+v", m)
	}
	// SMARTDRV, SETVER, HIMEM, EMM386 are not drivers to take over
	for _, o := range m.Other {
		if strings.Contains(o, "SMARTDRV") || strings.Contains(o, "SETVER") || strings.Contains(o, "HIMEM") {
			t.Fatal(m.Other)
		}
	}
	if s := m.Summary(); s != "hang (CTSB16, CTMMSYS, DIAGNOSE, SB16SET)" {
		t.Fatal(s)
	}
}

// The Integra's card (2026-11-15): menu blocks, a Gravis path, CuteMouse,
// MSCDEX without a CD driver, KEYB and MODE commented out.
func TestFindMachineIntegra(t *testing.T) {
	cfg := []string{
		"[MENU]", "MENUITEM=NORMAL, Normal", "MENUITEM=CD, CD-ROM", "[COMMON]", `DEVICE=C:\DOS\HIMEM.SYS`,
		"[NORMAL]", `DEVICEHIGH /L:1,12048 =C:\DOS\SETVER.EXE`,
		"[CD]", `DEVICEHIGH=/L:1,9456 /S C:\CDROM\VIDE-CDD.SYS /D:MSCD0001`, "COUNTRY=036,852,C:\\DOS\\COUNTRY.SYS",
	}
	auto := []string{
		"@ECHO OFF", `SET MTCPCFG=C:\MTCP\MTC.CFG`, `SET PATH=C:\GRAVIS\ULTRASND;C:\DOS;C:\DOS\NET`,
		`REM MODE CON CODEPAGE PREPARE=((852,850) C:\DOS\EGA.CPI)`, `REM LH /L:1,18144 KEYB HU,852,C:\DOS\KEYBOARD.SYS`,
		`C:\DOS\CTMOUSE.EXE /R11`, `REM C:\DOS\MSCDEX.EXE /D:ONIDE001`, `LH /L:1,27952 C:\DOS\MSCDEX.EXE /D:MSCD0001`,
		`SET ULTRASND=240,3,3,7,7`, `SET ULTRADIR=C:\GRAVIS\ULTRASND`, `C:\GRAVIS\ULTRASND\ULTRINIT.EXE`,
		`CALL C:\SOUND\MIX.BAT`, "GOTO END", ":END",
	}
	files := map[string]string{`SOUND\MIX.BAT`: "@ECHO OFF\r\nC:\\SOUND\\MIXERSET /P\r\n"}
	read := func(p string) ([]byte, error) {
		if s, ok := files[strings.ToUpper(p)]; ok {
			return []byte(s), nil
		}
		return nil, os.ErrNotExist
	}
	m := FindMachine(cfg, auto, read)
	if fmt.Sprint(m.Mouse) != fmt.Sprint(Group{Autoexec: []string{`C:\DOS\CTMOUSE.EXE /R11`}}) {
		t.Fatalf("mouse %q", m.Mouse)
	}
	if fmt.Sprint(m.CDROM) != fmt.Sprint(Group{Config: []string{`C:\CDROM\VIDE-CDD.SYS /D:MSCD0001`}, Autoexec: []string{`C:\DOS\MSCDEX.EXE /D:MSCD0001`}}) {
		t.Fatalf("cd %q", m.CDROM)
	}
	if fmt.Sprint(m.Sound.Autoexec) != fmt.Sprint([]string{`SET ULTRASND=240,3,3,7,7`, `SET ULTRADIR=C:\GRAVIS\ULTRASND`, `C:\GRAVIS\ULTRASND\ULTRINIT.EXE`, `CALL C:\SOUND\MIX.BAT`}) {
		t.Fatalf("sound %q", m.Sound)
	}
	if fmt.Sprint(m.Keyboard) != fmt.Sprint(Group{Config: []string{`COUNTRY=036,852,C:\DOS\COUNTRY.SYS`}}) {
		t.Fatalf("keyboard %q", m.Keyboard) // REM lines are not taken
	}
	// which of the named files are not on the system
	have := map[string]bool{`DOS\CTMOUSE.EXE`: true, `CDROM\VIDE-CDD.SYS`: true, `DOS\MSCDEX.EXE`: true, `SOUND\MIX.BAT`: true}
	miss := m.MissingFiles(func(p string) bool { return have[strings.ToUpper(p)] })
	if strings.Join(miss, " ") != `C:\GRAVIS\ULTRASND\ULTRINIT.EXE` {
		t.Fatal(miss)
	}
}

// MSCDEX alone (no CD driver in CONFIG.SYS) is no CD-ROM.
func TestFindMachineHalfCD(t *testing.T) {
	m := FindMachine([]string{`DEVICE=C:\DOS\HIMEM.SYS`}, []string{`C:\DOS\MSCDEX.EXE /D:MSCD001`}, nil)
	if !m.CDROM.Empty() || len(m.Other) != 1 || !m.Empty() {
		t.Fatalf("%+v", m)
	}
}

// A games menu with the SB16 machine's own sound driver instead of the
// default BLASTER line; back to the program's own without a trace.
func TestUseMachine(t *testing.T) {
	m := FindMachine(testLines(t, "sb16-config.sys"), testLines(t, "sb16-autoexec.bat"), nil)
	m.Mouse = Group{Autoexec: []string{`C:\DOS\CTMOUSE.EXE /R11`}}
	m.Keyboard = Group{Autoexec: []string{`KEYB HU,852,C:\DOS\KEYBOARD.SYS`}}
	p := TemplateByID("games-ms").Make()
	before, _ := json.Marshal(p)
	if kept := p.UseMachine(m); len(kept) != 0 {
		t.Fatal(kept)
	}
	o, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	cfg, auto := strings.Join(o.Config, "\n"), strings.Join(o.Autoexec, "\n")
	for _, want := range []string{`DEVICEHIGH=C:\SOUND\DRV\CTSB16.SYS /UNIT=0 /BLASTER=A:220 I:5 D:1 H:5`, `DEVICEHIGH=C:\SOUND\DRV\CTMMSYS.SYS`} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("no %q in\n%s", want, cfg)
		}
	}
	for _, want := range []string{`SET MIDI=SYNTH:1 MAP:E`, "\n" + `C:\SOUND\DIAGNOSE /S`, `LH C:\DOS\CTMOUSE.EXE /R11`, `KEYB HU,852,C:\DOS\KEYBOARD.SYS`} {
		if !strings.Contains(auto, want) {
			t.Fatalf("no %q in\n%s", want, auto)
		}
	}
	if strings.Contains(auto, "SET BLASTER=A220 I5 D1 H5 P330 T6\nSET BLASTER") || strings.Contains(auto, `C:\DRIVERS\CTMOUSE.EXE`) {
		t.Fatal(auto)
	}
	for _, d := range p.Drivers {
		if d == "ctmouse" {
			t.Fatal("CuteMouse from the catalog is not needed", p.Drivers)
		}
	}
	// a clean-boot item has no sound: no machine lines there
	if strings.Contains(strings.Join(p.Items[len(p.Items)-1].Autoexec, "\n"), "DIAGNOSE") {
		t.Fatal(p.Items[len(p.Items)-1])
	}
	p.UseMachine(nil)
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatalf("not back:\n%s\n%s", before, after)
	}
	// an item edited by hand keeps its lines
	p.Items[0].Autoexec = append(p.Items[0].Autoexec, "REM mine")
	if kept := p.UseMachine(m); len(kept) != 1 || kept[0] != p.Items[0].ID {
		t.Fatal(kept)
	}
}

// The image window's way: an SB16 machine's card backup is the source, its
// drivers go into a new games menu, and everything is on the new card.
func TestMachineFromBackupToNewCard(t *testing.T) {
	donor := makeDonor(t, MediaHDD, Geometry{100, 16, 63}, DOSMS)
	v, err := OpenImage(donor, true)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := os.ReadFile("testdata/sb16-config.sys")
	auto, _ := os.ReadFile("testdata/sb16-autoexec.bat")
	v.WriteFile("CONFIG.SYS", cfg)
	v.WriteFile("AUTOEXEC.BAT", auto)
	v.WriteFile(`SOUND\DRV\CTSB16.SYS`, []byte("sb16"))
	v.WriteFile(`SOUND\DRV\CTMMSYS.SYS`, []byte("mm"))
	v.WriteFile(`SOUND\DIAGNOSE.EXE`, []byte("diag"))
	v.Close()

	m, miss, err := MachineFromImage(donor)
	if err != nil || m.Sound.Empty() {
		t.Fatal(m, err)
	}
	if strings.Join(miss, " ") != `C:\SOUND\SB16SET` {
		t.Fatal(miss) // SB16SET.EXE is not on this backup
	}
	p := TemplateByID("games-ms").Make()
	p.UseMachine(m)
	out := filepath.Join(t.TempDir(), "new.img")
	if _, err := CreateBootImage(BootOptions{Path: out, Kind: MediaHDD, Geom: Geometry{120, 16, 63}, Source: donor, CopyAll: true}); err != nil {
		t.Fatal(err)
	}
	s := NewStore(filepath.Join(t.TempDir(), "drivers"))
	if _, err := WriteToImage(out, p, s, true); err != nil {
		t.Fatal(err)
	}
	nv, _ := OpenImage(out, false)
	defer nv.Close()
	c, _ := nv.ReadFile("CONFIG.SYS")
	a, _ := nv.ReadFile("AUTOEXEC.BAT")
	if !strings.Contains(string(c), `DEVICEHIGH=C:\SOUND\DRV\CTSB16.SYS`) || !strings.Contains(string(a), `C:\SOUND\DIAGNOSE /S`) {
		t.Fatalf("%s\n%s", c, a)
	}
	if b, _ := nv.ReadFile(`SOUND\DRV\CTSB16.SYS`); string(b) != "sb16" {
		t.Fatal("the driver file was not copied")
	}
	// the old files are kept as .BAK
	if !nv.Exists("CONFIG.BAK") {
		t.Fatal("no CONFIG.BAK")
	}
}

// Drivers the system has but does not load: commented-out lines whose files
// are there, and CD / mouse driver files no line names.
func TestFindMachineNotLoaded(t *testing.T) {
	// the Integra card: MSCDEX is loaded, there is no CD driver in CONFIG.SYS,
	// but VIDE-CDD.SYS is on the disk
	m := findMachine([]string{`DEVICE=C:\DOS\HIMEM.SYS`}, []string{`LH /L:1,27952 C:\DOS\MSCDEX.EXE /D:MSCD0001`}, nil,
		[]string{`DOS\HIMEM.SYS`, `DOS\MSCDEX.EXE`, `CDROM\VIDE-CDD.SYS`, `CDROM\README.TXT`})
	if fmt.Sprint(m.CDROM) != fmt.Sprint(Group{Config: []string{`C:\CDROM\VIDE-CDD.SYS /D:MSCD0001`}, Autoexec: []string{`C:\DOS\MSCDEX.EXE /D:MSCD0001`}, Found: "file"}) {
		t.Fatalf("%q", m.CDROM)
	}
	if !m.Mouse.Empty() {
		t.Fatal(m.Mouse)
	}

	// both halves commented out, with their own device name; files there
	cfg := []string{`DEVICE=C:\DOS\HIMEM.SYS`, `REM DEVICEHIGH /L:1,9000 =C:\ATAPI\ATAPI.SYS /D:CD1 /I:0`, `REM DEVICE=C:\GONE\GONE.SYS /D:X`}
	auto := []string{`REM LH C:\DOS\MSCDEX.EXE /D:OTHER /L:E`, `REM C:\MOUSE\MOUSE.COM /Y`, `REM SET BLASTER=A220 I7 D1`}
	files := []string{`ATAPI\ATAPI.SYS`, `DOS\MSCDEX.EXE`, `MOUSE\MOUSE.COM`}
	m = findMachine(cfg, auto, nil, files)
	if fmt.Sprint(m.CDROM) != fmt.Sprint(Group{Config: []string{`C:\ATAPI\ATAPI.SYS /D:CD1 /I:0`}, Autoexec: []string{`C:\DOS\MSCDEX.EXE /D:CD1 /L:E`}, Found: "rem"}) {
		t.Fatalf("%q", m.CDROM)
	}
	if fmt.Sprint(m.Mouse) != fmt.Sprint(Group{Autoexec: []string{`C:\MOUSE\MOUSE.COM /Y`}, Found: "rem"}) {
		t.Fatalf("%q", m.Mouse)
	}
	if !m.Sound.Empty() {
		t.Fatal("a SET line alone is no sound driver", m.Sound)
	}

	// a commented-out driver whose file is gone is not used; the mouse
	// driver file alone is
	m = findMachine([]string{`REM DEVICE=C:\GONE\GONE.SYS /D:X`}, []string{`C:\DOS\MSCDEX.EXE /D:X`}, nil, []string{`DOS\MSCDEX.EXE`, `TOOLS\CTMOUSE.EXE`})
	if !m.CDROM.Empty() || fmt.Sprint(m.Mouse) != fmt.Sprint(Group{Autoexec: []string{`C:\TOOLS\CTMOUSE.EXE`}, Found: "file"}) {
		t.Fatalf("%q / %q", m.CDROM, m.Mouse)
	}
	// what the new menu then loads
	p := TemplateByID("everyday-ms").Make()
	m = findMachine([]string{`DEVICE=C:\DOS\HIMEM.SYS`}, []string{`C:\DOS\MSCDEX.EXE /D:MSCD0001`}, nil, []string{`DOS\MSCDEX.EXE`, `CDROM\VIDE-CDD.SYS`})
	p.UseMachine(m)
	o, _ := Generate(p)
	if c := strings.Join(o.Config, "\n"); !strings.Contains(c, `DEVICEHIGH=C:\CDROM\VIDE-CDD.SYS /D:MSCD0001`) || strings.Contains(c, "OAKCDROM") {
		t.Fatal(c)
	}
	for _, d := range p.Drivers {
		if d == "oakcdrom" {
			t.Fatal(p.Drivers)
		}
	}
}
