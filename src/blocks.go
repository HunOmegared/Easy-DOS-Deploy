package main

import (
	"fmt"
	"strings"
)

// BlockSet describes a menu item by what it should load instead of by its
// lines. Build turns it into CONFIG.SYS / AUTOEXEC.BAT lines for the
// chosen DOS. The lines can still be edited by hand afterwards.
type BlockSet struct {
	Memory   string `json:"memory"`             // none, xms, umb, ems
	Mouse    bool   `json:"mouse,omitempty"`    //
	CDROM    bool   `json:"cdrom,omitempty"`    //
	CDDriver string `json:"cddriver,omitempty"` // oakcdrom, udvd2
	CDLetter string `json:"cdletter,omitempty"` // MSCDEX drive letter
	Sound    bool   `json:"sound,omitempty"`    //
	Blaster  string `json:"blaster,omitempty"`  // BLASTER variable
	PnP      bool   `json:"pnp,omitempty"`      // run UNISOUND for ISA PnP cards
	Net      bool   `json:"net,omitempty"`      //
	NetCard  string `json:"netcard,omitempty"`  // driver ID: ne2000, 3c509
	NetArgs  string `json:"netargs,omitempty"`  // packet driver parameters
	EC       bool   `json:"ec,omitempty"`       // start Easy Commander
}

// Memory choices, in the order shown.
var MemoryChoices = []string{"none", "xms", "umb", "ems"}

func MemoryNames() []string {
	return []string{
		T("Nothing (no memory manager)"),
		T("XMS only (HIMEM), DOS high"),
		T("XMS + upper memory, no EMS (386+)"),
		T("XMS + upper memory + EMS (386+)"),
	}
}

var CDDrivers = []string{"oakcdrom", "udvd2"}

const (
	defaultBlaster = "A220 I5 D1 H5 P330 T6"
	defaultCDDrive = "D"
)

type netCard struct {
	id, file, args string
}

var netCards = []netCard{
	{"ne2000", "NE2000.COM", "0x60 3 0x300"},
	{"3c509", "3C509.COM", "0x60"},
}

func NetCardNames() []string { return []string{"NE2000 (86Box, PCem)", "3Com 3C509"} }

func netCardByID(id string) netCard {
	for _, c := range netCards {
		if c.id == id {
			return c
		}
	}
	return netCards[0]
}

// Normalize fills in defaults.
func (b *BlockSet) Normalize() {
	if b.Memory == "" {
		b.Memory = "umb"
	}
	if b.CDDriver == "" {
		b.CDDriver = "oakcdrom"
	}
	if b.CDLetter == "" {
		b.CDLetter = defaultCDDrive
	}
	b.CDLetter = strings.ToUpper(strings.TrimSuffix(strings.TrimSpace(b.CDLetter), ":"))
	if b.Blaster == "" {
		b.Blaster = defaultBlaster
	}
	if b.NetCard == "" {
		b.NetCard = "ne2000"
	}
	if b.NetArgs == "" {
		b.NetArgs = netCardByID(b.NetCard).args
	}
}

// Title is a short menu text made from the blocks.
func (b BlockSet) Title() string {
	parts := []string{"DOS"}
	if b.Mouse {
		parts = append(parts, T("mouse"))
	}
	if b.CDROM {
		parts = append(parts, "CD-ROM")
	}
	if b.Sound {
		parts = append(parts, T("sound"))
	}
	if b.Net {
		parts = append(parts, T("network"))
	}
	if b.EC && b.Net {
		parts = append(parts, "Easy Commander")
	}
	t := strings.Join(parts, " + ")
	switch b.Memory {
	case "ems":
		t += " (EMS)"
	case "none":
		t += " (" + T("no memory manager") + ")"
	}
	return t
}

// Build makes the lines and the list of drivers for the target DOS.
func (b BlockSet) Build(t Target) (cfg, auto, drivers []string) {
	b.Normalize()
	free := t == TargetFreeDOS
	high := b.Memory == "umb" || b.Memory == "ems"
	dev, lh := "DEVICE=", ""
	if high {
		dev, lh = "DEVICEHIGH=", "LH "
	}
	// memory
	switch {
	case b.Memory == "xms" && free:
		cfg = append(cfg, `DEVICE=C:\FREEDOS\BIN\HIMEMX.EXE`, "DOS=HIGH")
	case b.Memory == "xms":
		cfg = append(cfg, `DEVICE=C:\DOS\HIMEM.SYS`, "DOS=HIGH")
	case b.Memory == "umb" && free:
		cfg = append(cfg, `DEVICE=C:\FREEDOS\BIN\JEMMEX.EXE NOEMS X=TEST I=TEST NOVME NOINVLPG`, "DOS=HIGH,UMB")
	case b.Memory == "umb":
		cfg = append(cfg, `DEVICE=C:\DOS\HIMEM.SYS`, `DEVICE=C:\DOS\EMM386.EXE NOEMS`, "DOS=HIGH,UMB")
	case b.Memory == "ems" && free:
		cfg = append(cfg, `DEVICE=C:\FREEDOS\BIN\JEMMEX.EXE X=TEST I=TEST NOVME NOINVLPG`, "DOS=HIGH,UMB")
	case b.Memory == "ems":
		cfg = append(cfg, `DEVICE=C:\DOS\HIMEM.SYS`, `DEVICE=C:\DOS\EMM386.EXE RAM`, "DOS=HIGH,UMB")
	}
	// CD-ROM: device driver in CONFIG.SYS, extension in AUTOEXEC.BAT
	if b.CDROM {
		switch b.CDDriver {
		case "udvd2":
			if free {
				cfg = append(cfg, dev+`C:\FREEDOS\BIN\UDVD2.SYS /D:MSCD001`)
			} else {
				cfg = append(cfg, dev+`C:\DRIVERS\UDVD2.SYS /D:MSCD001`)
				drivers = append(drivers, "udvd2")
			}
		default:
			cfg = append(cfg, dev+`C:\DRIVERS\OAKCDROM.SYS /D:MSCD001`)
			drivers = append(drivers, "oakcdrom")
		}
		if free {
			auto = append(auto, lh+`C:\FREEDOS\BIN\SHSUCDX.COM /D:MSCD001`)
		} else {
			auto = append(auto, lh+`C:\DOS\MSCDEX.EXE /D:MSCD001 /L:`+b.CDLetter)
		}
	}
	if b.Mouse {
		if free {
			auto = append(auto, lh+`C:\FREEDOS\BIN\CTMOUSE.EXE`)
		} else {
			auto = append(auto, lh+`C:\DRIVERS\CTMOUSE.EXE`)
			drivers = append(drivers, "ctmouse")
		}
	}
	if b.Sound {
		auto = append(auto, "SET BLASTER="+strings.TrimSpace(b.Blaster))
		if b.PnP {
			auto = append(auto, `C:\DRIVERS\UNISOUND.COM /V70`)
			drivers = append(drivers, "unisound")
		}
	}
	if b.Net {
		c := netCardByID(b.NetCard)
		auto = append(auto,
			`SET MTCPCFG=C:\MTCP\MTCP.CFG`,
			fmt.Sprintf(`%sC:\DRIVERS\%s %s`, lh, c.file, strings.TrimSpace(b.NetArgs)),
			`C:\MTCP\DHCP.EXE`)
		drivers = append(drivers, c.id, "mtcp", "mtcpcfg")
		if b.EC {
			auto = append(auto, `C:\EC\EC.EXE`)
			drivers = append(drivers, "ec")
		}
	}
	return cfg, auto, drivers
}

// Preset is a ready combination of blocks.
type Preset struct {
	ID     string // menu item ID
	Name   string // English, show with T()
	Blocks BlockSet
}

var Presets = []Preset{
	{"DOS", N("Plain DOS"), BlockSet{Memory: "umb"}},
	{"MOUSE", N("DOS + mouse"), BlockSet{Memory: "umb", Mouse: true}},
	{"CDROM", N("DOS + mouse + CD-ROM"), BlockSet{Memory: "umb", Mouse: true, CDROM: true}},
	{"FULL", N("Everything: mouse + CD-ROM + sound"), BlockSet{Memory: "umb", Mouse: true, CDROM: true, Sound: true}},
	{"GAMESEMS", N("Games with EMS: mouse + sound"), BlockSet{Memory: "ems", Mouse: true, Sound: true}},
	{"GAMES", N("Games without EMS: mouse + sound"), BlockSet{Memory: "umb", Mouse: true, Sound: true}},
	{"NET", N("Network: packet driver + mTCP + Easy Commander"), BlockSet{Memory: "umb", Net: true, EC: true}},
	{"CLEAN", N("Clean boot (nothing loaded)"), BlockSet{Memory: "none"}},
}

// ItemFromBlocks makes a menu item; the caller adds the drivers to the project.
func ItemFromBlocks(id, title string, b BlockSet, t Target) (Item, []string) {
	b.Normalize()
	cfg, auto, drv := b.Build(t)
	if title == "" {
		title = b.Title()
	}
	bb := b
	return Item{ID: id, Title: title, Config: cfg, Autoexec: auto, Blocks: &bb}, drv
}

// BlocksMatch reports whether an item's lines are still exactly what its
// blocks produce (so rebuilding them loses no hand edits).
func (it *Item) BlocksMatch(t Target) bool {
	if it.Blocks == nil {
		return len(it.Config) == 0 && len(it.Autoexec) == 0
	}
	cfg, auto, _ := it.Blocks.Build(t)
	return strings.Join(cfg, "\n") == strings.Join(it.Config, "\n") && strings.Join(auto, "\n") == strings.Join(it.Autoexec, "\n")
}

// AddDrivers adds driver IDs that are not yet selected.
func (p *Project) AddDrivers(ids []string) {
	have := map[string]bool{}
	for _, d := range p.Drivers {
		have[d] = true
	}
	for _, d := range ids {
		if !have[d] {
			p.Drivers = append(p.Drivers, d)
			have[d] = true
		}
	}
}

// RebuildBlockItems regenerates the lines of block-made items, for example
// after the target DOS changed. Items edited by hand are left alone and
// their IDs returned.
func (p *Project) RebuildBlockItems(from Target) (kept []string) {
	for i := range p.Items {
		it := &p.Items[i]
		if it.Blocks == nil {
			continue
		}
		if !it.BlocksMatch(from) {
			kept = append(kept, it.ID)
			continue
		}
		cfg, auto, drv := it.Blocks.Build(p.Target)
		it.Config, it.Autoexec = cfg, auto
		p.AddDrivers(drv)
	}
	return kept
}

// commonLines are the lines every item shares, per DOS.
func commonStart(t Target) Item {
	if t == TargetFreeDOS {
		return Item{
			Config: []string{`!FILES=40`, `!BUFFERS=20`, `!LASTDRIVE=Z`},
			Autoexec: []string{`@ECHO OFF`, `SET DOSDIR=C:\FREEDOS`, `SET PATH=%DOSDIR%\BIN;C:\DRIVERS`,
				`SET TEMP=%DOSDIR%\TEMP`, `SET TMP=%TEMP%`, `PROMPT $P$G`},
		}
	}
	return Item{
		Config:   []string{`FILES=30`, `BUFFERS=20`, `LASTDRIVE=Z`},
		Autoexec: []string{`@ECHO OFF`, `PROMPT $P$G`, `PATH C:\DOS;C:\DRIVERS`, `SET TEMP=C:\DOS`},
	}
}

func commonEnd(t Target) Item {
	if t == TargetFreeDOS {
		return Item{Config: []string{`SHELL=C:\FREEDOS\BIN\COMMAND.COM C:\FREEDOS\BIN /E:1024 /P=C:\FDAUTO.BAT`}}
	}
	return Item{}
}

// ProjectFromPresets builds a project with the given presets as items.
func ProjectFromPresets(t Target, timeout int, presetIDs ...string) *Project {
	p := &Project{Target: t, Timeout: timeout, Start: commonStart(t), End: commonEnd(t)}
	if t == TargetFreeDOS {
		p.AutoexecName = "FDAUTO.BAT"
	}
	for _, id := range presetIDs {
		for _, pr := range Presets {
			if pr.ID == id {
				it, drv := ItemFromBlocks(pr.ID, "", pr.Blocks, t)
				p.Items = append(p.Items, it)
				p.AddDrivers(drv)
			}
		}
	}
	return p
}

// DefaultProject is what the program starts with: a plain DOS menu.
func DefaultProject(t Target) *Project { return ProjectFromPresets(t, 5, "DOS") }
