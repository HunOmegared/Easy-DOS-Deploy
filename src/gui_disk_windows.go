//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

// ---- "New bootable disk image" window ----

const (
	bwKind = 10 + iota
	bwProfile
	bwSaveProf
	bwDelProf
	bwC
	bwH
	bwS
	bwTotal
	bwFromImg
	bwFromBios
	bwBlank
	bwFreeDOS
	bwSetupDOS
	bwSetupBtn
	bwPlan
	bwSource
	bwSrcBrowse
	bwCopyAll
	bwUseOwn
	bwOwnInfo
	bwOwnLines
	bwLblMenu
	bwMenuKind
	bwMenuEdit
	bwDrvInfo
	bwDrvBtn
	bwLabel
	bwOut
	bwOutBrowse
	bwCreate
	bwClose
	bwLblKind
	bwLblProfile
	bwLblC
	bwLblH
	bwLblS
	bwLblTotal
	bwLblSource
	bwLblLabel
	bwLblOut
	bwHelp
	bwHeadDisk
	bwIntroDisk
	bwHeadSys
	bwIntroSys
	bwHeadBuild
	bwIntroBuild
	bwSysSource // radio: a disk or image that already boots
	bwNoteSetup
	bwNoteFree
	bwNoteSource
	bwNoteBlank
	bwSetupInfo
	bwWriteMenu // build: write the menu of step 4
	bwMenuSum
	bwGeomHelp
	bwOnlyC
	bwWriteCard
	bwAdvanced // shows the image size
	bwAddGeom
)

// System choices (radio buttons of the System page).
const (
	sysSetup = iota
	sysFree
	sysSource
	sysBlank
)

// The image window's controls live on three pages of the main window.
const (
	bpDisk = iota
	bpSystem
	bpBuild
)

func bootPanelOf(id int) int {
	switch id {
	case bwHeadSys, bwIntroSys, bwBlank, bwFreeDOS, bwSetupDOS, bwSetupBtn, bwLblSource, bwSource, bwSrcBrowse, bwCopyAll, bwUseOwn, bwOwnInfo, bwOwnLines,
		bwSysSource, bwNoteSetup, bwNoteFree, bwNoteSource, bwNoteBlank, bwSetupInfo:
		return bpSystem
	case bwHeadBuild, bwIntroBuild, bwLblMenu, bwMenuKind, bwMenuEdit, bwDrvInfo, bwDrvBtn, bwLblLabel, bwLabel, bwLblOut, bwOut, bwOutBrowse, bwCreate, bwClose,
		bwWriteMenu, bwMenuSum, bwWriteCard, bwAddGeom:
		return bpBuild
	}
	return bpDisk
}

type bootWin struct {
	hwnd    uintptr
	c       map[int]uintptr
	loading bool
	srcPath string
	srcDOS  DOSKind
	// the drivers found on the source (nil: none), and the files its lines
	// name that are not on it
	machine     *Machine
	machineMiss []string
	machineSrc  string
	fromSource  []string        // catalog driver files taken from the source
	triedSource map[string]bool // catalog IDs already looked for on it
	panels      [3]uintptr      // the pages (bpDisk ...) in the main window
	sysChoice   int             // sysSetup ...
	// MS-DOS from its Setup disks (bwSetupDOS)
	setup      *SetupDisks
	setupPaths []string
	setupOpt   SetupOptions
}

// openBootWin shows the Machine and disk page.
func (a *app) openBootWin() {
	a.showPage(pageDisk)
}

// createBootPages makes the Machine and disk, System and Build pages.
func (a *app) createBootPages() {
	b := &bootWin{c: map[int]uintptr{}}
	a.boot = b
	b.hwnd = a.hwnd // the owner of its dialogs and message boxes
	for i := range b.panels {
		b.panels[i] = a.newPanel("EasyDOSDeployBoot")
	}
	mk := func(id int, class, text string, style, ex uint32) uintptr {
		h := createWindow(ex, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, b.panels[bootPanelOf(id)], uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		b.c[id] = h
		return h
	}
	head := func(id, intro int, title, text string) {
		sendMsg(mk(id, "STATIC", title, 0, 0), WM_SETFONT, a.head, 1)
		mk(intro, "STATIC", text, 0, 0)
	}
	head(bwHeadDisk, bwIntroDisk, T("Machine and disk"), T("The disk the system goes onto, with the geometry the target machine's BIOS gives to DOS. A disk bigger than 2 GB is split into the drives C:, D: ... so none of it is lost."))
	head(bwHeadSys, bwIntroSys, T("System"), T("Where DOS comes from: your MS-DOS Setup disks, a card or image that already boots, or FreeDOS downloaded by the program."))
	head(bwHeadBuild, bwIntroBuild, T("Build"), T("Makes the disk image with the system, the drivers and the boot menu. Try it in an emulator, then write it to the card."))
	edit := uint32(WS_TABSTOP | ES_AUTOHSCROLL)
	mk(bwLblKind, "STATIC", T("Type:"), 0, 0)
	mk(bwKind, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, 0)
	mk(bwLblProfile, "STATIC", T("Disk profile:"), 0, 0)
	mk(bwProfile, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWN|WS_VSCROLL|ES_AUTOHSCROLL, 0)
	mk(bwSaveProf, "BUTTON", T("Save profile"), WS_TABSTOP, 0)
	mk(bwDelProf, "BUTTON", T("Delete profile"), WS_TABSTOP, 0)
	mk(bwLblC, "STATIC", T("Cylinders:"), 0, 0)
	mk(bwC, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(bwLblH, "STATIC", T("Heads:"), 0, 0)
	mk(bwH, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(bwLblS, "STATIC", T("Sectors/track:"), 0, 0)
	mk(bwS, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(bwFromImg, "BUTTON", T("From a disk backup..."), WS_TABSTOP, 0)
	mk(bwFromBios, "BUTTON", T("From BIOS setup..."), WS_TABSTOP, 0)
	mk(bwGeomHelp, "BUTTON", T("Help with the disk profile..."), WS_TABSTOP, 0)
	mk(bwOnlyC, "BUTTON", T("Only drive C: (2 GB at most), so that the CD-ROM stays D:"), WS_TABSTOP|BS_AUTOCHECKBOX, 0)
	// the System page: one choice, its settings under it
	radio := uint32(WS_TABSTOP | BS_RADIOBUTTON)
	mk(bwSetupDOS, "BUTTON", T("MS-DOS from its Setup disks"), radio|WS_GROUP, 0)
	mk(bwNoteSetup, "STATIC", T("The program installs MS-DOS from the images of your Setup disks into C:\\DOS, as Setup would. Recommended for a new system."), 0, 0)
	mk(bwSetupBtn, "BUTTON", T("Disks..."), WS_TABSTOP, 0)
	mk(bwSetupInfo, "STATIC", "", 0, 0)
	mk(bwFreeDOS, "BUTTON", T("FreeDOS, downloaded by the program"), radio, 0)
	mk(bwNoteFree, "STATIC", T("Free and MS-DOS compatible; most DOS programs and games run on it. The program can download it, so an empty card is enough."), 0, 0)
	mk(bwSysSource, "BUTTON", T("A disk or image that already boots DOS"), radio, 0)
	mk(bwNoteSource, "STATIC", T("A card backup, a hard disk image or a boot floppy image: its boot code and system files are taken, and its drivers can be used."), 0, 0)
	mk(bwBlank, "BUTTON", T("I'll install DOS myself (empty image)"), radio, 0)
	mk(bwNoteBlank, "STATIC", T("For DOS Setup in an emulator: the image has the right geometry but no system."), 0, 0)
	mk(bwLblTotal, "STATIC", T("Image size in sectors (empty: cylinders × heads × sectors):"), 0, 0)
	mk(bwTotal, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(bwHelp, "STATIC", T("Choose a disk profile, or enter the geometry DOS gets from the target machine's BIOS (MSD or GEOM shows it), not the one in BIOS setup. Not sure? Help with the disk profile tells how to find it."), 0, 0)
	mk(bwAdvanced, "BUTTON", T("Advanced: a different image size"), WS_TABSTOP|BS_AUTOCHECKBOX, 0)
	mk(bwPlan, "STATIC", "", 0, 0)
	mk(bwLblSource, "STATIC", T("System source: a bootable disk image or card backup (empty: not bootable):"), 0, 0)
	mk(bwSource, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(bwSrcBrowse, "BUTTON", T("Browse..."), WS_TABSTOP, 0)
	mk(bwCopyAll, "BUTTON", T("Copy all other files of the source too (e.g. the DOS folder)"), WS_TABSTOP|BS_AUTOCHECKBOX, 0)
	mk(bwUseOwn, "BUTTON", T("Use the drivers of the system source"), WS_TABSTOP|BS_AUTOCHECKBOX, 0)
	mk(bwOwnInfo, "STATIC", "", 0, 0)
	mk(bwOwnLines, "BUTTON", T("Lines..."), WS_TABSTOP, 0)
	mk(bwWriteMenu, "BUTTON", T("Write the boot menu of step 4 and its drivers"), WS_TABSTOP|BS_AUTOCHECKBOX, 0)
	mk(bwMenuSum, "STATIC", "", 0, 0)
	mk(bwMenuEdit, "BUTTON", T("Edit..."), WS_TABSTOP, 0)
	mk(bwDrvInfo, "STATIC", "", 0, 0)
	mk(bwDrvBtn, "BUTTON", T("Drivers..."), WS_TABSTOP, 0)
	mk(bwLblLabel, "STATIC", T("Volume label:"), 0, 0)
	sendMsg(mk(bwLabel, "EDIT", "", edit|ES_UPPERCASE, WS_EX_CLIENTEDGE), EM_SETLIMIT, 11, 0)
	mk(bwLblOut, "STATIC", T("New image file:"), 0, 0)
	mk(bwOut, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(bwOutBrowse, "BUTTON", T("Browse..."), WS_TABSTOP, 0)
	mk(bwCreate, "BUTTON", T("Create"), WS_TABSTOP|BS_DEFPUSHBUTTON, 0)
	mk(bwWriteCard, "BUTTON", T("Write to disk..."), WS_TABSTOP, 0)
	mk(bwAddGeom, "BUTTON", T("Put GEOM.COM onto the disk (it shows the geometry the BIOS gives)"), WS_TABSTOP|BS_AUTOCHECKBOX, 0)
	mk(bwClose, "BUTTON", T("Close"), WS_TABSTOP, 0)
	pShowWindow.Call(b.c[bwClose], 0) // a page, not a window
	setCheck(b.c[bwWriteMenu], true)
	b.setSys(sysSource)
	if len(a.set.SetupDisks) > 0 {
		b.sysChoice = sysSetup // last time from the Setup disks: offered again
		setCheck(b.c[bwSetupDOS], true)
		setCheck(b.c[bwSysSource], false)
	}
	fillCombo(b.c[bwKind], MediaNames(), 0)
	b.fillProfiles("")
	b.fillMenus(menuEditor)
	b.layout()
	b.update()
}

// openBootWinWith opens the image window with a type and, optionally, a
// geometry already filled in.
func (a *app) openBootWinWith(kind MediaKind, g *Geometry) {
	a.openBootWin()
	b := a.boot
	sendMsg(b.c[bwKind], CB_SETCURSEL, uintptr(kind), 0)
	if g != nil {
		b.setGeom(*g, 0)
	} else {
		b.update()
	}
}

// Boot menu choices in the image window: the ready menus load into the
// editor (so the Drivers window and the editor show the same), "the menu
// in the editor" uses what is there, "keep" writes no menu.
const (
	menuEditor = 4 // = len(wizMenus)
	menuKeep   = 5
)

// fillMenus shows what the Build page writes: the menu of step 4.
func (b *bootWin) fillMenus(int) {
	a := theApp
	var titles []string
	for _, it := range a.proj.Items {
		titles = append(titles, title(it))
	}
	fd := checked(b.c[bwFreeDOS])
	if fd {
		setCheck(b.c[bwWriteMenu], true) // no menu of a source to keep
	}
	enable(b.c[bwWriteMenu], !fd && !checked(b.c[bwBlank]))
	if checked(b.c[bwWriteMenu]) {
		setText(b.c[bwMenuSum], T("Boot menu (step 4): %s", strings.Join(titles, "; ")))
	} else {
		setText(b.c[bwMenuSum], T("The CONFIG.SYS and AUTOEXEC.BAT of the system source stay as they are."))
	}
	b.updateDrivers()
}

func (b *bootWin) menuChoice() int {
	if checked(b.c[bwWriteMenu]) || checked(b.c[bwFreeDOS]) {
		return menuEditor
	}
	return menuKeep
}

// setSys chooses the system (the radio buttons of the System page).
func (b *bootWin) setSys(k int) {
	b.sysChoice = k
	for i, id := range []int{bwSetupDOS, bwFreeDOS, bwSysSource, bwBlank} {
		setCheck(b.c[id], i == k)
	}
	b.layout()
}

// sourceTarget is the DOS of the system source, or the editor's.
func (b *bootWin) sourceTarget() Target {
	a := theApp
	if checked(b.c[bwFreeDOS]) {
		return TargetFreeDOS
	}
	if b.setupMode() {
		return TargetMSDOS
	}
	src := strings.Trim(strings.TrimSpace(getText(b.c[bwSource])), `"`)
	if src != "" {
		if src != b.srcPath {
			b.srcPath, b.srcDOS = src, DOSNone
			if di, err := ReadDiskInfo(src); err == nil {
				b.srcDOS = di.DOS
			}
		}
		switch b.srcDOS {
		case DOSFree:
			return TargetFreeDOS
		case DOSMS, DOSPC:
			return TargetMSDOS
		}
	}
	return a.proj.Target
}

// findMachine reads the drivers of the system source, once per source.
func (b *bootWin) findMachine() {
	src := strings.Trim(strings.TrimSpace(getText(b.c[bwSource])), `"`)
	if b.setupMode() || checked(b.c[bwFreeDOS]) {
		src = "" // no drivers on Setup disks; FreeDOS has no source
	}
	if src == b.machineSrc {
		return
	}
	b.machineSrc, b.machine, b.machineMiss, b.fromSource, b.triedSource = src, nil, nil, nil, map[string]bool{}
	if src != "" {
		if m, miss, err := MachineFromImage(src); err == nil && !m.Empty() {
			b.machine, b.machineMiss = m, miss
			setCheck(b.c[bwUseOwn], true) // a new source: its drivers are the ones known to work
		}
	}
}

// takeFromSource copies the catalog drivers the menu needs and that are on
// the system source into the drivers folder, so they need not be browsed
// for. Never the FreeDOS kernel or FreeCOM: an MS-DOS source has a
// COMMAND.COM of its own.
func (b *bootWin) takeFromSource() {
	a := theApp
	if b.machineSrc == "" || checked(b.c[bwFreeDOS]) {
		return
	}
	got := false
	for _, id := range a.proj.Drivers {
		d := a.store.Find(id)
		if d == nil || id == fdKernelID || id == fdCommandID || b.triedSource[id] || a.store.Ready(d) {
			continue
		}
		b.triedSource[id] = true
		if files, err := a.store.extractFromImage(d, b.machineSrc); err == nil && len(files) > 0 {
			b.fromSource = append(b.fromSource, files...)
			got = true
		}
	}
	if got {
		a.updateStatus()
	}
}

// useOwn is true when the new menu uses the drivers of the source.
func (b *bootWin) useOwn() bool {
	return b.machine != nil && checked(b.c[bwUseOwn]) && !checked(b.c[bwFreeDOS]) && b.menuChoice() != menuKeep
}

// applyOwn shows what was found on the source and makes the editor's menu
// use those drivers or the program's own.
func (b *bootWin) applyOwn() {
	a := theApp
	b.findMachine()
	usable := b.machine != nil && !checked(b.c[bwFreeDOS]) && b.menuChoice() != menuKeep && !checked(b.c[bwBlank])
	enable(b.c[bwUseOwn], usable)
	enable(b.c[bwOwnLines], b.machine != nil)
	setText(b.c[bwOwnInfo], b.ownInfo())
	var want *Machine
	if b.useOwn() {
		want = b.machine
		setCheck(b.c[bwCopyAll], true) // the drivers are files of the source
	}
	if want != a.proj.Machine {
		kept := a.proj.UseMachine(want)
		a.autoChanged()
		a.loadProject()
		if len(kept) > 0 {
			msgBox(b.hwnd, T("These menu items were edited by hand, so their lines were not changed: %s", strings.Join(kept, ", ")), appTitle(), MB_ICONINFORMATION)
		}
	}
	b.updateDrivers()
}

// setupMode is true when MS-DOS comes from its Setup disks.
func (b *bootWin) setupMode() bool { return checked(b.c[bwSetupDOS]) }

// setupChanged shows the Setup disks in the source line and follows the
// DOS of the choice.
func (b *bootWin) setupChanged() {
	if b.setup != nil {
		ver := b.setup.Version
		if ver == "" {
			ver = "MS-DOS"
		}
		files, missing := b.setup.DOSFiles(b.setupOpt)
		t := T("%s from %d Setup disks: %d files go into C:\\DOS.", ver, len(b.setupPaths), len(files))
		if len(missing) > 0 {
			t += "\n" + strings.Join(missing, "\n")
		}
		setText(b.c[bwSetupInfo], t)
	} else {
		setText(b.c[bwSetupInfo], T("No disks yet: choose the images of your Setup disks."))
	}
	b.update()
	b.followSource()
}

// followSource switches the editor's menu to the DOS of a new source.
func (b *bootWin) followSource() {
	a := theApp
	t := b.sourceTarget()
	if t != a.proj.Target {
		from := a.proj.Target
		a.proj.Target = t
		a.targetChanged(from)
		a.autoChanged()
		a.loadProject()
	}
	b.fillMenus(b.menuChoice())
	b.applyOwn()
}

func (b *bootWin) updateDrivers() {
	a := theApp
	if b.menuChoice() == menuKeep {
		setText(b.c[bwDrvInfo], T("No menu and no drivers are written; the files of the source stay as they are."))
		enable(b.c[bwDrvBtn], false)
		return
	}
	enable(b.c[bwDrvBtn], true)
	b.takeFromSource()
	dl, br := DriverNeeds(a.proj, a.store, checked(b.c[bwFreeDOS]))
	var lines []string
	if len(b.fromSource) > 0 {
		lines = append(lines, T("Taken from the system source: %s", strings.Join(b.fromSource, ", ")))
	}
	if len(dl) > 0 {
		lines = append(lines, T("To download: %s", strings.Join(dl, ", ")))
	}
	if len(br) > 0 {
		lines = append(lines, T("To browse for (Drivers...): %s", strings.Join(br, ", ")))
	}
	switch {
	case len(lines) > 0:
		setText(b.c[bwDrvInfo], strings.Join(lines, "\n"))
	case len(a.proj.Drivers) == 0:
		setText(b.c[bwDrvInfo], T("This menu needs no extra drivers."))
	default:
		setText(b.c[bwDrvInfo], T("All %d drivers of this menu are here.", len(a.proj.Drivers)))
	}
}

// builtinProfiles come before the user's own disk profiles.
func builtinProfiles() []GeomProfile {
	g := UniversalGeom
	return []GeomProfile{{Name: T("Universal starter card: any IDE BIOS, about 500 MB (BIOS: User 1024/16/63)"), C: g.C, H: g.H, S: g.S}}
}

func allProfiles() []GeomProfile { return append(builtinProfiles(), theApp.set.Profiles...) }

func (b *bootWin) fillProfiles(sel string) {
	cb := b.c[bwProfile]
	sendMsg(cb, CB_RESETCONTENT, 0, 0)
	idx := -1
	for i, p := range allProfiles() {
		sendMsg(cb, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(p.Name))))
		if p.Name == sel {
			idx = i
		}
	}
	if idx >= 0 {
		sendMsg(cb, CB_SETCURSEL, uintptr(idx), 0)
	} else {
		setText(cb, sel)
	}
}

func (b *bootWin) layout() {
	a := theApp
	s := a.s
	m := s(14)
	rh := s(23)
	lh := s(18)
	lp, bp := s(6), s(24) // padding of labels and buttons
	// heading and introduction of a page; returns the first free y
	top := func(head, intro, w int) int {
		move(b.c[head], m, m, w-2*m, s(28))
		move(b.c[intro], m, m+s(32), w-2*m, 3*lh)
		return m + s(32) + 3*lh + s(10)
	}
	lbl := func(id, x, y, ww int) { move(b.c[id], x, y+s(4), ww, lh) }

	// Machine and disk
	w, _ := clientSize(b.panels[bpDisk])
	cw := w - 2*m
	col := fitW(s(110), lp, b.c[bwLblKind], b.c[bwLblProfile])
	y := top(bwHeadDisk, bwIntroDisk, w)
	lbl(bwLblKind, m, y, col)
	move(b.c[bwKind], m+col, y, s(220), rh+s(160))
	y += rh + s(10)
	lbl(bwLblProfile, m, y, col)
	pb := fitW(s(110), bp, b.c[bwSaveProf], b.c[bwDelProf])
	move(b.c[bwProfile], m+col, y, cw-col-2*pb-s(12), rh+s(160))
	move(b.c[bwSaveProf], w-m-2*pb-s(6), y, pb, rh)
	move(b.c[bwDelProf], w-m-pb, y, pb, rh)
	y += rh + s(4)
	gh := fitW(s(160), bp, b.c[bwGeomHelp])
	move(b.c[bwGeomHelp], m+col, y, gh, rh)
	y += rh + s(10)
	move(b.c[bwHelp], m, y, cw, 2*lh)
	y += 2*lh + s(8)
	x := m
	for _, p := range []struct{ l, e, lw int }{{bwLblC, bwC, 70}, {bwLblH, bwH, 50}, {bwLblS, bwS, 100}} {
		lw := fitW(s(p.lw), lp, b.c[p.l])
		move(b.c[p.l], x, y+s(4), lw, lh)
		x += lw
		move(b.c[p.e], x, y, s(60), rh)
		x += s(60) + s(14)
	}
	g1 := fitW(0, bp, b.c[bwFromImg])
	g2 := fitW(0, bp, b.c[bwFromBios])
	if half := (w - m - x - s(6)) / 2; g1 <= half && g2 <= half {
		move(b.c[bwFromImg], x, y, half, rh)
		move(b.c[bwFromBios], x+half+s(6), y, w-m-x-half-s(6), rh)
	} else {
		y += rh + s(6)
		move(b.c[bwFromImg], m, y, g1, rh)
		move(b.c[bwFromBios], m+g1+s(6), y, g2, rh)
	}
	y += rh + s(12)
	move(b.c[bwPlan], m, y, cw, 4*lh)
	y += 4*lh + s(6)
	move(b.c[bwOnlyC], m, y, cw, rh)
	y += rh + s(16)
	move(b.c[bwAdvanced], m, y, cw, rh)
	y += rh + s(4)
	adv := checked(b.c[bwAdvanced])
	for _, id := range []int{bwLblTotal, bwTotal} {
		v := uintptr(0)
		if adv {
			v = SW_SHOW
		}
		pShowWindow.Call(b.c[id], v)
	}
	move(b.c[bwLblTotal], m+s(22), y+s(4), cw-s(22)-s(130), lh)
	move(b.c[bwTotal], w-m-s(120), y, s(120), rh)

	// System: the four choices; the settings of the chosen one under it
	w, _ = clientSize(b.panels[bpSystem])
	cw = w - 2*m
	sb := fitW(s(100), bp, b.c[bwSrcBrowse], b.c[bwSetupBtn], b.c[bwOwnLines])
	in := m + s(22) // indent of notes and settings
	iw := w - m - in
	y = top(bwHeadSys, bwIntroSys, w)
	details := map[int][]int{
		sysSetup:  {bwSetupBtn, bwSetupInfo},
		sysSource: {bwLblSource, bwSource, bwSrcBrowse, bwCopyAll, bwUseOwn, bwOwnLines, bwOwnInfo},
	}
	for k, r := range [][2]int{{bwSetupDOS, bwNoteSetup}, {bwFreeDOS, bwNoteFree}, {bwSysSource, bwNoteSource}, {bwBlank, bwNoteBlank}} {
		move(b.c[r[0]], m, y, cw, rh)
		y += rh
		move(b.c[r[1]], in, y, iw, 2*lh)
		y += 2*lh + s(4)
		for _, id := range details[k] {
			v := uintptr(0)
			if k == b.sysChoice {
				v = SW_SHOW
			}
			pShowWindow.Call(b.c[id], v)
		}
		if k != b.sysChoice {
			y += s(10)
			continue
		}
		switch k {
		case sysSetup:
			move(b.c[bwSetupBtn], in, y, sb, rh)
			move(b.c[bwSetupInfo], in+sb+s(12), y+s(4), iw-sb-s(12), 3*lh)
			y += 3*lh + s(8)
		case sysSource:
			move(b.c[bwLblSource], in, y, iw, lh)
			y += lh + s(4)
			move(b.c[bwSource], in, y, iw-sb-s(10), rh)
			move(b.c[bwSrcBrowse], w-m-sb, y, sb, rh)
			y += rh + s(6)
			move(b.c[bwCopyAll], in, y, iw, rh)
			y += rh + s(2)
			move(b.c[bwUseOwn], in, y, iw-sb-s(10), rh)
			move(b.c[bwOwnLines], w-m-sb, y, sb, rh)
			y += rh + s(2)
			move(b.c[bwOwnInfo], in+s(20), y, iw-s(20)-sb-s(10), 4*lh)
			y += 4*lh + s(6)
		}
		y += s(10)
	}

	// Build
	w, h := clientSize(b.panels[bpBuild])
	cw = w - 2*m
	col = fitW(s(110), lp, b.c[bwLblLabel])
	sb = fitW(s(100), bp, b.c[bwMenuEdit], b.c[bwDrvBtn], b.c[bwOutBrowse])
	y = top(bwHeadBuild, bwIntroBuild, w)
	move(b.c[bwWriteMenu], m, y, cw-sb-s(10), rh)
	move(b.c[bwMenuEdit], w-m-sb, y, sb, rh)
	y += rh + s(2)
	move(b.c[bwMenuSum], m+s(22), y, cw-s(22)-sb-s(10), 2*lh)
	y += 2*lh + s(6)
	move(b.c[bwDrvInfo], m+s(22), y+s(4), cw-s(22)-sb-s(10), 4*lh)
	move(b.c[bwDrvBtn], w-m-sb, y, sb, rh)
	y += 4*lh + s(12)
	lbl(bwLblLabel, m, y, col)
	move(b.c[bwLabel], m+col, y, s(140), rh)
	y += rh + s(8)
	move(b.c[bwAddGeom], m, y, cw, rh)
	y += rh + s(12)
	move(b.c[bwLblOut], m, y, cw, lh)
	y += lh + s(4)
	move(b.c[bwOut], m, y, cw-sb-s(10), rh)
	move(b.c[bwOutBrowse], w-m-sb, y, sb, rh)
	bw := fitW(s(160), bp, b.c[bwCreate], b.c[bwWriteCard])
	move(b.c[bwCreate], w-m-2*bw-s(10), h-m-s(32), bw, s(32))
	move(b.c[bwWriteCard], w-m-bw, h-m-s(32), bw, s(32))
}

func atoiU(s string) uint32 {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if err != nil {
		return 0
	}
	return uint32(n)
}

func (b *bootWin) options() BootOptions {
	o := BootOptions{
		Kind:    MediaKind(sendMsg(b.c[bwKind], CB_GETCURSEL, 0, 0)),
		Geom:    Geometry{atoiU(getText(b.c[bwC])), atoiU(getText(b.c[bwH])), atoiU(getText(b.c[bwS]))},
		Source:  strings.Trim(strings.TrimSpace(getText(b.c[bwSource])), `"`),
		CopyAll: sendMsg(b.c[bwCopyAll], BM_GETCHECK, 0, 0) == 1,
		Label:   strings.TrimSpace(getText(b.c[bwLabel])),
		Path:    strings.Trim(strings.TrimSpace(getText(b.c[bwOut])), `"`),
		OnlyC:   checked(b.c[bwOnlyC]),
		AddGeom: checked(b.c[bwAddGeom]),
	}
	if t, err := strconv.ParseInt(strings.TrimSpace(getText(b.c[bwTotal])), 10, 64); err == nil {
		o.Total = t
	}
	o.Blank = o.Kind == MediaHDD && checked(b.c[bwBlank])
	if !checked(b.c[bwSysSource]) {
		o.Source, o.CopyAll = "", false
	}
	if checked(b.c[bwFreeDOS]) && !o.Blank {
		o.Source, o.CopyAll = "", false
	}
	if b.setupMode() && !o.Blank {
		o.Source, o.CopyAll = "", false
		if b.setup != nil {
			o.Source = b.setup.Boot
			o.Extra, _ = b.setup.DOSFiles(b.setupOpt)
			o.NoDriveSpace = !b.setupOpt.DriveSpace
		}
	}
	return o
}

// updateWriteCard enables "Write to disk" when the image file is there.
func (b *bootWin) updateWriteCard() {
	p := b.options().Path
	_, err := os.Stat(p)
	enable(b.c[bwWriteCard], p != "" && err == nil)
}

// update enables the hard disk fields and shows the partition plan.
func (b *bootWin) update() {
	if b.loading {
		return
	}
	b.updateWriteCard()
	o := b.options()
	hdd := o.Kind == MediaHDD
	for _, id := range []int{bwProfile, bwSaveProf, bwDelProf, bwC, bwH, bwS, bwTotal, bwFromImg, bwFromBios, bwBlank} {
		enable(b.c[id], hdd)
	}
	fd := checked(b.c[bwFreeDOS]) || b.setupMode()
	for _, id := range []int{bwWriteMenu, bwMenuEdit, bwDrvBtn, bwLabel} {
		enable(b.c[id], !o.Blank)
	}
	for _, id := range []int{bwSource, bwSrcBrowse, bwCopyAll} {
		enable(b.c[id], !o.Blank && !fd)
	}
	enable(b.c[bwBlank], hdd)
	if !hdd && checked(b.c[bwBlank]) {
		b.setSys(sysSource) // an empty floppy image is not offered
	}
	if !hdd {
		setText(b.c[bwPlan], T("Standard floppy format (FAT12), as DOS FORMAT makes it."))
		return
	}
	if o.Geom.C == 0 && o.Geom.H == 0 && o.Geom.S == 0 {
		setText(b.c[bwPlan], T("Give the geometry or choose a profile."))
		return
	}
	p, err := PlanHDD(o.Geom)
	if err != nil {
		setText(b.c[bwPlan], T("Error: %s", err.Error()))
		return
	}
	bigger := len(p.Logical) > 0
	enable(b.c[bwOnlyC], bigger)
	if o.OnlyC {
		p.OnlyC()
	}
	t := p.Describe()
	if o.Geom == UniversalGeom {
		t += "\n" + T("Starter card: in the BIOS setup of each machine set this drive to type User, 1024 cylinders, 16 heads, 63 sectors, and LBA off (NORMAL or CHS). Not Auto.")
	}
	switch {
	case bigger && o.OnlyC:
		t += "\n" + T("The disk is bigger than 2 GB: the space after C: stays unused.")
	case bigger:
		t += "\n" + T("Note: some CD-ROM games only work when the CD-ROM is drive D:. With D: on the disk, the CD-ROM becomes %c:. If you play such games, tick \"Only drive C:\".", 'C'+len(p.Drives()))
	}
	if o.Total > 0 && o.Total < o.Geom.Sectors() {
		t += "  " + T("The size is smaller than cylinders × heads × sectors; that is used instead.")
	}
	setText(b.c[bwPlan], t)
}

func (b *bootWin) setGeom(g Geometry, total int64) {
	b.loading = true
	setText(b.c[bwC], strconv.Itoa(int(g.C)))
	setText(b.c[bwH], strconv.Itoa(int(g.H)))
	setText(b.c[bwS], strconv.Itoa(int(g.S)))
	if total > 0 && total != g.Sectors() {
		setText(b.c[bwTotal], strconv.FormatInt(total, 10))
		setCheck(b.c[bwAdvanced], true)
	} else {
		setText(b.c[bwTotal], "")
	}
	b.layout()
	b.loading = false
	b.update()
}

func (b *bootWin) loadProfile() {
	a := theApp
	i := int(sendMsg(b.c[bwProfile], CB_GETCURSEL, 0, 0))
	all := allProfiles()
	if i < 0 || i >= len(all) {
		return
	}
	p := all[i]
	_ = a
	sendMsg(b.c[bwKind], CB_SETCURSEL, uintptr(MediaHDD), 0)
	b.setGeom(Geometry{p.C, p.H, p.S}, p.Total)
}

func (b *bootWin) saveProfile() {
	a := theApp
	name := strings.TrimSpace(getText(b.c[bwProfile]))
	o := b.options()
	if _, err := PlanHDD(o.Geom); err != nil {
		msgBox(b.hwnd, err.Error(), appTitle(), MB_ICONWARNING)
		return
	}
	if name == "" {
		var ok bool
		if name, ok = inputBox(b.hwnd, T("Save profile"), T("Name of the disk profile:"), T("For example: Integra 486, 1 GB CF"), ""); !ok {
			return
		}
	}
	p := GeomProfile{Name: name, C: o.Geom.C, H: o.Geom.H, S: o.Geom.S, Total: o.Total}
	found := false
	for i := range a.set.Profiles {
		if a.set.Profiles[i].Name == name {
			old := a.set.Profiles[i]
			if (old.C != p.C || old.H != p.H || old.S != p.S || old.Total != p.Total) &&
				msgBox(b.hwnd, T("The profile %s already exists with %d/%d/%d. Overwrite it?", name, old.C, old.H, old.S), appTitle(), MB_YESNO|MB_ICONQUESTION) != IDYES {
				return
			}
			a.set.Profiles[i], found = p, true
		}
	}
	if !found {
		a.set.Profiles = append(a.set.Profiles, p)
	}
	if err := saveSettings(a.data, a.set); err != nil {
		msgBox(b.hwnd, T("Could not save:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
	}
	b.fillProfiles(name)
}

func (b *bootWin) deleteProfile() {
	a := theApp
	i := int(sendMsg(b.c[bwProfile], CB_GETCURSEL, 0, 0)) - len(builtinProfiles())
	if i < 0 {
		if i >= -len(builtinProfiles()) {
			msgBox(b.hwnd, T("This profile is built in; it cannot be deleted."), appTitle(), MB_ICONINFORMATION)
		}
		return
	}
	if i >= len(a.set.Profiles) {
		return
	}
	if msgBox(b.hwnd, T("Delete profile %s?", a.set.Profiles[i].Name), appTitle(), MB_YESNO|MB_ICONQUESTION) != IDYES {
		return
	}
	a.set.Profiles = append(a.set.Profiles[:i], a.set.Profiles[i+1:]...)
	saveSettings(a.data, a.set)
	b.fillProfiles("")
}

func (b *bootWin) fromImage() {
	p := fileDialog(b.hwnd, false, T("Card backup or hard disk image"), "", "", imgFilter()...)
	if p == "" {
		return
	}
	b.applyDiskInfo(p)
}

func (b *bootWin) applyDiskInfo(p string) {
	di, err := ReadDiskInfo(p)
	if err != nil {
		msgBox(b.hwnd, T("Could not read it:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	if !di.Partitioned {
		msgBox(b.hwnd, T("This is a floppy image (no partition table); it has no card geometry. It can still be the system source of a floppy."), appTitle(), MB_ICONINFORMATION)
		return
	}
	sendMsg(b.c[bwKind], CB_SETCURSEL, uintptr(MediaHDD), 0)
	b.setGeom(di.Geom, di.Total)
	if di.DOS != DOSNone {
		b.setSys(sysSource)
		setText(b.c[bwSource], p)
		b.followSource()
	}
	msgBox(b.hwnd, T("Read from the image:\n\nGeometry (as FDISK recorded it): %d / %d / %d\nSize: %d sectors (%s)\nVolume: %s\nSystem: %s\n\nCheck that heads and sectors match what MSD shows on the target machine.",
		di.Geom.C, di.Geom.H, di.Geom.S, di.Total, humanSize(di.Total*512), di.Desc, di.DOS), appTitle(), MB_ICONINFORMATION)
}

func (b *bootWin) create() {
	a := theApp
	o := b.options()
	asked := false
	if o.Path == "" {
		// as in any Windows program: Save as
		name := "dos.img"
		if a.path != "" {
			name = strings.TrimSuffix(filepath.Base(a.path), filepath.Ext(a.path)) + ".img"
		}
		p := fileDialog(b.hwnd, true, T("New image file"), "img", name, T("Disk images (*.img, *.ima, *.vhd, *.hdd, *.dsk)"), "*.img;*.ima", T("All files"), "*.*")
		if p == "" {
			return
		}
		o.Path, asked = p, true // the dialog asked about overwriting
	}
	if filepath.Ext(o.Path) == "" {
		o.Path += ".img"
	}
	setText(b.c[bwOut], o.Path)
	if _, err := os.Stat(o.Path); err == nil && !asked {
		if msgBox(b.hwnd, T("%s already exists. Overwrite it?", o.Path), appTitle(), MB_YESNO|MB_ICONQUESTION) != IDYES {
			return
		}
	}
	if o.Source != "" && filepath.Clean(o.Source) == filepath.Clean(o.Path) {
		msgBox(b.hwnd, T("The system source and the new image must be different files."), appTitle(), MB_ICONWARNING)
		return
	}
	if o.Blank {
		r, err := CreateBootImage(o)
		if err != nil {
			msgBox(b.hwnd, T("Could not create the image:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
			return
		}
		msgBox(b.hwnd, reportText(r, T("The empty image is ready: %s", o.Path))+"\n\n"+
			T("Next:\n1. 86Box: add the image as a hard disk with %d cylinders, %d heads, %d sectors.\n2. Boot your DOS Setup disk 1 (or a boot floppy: FDISK, FORMAT C: /S) and install DOS.\n3. Here: Output > Write to disk image... puts the boot menu and drivers on it.\n4. Disk image > Write an image to a disk.", o.Geom.C, o.Geom.H, o.Geom.S),
			appTitle(), MB_ICONINFORMATION)
		return
	}
	if b.setupMode() && !o.Blank && b.setup == nil {
		a.openSetupDisks()
		return
	}
	withMenu := b.menuChoice() != menuKeep
	if withMenu && (!a.ready() || !a.checkDrivers()) {
		return
	}
	if b.useOwn() && !o.CopyAll {
		if msgBox(b.hwnd, T("The drivers of the system source are files on it, so all its files should be copied too. Copy all files of the source?"), appTitle(), MB_YESNO|MB_ICONQUESTION) == IDYES {
			setCheck(b.c[bwCopyAll], true)
			o.CopyAll = true
		}
	}
	if checked(b.c[bwFreeDOS]) {
		files, missing := FreeDOSFiles(a.proj, a.store)
		if len(missing) > 0 {
			b.downloadFreeDOS(missing)
			return
		}
		o.FreeDOS = files
	}
	// the work runs in the background with a progress window; the report
	// is shown when it is done
	var r *Report
	var menuErr error
	var missing []string
	if b.setupMode() && b.setup != nil {
		_, missing = b.setup.DOSFiles(b.setupOpt)
	}
	proj, store := a.proj, a.store
	a.startJob(T("Creating the image: %s", filepath.Base(o.Path)), func(prog Progress) error {
		o.Progress = func(done, total int) { prog(phaseFiles, int64(done), int64(total)) }
		var err error
		if r, err = CreateBootImage(o); err != nil {
			return err
		}
		r.Warnings = append(r.Warnings, missing...)
		if withMenu {
			prog(phaseMenu, 0, 1)
			r2, err := WriteToImage(o.Path, proj, store, true)
			// the menu writer reports the free space again at the end
			if n := len(r.Done); n > 0 && strings.HasPrefix(r.Done[n-1], strings.TrimSpace(strings.Split(T("Free space: %s", ""), ":")[0])) {
				r.Done = r.Done[:n-1]
			}
			if r2 != nil {
				r.Done = append(r.Done, r2.Done...)
				for _, w := range r2.Warnings {
					if !strings.Contains(w, "IO.SYS") { // the system check is ours already
						r.Warnings = append(r.Warnings, w)
					}
				}
			}
			menuErr = err
		}
		if len(proj.Files) > 0 && menuErr == nil && o.Kind == MediaHDD {
			r3, err := CopyOwnFiles(o.Path, proj.Files, func(d, n int) { prog(phaseOwn, int64(d), int64(n)) })
			if err != nil {
				menuErr = err
			} else {
				r.Done = append(r.Done, r3.Done...)
			}
		}
		return nil
	}, func(err error) {
		b.created(o, r, err, menuErr)
	})
}

// created reports a finished image.
func (b *bootWin) created(o BootOptions, r *Report, err, menuErr error) {
	a := theApp
	if err != nil {
		msgBox(b.hwnd, T("Could not create the image:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	if menuErr != nil {
		msgBox(b.hwnd, reportText(r, T("Error: %s", menuErr.Error())), appTitle(), MB_ICONERROR)
		return
	}
	if b.useOwn() {
		r.Done = append(r.Done, T("Drivers of the system source used: %s", b.machine.Summary()))
		if len(b.machineMiss) > 0 {
			r.Warnings = append(r.Warnings, T("The lines of the system source name files that are not on it: %s", strings.Join(b.machineMiss, ", ")))
		}
	}
	b.updateWriteCard()
	text := reportText(r, T("The image is ready: %s", o.Path))
	if o.Kind == MediaHDD {
		text += "\n\n" + T("Try it in 86Box with the same geometry (%d/%d/%d) first.\n\nWrite it to a card now?", o.Geom.C, o.Geom.H, o.Geom.S)
		if msgBox(b.hwnd, text, appTitle(), MB_YESNO|MB_ICONINFORMATION) == IDYES {
			a.writeCard(o.Path)
		}
		return
	}
	msgBox(b.hwnd, text, appTitle(), MB_ICONINFORMATION)
}

func bootProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	b := a.boot
	switch msg {
	case WM_SIZE:
		if b != nil && len(b.c) > 0 {
			b.layout()
		}
		return 0
	case WM_GETMINMAXINFO:
		mm := *(**minMaxInfo)(unsafe.Pointer(&lParam))
		mm.minTrack = point{int32(a.s(640)), int32(a.s(560))}
		return 0
	case WM_COMMAND:
		if b == nil {
			return 0
		}
		code := hiword(wParam)
		switch int(loword(wParam)) {
		case bwKind:
			if code == CBN_SELCHANGE {
				b.update()
			}
		case bwProfile:
			if code == CBN_SELCHANGE {
				b.loadProfile()
			}
		case bwOnlyC:
			if code == BN_CLICKED {
				b.update()
			}
		case bwAdvanced:
			if code == BN_CLICKED {
				if !checked(b.c[bwAdvanced]) {
					setText(b.c[bwTotal], "") // hidden: from the geometry again
				}
				b.layout()
			}
		case bwC, bwH, bwS, bwTotal:
			if code == EN_CHANGE {
				b.update()
			}
		case bwWriteMenu:
			if code == BN_CLICKED {
				b.fillMenus(0)
				b.applyOwn()
			}
		case bwMenuEdit:
			a.showPage(pageMenu)
		case bwGeomHelp:
			a.openGeomHelp()
		case bwDrvBtn:
			a.openDrivers()
		case bwSaveProf:
			b.saveProfile()
		case bwDelProf:
			b.deleteProfile()
		case bwFromImg:
			b.fromImage()
		case bwFromBios:
			a.openCalc()
		case bwSetupDOS, bwFreeDOS, bwSysSource, bwBlank:
			if code == BN_CLICKED {
				k := map[int]int{bwSetupDOS: sysSetup, bwFreeDOS: sysFree, bwSysSource: sysSource, bwBlank: sysBlank}[int(loword(wParam))]
				b.setSys(k)
				b.setupChanged()
				if k == sysSetup && b.setup == nil {
					a.openSetupDisks()
				}
			}
		case bwSetupBtn:
			a.openSetupDisks()
		case bwUseOwn:
			if code == BN_CLICKED {
				b.applyOwn()
			}
		case bwOwnLines:
			b.showOwnLines()
		case bwSrcBrowse:
			if p := fileDialog(hwnd, false, T("System source"), "", "", imgFilter()...); p != "" {
				setText(b.c[bwSource], p)
				b.followSource()
			}
		case bwOutBrowse:
			if p := fileDialog(hwnd, true, T("New image file"), "img", "dos.img", T("Disk images (*.img, *.ima, *.vhd, *.hdd, *.dsk)"), "*.img;*.ima", T("All files"), "*.*"); p != "" {
				setText(b.c[bwOut], p)
			}
		case bwCreate:
			b.create()
		case bwWriteCard:
			if p := b.options().Path; p != "" {
				a.writeCard(p)
			}
		case bwOut:
			if code == EN_CHANGE {
				b.updateWriteCard()
			}
		}
		theApp.updateSide()
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// ---- card picker ----

const (
	pkList = 10 + iota
	pkRefresh
	pkOK
	pkCancel
	pkHead
)

type pickWin struct {
	hwnd  uintptr
	c     map[int]uintptr
	disks []PhysDisk
	done  func(PhysDisk)
}

func (a *app) pickCard(title string, done func(PhysDisk)) {
	if a.pick != nil {
		a.pick.close()
	}
	p := &pickWin{c: map[int]uintptr{}, done: done}
	a.pick = p
	p.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyDOSDeployPick", title, WS_OVERLAPPEDWINDOW&^0x00050000, cwDefault, cwDefault, a.s(560), a.s(320), a.hwnd, 0)
	mk := func(id int, class, text string, style, ex uint32) {
		h := createWindow(ex, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, p.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		p.c[id] = h
	}
	mk(pkHead, "STATIC", T("Removable disks only: CF and SD cards in a reader, IDE disks on a USB adapter, and USB drives. Insert the card or connect the adapter, then click Refresh. The Windows system disk is never listed."), 0, 0)
	mk(pkList, "LISTBOX", "", WS_TABSTOP|LBS_NOTIFY|LBS_NOINTEGRALHEIGHT|WS_VSCROLL, WS_EX_CLIENTEDGE)
	mk(pkRefresh, "BUTTON", T("Refresh"), WS_TABSTOP, 0)
	mk(pkOK, "BUTTON", T("OK"), WS_TABSTOP|BS_DEFPUSHBUTTON, 0)
	mk(pkCancel, "BUTTON", T("Cancel"), WS_TABSTOP, 0)
	p.layout()
	p.refresh()
	enable(a.hwnd, false)
	pShowWindow.Call(p.hwnd, SW_SHOW)
}

func (p *pickWin) layout() {
	a := theApp
	w, h := clientSize(p.hwnd)
	s := a.s
	m := s(10)
	bh := s(28)
	move(p.c[pkHead], m, m, w-2*m, s(36))
	move(p.c[pkList], m, m+s(40), w-2*m, h-3*m-s(40)-bh)
	move(p.c[pkRefresh], m, h-m-bh, fitW(s(110), s(24), p.c[pkRefresh]), bh)
	bw := fitW(s(100), s(24), p.c[pkOK], p.c[pkCancel])
	move(p.c[pkOK], w-m-2*bw-s(8), h-m-bh, bw, bh)
	move(p.c[pkCancel], w-m-bw, h-m-bh, bw, bh)
}

func (p *pickWin) refresh() {
	lb := p.c[pkList]
	sendMsg(lb, LB_RESETCONTENT, 0, 0)
	p.disks = nil
	for _, d := range ListDisks() {
		if d.Card() {
			p.disks = append(p.disks, d)
			sendMsg(lb, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(d.Label()))))
		}
	}
	if len(p.disks) == 0 {
		sendMsg(lb, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(T("No suitable disk found.")))))
	} else {
		sendMsg(lb, LB_SETCURSEL, 0, 0)
	}
	enable(p.c[pkOK], len(p.disks) > 0)
}

func (p *pickWin) close() {
	a := theApp
	enable(a.hwnd, true)
	pDestroyWindow.Call(p.hwnd)
	a.pick = nil
	pSetForegroundWindow.Call(a.hwnd)
}

func (p *pickWin) ok() {
	i := int(sendMsg(p.c[pkList], LB_GETCURSEL, 0, 0))
	if i < 0 || i >= len(p.disks) {
		return
	}
	d, done := p.disks[i], p.done
	p.close()
	done(d)
}

func pickProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	p := a.pick
	switch msg {
	case WM_SIZE:
		if p != nil && len(p.c) > 0 {
			p.layout()
		}
		return 0
	case WM_COMMAND:
		if p == nil {
			return 0
		}
		switch int(loword(wParam)) {
		case pkRefresh:
			p.refresh()
		case pkOK, 1:
			if hiword(wParam) == 0 {
				p.ok()
			}
		case pkList:
			if hiword(wParam) == 2 { // LBN_DBLCLK
				p.ok()
			}
		case pkCancel, IDCANCEL:
			if hiword(wParam) == 0 {
				p.close()
			}
		}
		return 0
	case WM_CLOSE:
		if p != nil {
			p.close()
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// ---- progress window and card jobs ----

const (
	wmJobProgress = WM_APP + 2
	wmJobDone     = WM_APP + 3
)

type job struct {
	hwnd   uintptr
	text   uintptr
	what   string
	mu     sync.Mutex
	phase  int
	done   int64
	total  int64
	err    error
	finish func(err error)
}

func (a *app) startJob(what string, run func(prog Progress) error, finish func(err error)) {
	j := &job{what: what, finish: finish}
	a.job = j
	j.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyDOSDeployJob", appTitle(), 0x00C00000|0x00080000, cwDefault, cwDefault, a.s(460), a.s(130), a.hwnd, 0) // WS_CAPTION|WS_SYSMENU
	j.text = createWindow(0, "STATIC", what, WS_CHILD|WS_VISIBLE, a.s(16), a.s(18), a.s(420), a.s(60), j.hwnd, 10)
	sendMsg(j.text, WM_SETFONT, a.font, 1)
	enable(a.hwnd, false)
	if a.boot != nil {
		enable(a.boot.hwnd, false)
	}
	pShowWindow.Call(j.hwnd, SW_SHOW)
	go func() {
		last := int64(-1)
		err := run(func(phase int, done, total int64) {
			pct := int64(phase)*1000 + done*100/max64(total, 1)
			if pct == last {
				return
			}
			last = pct
			j.mu.Lock()
			j.phase, j.done, j.total = phase, done, total
			j.mu.Unlock()
			postMsg(j.hwnd, wmJobProgress, 0, 0)
		})
		j.mu.Lock()
		j.err = err
		j.mu.Unlock()
		postMsg(j.hwnd, wmJobDone, 0, 0)
	}()
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func jobProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	j := a.job
	switch msg {
	case wmJobProgress:
		if j != nil {
			j.mu.Lock()
			ph, d, t := j.phase, j.done, j.total
			j.mu.Unlock()
			if ph == phaseFiles || ph == phaseMenu || ph == phaseOwn {
				step := T("Copying files: %d / %d", d, t)
				switch ph {
				case phaseMenu:
					step = T("Writing the boot menu and the drivers...")
				case phaseOwn:
					step = T("Copying your files: %d / %d", d, t)
				}
				setText(j.text, j.what+"\n\n"+step)
				return 0
			}
			step := T("Reading")
			switch ph {
			case phaseWrite:
				step = T("Writing (empty areas are skipped)")
			case phaseVerify:
				step = T("Checking (reading back)")
			}
			setText(j.text, fmt.Sprintf("%s\n\n%s: %s / %s  (%d%%)", j.what, step, humanSize(d), humanSize(t), d*100/max64(t, 1)))
		}
		return 0
	case wmJobDone:
		if j != nil {
			j.mu.Lock()
			err := j.err
			j.mu.Unlock()
			enable(a.hwnd, true)
			if a.boot != nil {
				enable(a.boot.hwnd, true)
			}
			pDestroyWindow.Call(hwnd)
			a.job = nil
			j.finish(err)
		}
		return 0
	case WM_CLOSE:
		return 0 // cannot be cancelled half way
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// adminCheck handles "access denied": offers to restart as administrator.
func (a *app) adminCheck(err error) bool {
	if !needAdmin(err) {
		return false
	}
	if msgBox(a.hwnd, T("Reading and writing a card directly needs administrator rights.\n\nRestart the program as administrator? Your project is kept."), appTitle(), MB_YESNO|MB_ICONQUESTION) != IDYES {
		return true
	}
	a.commit()
	a.readSettingsQuiet()
	a.captureBuild()
	tmp := filepath.Join(os.TempDir(), "easydosdeploy-restore.bmp.json")
	if a.proj.Save(tmp) != nil {
		tmp = ""
	}
	args := ""
	if tmp != "" {
		args = `--restore "` + tmp + `"`
		if a.path != "" {
			args += ` --path "` + a.path + `"`
		}
	}
	if runAsAdmin(args) {
		a.dirty = false
		pDestroyWindow.Call(a.hwnd)
	}
	return true
}

func (a *app) backupCard() {
	a.pickCard(T("Back up a disk to an image file"), func(d PhysDisk) {
		path := fileDialog(a.hwnd, true, T("Save card backup"), "img", "card-backup.img", imgFilter()...)
		if path == "" {
			return
		}
		a.startJob(T("Reading %s...", d.Label()), func(prog Progress) error {
			return ReadDisk(d, path, prog)
		}, func(err error) {
			if err != nil {
				if !a.adminCheck(err) {
					msgBox(a.hwnd, T("The backup failed:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
				}
				return
			}
			if msgBox(a.hwnd, T("Backup saved: %s\n\nRead the card geometry and system from it now (New bootable disk image)?", path), appTitle(), MB_YESNO|MB_ICONQUESTION) == IDYES {
				a.openBootWin()
				a.boot.applyDiskInfo(path)
			}
		})
	})
}

func (a *app) writeCard(image string) {
	if image == "" {
		image = fileDialog(a.hwnd, false, T("Image to write to the card"), "", "", imgFilter()...)
		if image == "" {
			return
		}
	}
	st, err := os.Stat(image)
	if err != nil {
		msgBox(a.hwnd, err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	a.pickCard(T("Write image to disk"), func(d PhysDisk) {
		if st.Size() > d.Size+512 {
			msgBox(a.hwnd, T("The image (%s) is bigger than the card (%s).", humanSize(st.Size()), humanSize(d.Size)), appTitle(), MB_ICONERROR)
			return
		}
		parts, err := InspectPhysDisk(d)
		if err != nil {
			if !a.adminCheck(err) {
				msgBox(a.hwnd, T("Could not read it:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
			}
			return
		}
		now, modern := DescribeParts(parts)
		text := T("EVERYTHING on this disk will be overwritten, including its partition table:\n\n%s\n\nIt holds now:\n%s\n\nImage: %s (%s)\n\nAreas that are empty in the image are skipped; everything written is read back and checked.",
			d.Label(), now, image, humanSize(st.Size()))
		if modern {
			text = T("WARNING: this disk has a modern partition (Windows, Linux or GPT). It is almost certainly NOT the disk of your retro PC.") + "\n\n" + text
		}
		a.confirmTyped(text, strconv.Itoa(d.Number), modern, func() {
			a.runWrite(d, image)
		})
	})
}

func (a *app) runWrite(d PhysDisk, image string) {
	var res *WriteResult
	a.startJob(T("Writing to %s...", d.Label()), func(prog Progress) error {
		var err error
		res, err = WriteDisk(d, image, true, true, prog)
		return err
	}, func(err error) {
		if err != nil {
			if !a.adminCheck(err) {
				msgBox(a.hwnd, T("Writing the card failed:")+"\n"+err.Error()+"\n\n"+T("The card may now be unusable until it is written again."), appTitle(), MB_ICONERROR)
			}
			return
		}
		msgBox(a.hwnd, T("The card is ready. Written: %s, skipped as empty: %s, read back and checked.\n\nPut it in the target machine; the BIOS settings must be the same as when the geometry was taken.",
			humanSize(res.Written), humanSize(res.Skipped)), appTitle(), MB_ICONINFORMATION)
	})
}

// ---- confirmation by typing ----

const (
	cfText = 10 + iota
	cfLbl
	cfEdit
	cfOK
	cfCancel
)

type confirmWin struct {
	hwnd   uintptr
	c      map[int]uintptr
	want   string
	onOK   func()
	closed bool
}

// confirmTyped asks for a dangerous action; OK only works after the user
// types want (the disk number).
func (a *app) confirmTyped(text, want string, alarm bool, onOK func()) {
	cw := &confirmWin{c: map[int]uintptr{}, want: want, onOK: onOK}
	a.confirm = cw
	title := T("Overwrite disk")
	if alarm {
		title = "!!! " + title + " !!!"
	}
	cw.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyDOSDeployConfirm", title, 0x00C00000|0x00080000, cwDefault, cwDefault, a.s(600), a.s(470), a.hwnd, 0)
	mk := func(id int, class, t string, style, ex uint32) uintptr {
		h := createWindow(ex, class, t, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, cw.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		cw.c[id] = h
		return h
	}
	mk(cfText, "EDIT", strings.ReplaceAll(text, "\n", "\r\n"), ES_MULTILINE|ES_READONLY|ES_AUTOVSCROLL|WS_VSCROLL, WS_EX_CLIENTEDGE)
	mk(cfLbl, "STATIC", T("To confirm, type the disk number (%s):", want), 0, 0)
	mk(cfEdit, "EDIT", "", WS_TABSTOP|ES_AUTOHSCROLL, WS_EX_CLIENTEDGE)
	mk(cfOK, "BUTTON", T("Overwrite"), WS_TABSTOP, 0)
	mk(cfCancel, "BUTTON", T("Cancel"), WS_TABSTOP|BS_DEFPUSHBUTTON, 0)
	w, h := clientSize(cw.hwnd)
	s, m := a.s, a.s(12)
	move(cw.c[cfText], m, m, w-2*m, h-3*m-s(28)-s(60))
	move(cw.c[cfLbl], m, h-2*m-s(28)-s(50), w-2*m, s(18))
	move(cw.c[cfEdit], m, h-2*m-s(28)-s(28), s(120), s(23))
	bw := fitW(s(110), s(24), cw.c[cfOK], cw.c[cfCancel])
	move(cw.c[cfOK], w-m-2*bw-s(8), h-m-s(28), bw, s(28))
	move(cw.c[cfCancel], w-m-bw, h-m-s(28), bw, s(28))
	enable(cw.c[cfOK], false)
	enable(a.hwnd, false)
	if a.boot != nil {
		enable(a.boot.hwnd, false)
	}
	pShowWindow.Call(cw.hwnd, SW_SHOW)
	pSetFocus.Call(cw.c[cfEdit])
}

func (cw *confirmWin) close(ok bool) {
	a := theApp
	if cw.closed {
		return
	}
	cw.closed = true
	enable(a.hwnd, true)
	if a.boot != nil {
		enable(a.boot.hwnd, true)
	}
	pDestroyWindow.Call(cw.hwnd)
	a.confirm = nil
	if ok {
		cw.onOK()
	}
}

func confirmProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	cw := a.confirm
	switch msg {
	case WM_COMMAND:
		if cw == nil {
			return 0
		}
		switch int(loword(wParam)) {
		case cfEdit:
			if hiword(wParam) == EN_CHANGE {
				enable(cw.c[cfOK], strings.TrimSpace(getText(cw.c[cfEdit])) == cw.want)
			}
		case cfOK:
			if strings.TrimSpace(getText(cw.c[cfEdit])) == cw.want {
				cw.close(true)
			}
		case cfCancel, IDCANCEL, 1:
			if hiword(wParam) == 0 {
				cw.close(false)
			}
		}
		return 0
	case WM_CLOSE:
		if cw != nil {
			cw.close(false)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// ---- geometry from BIOS setup values ----

const (
	gcC = 10 + iota
	gcH
	gcS
	gcMode
	gcResult
	gcOK
	gcCancel
	gcHelp
	gcLblC
	gcLblH
	gcLblS
	gcLblMode
	gcProfileHelp
)

type calcWin struct {
	hwnd uintptr
	c    map[int]uintptr
}

func (a *app) openCalc() {
	if a.calc != nil || a.boot == nil {
		return
	}
	w := &calcWin{c: map[int]uintptr{}}
	a.calc = w
	w.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyDOSDeployCalc", T("Geometry from BIOS setup"), 0x00C00000|0x00080000, cwDefault, cwDefault, a.s(600), a.s(440), a.boot.hwnd, 0)
	mk := func(id int, class, text string, style, ex uint32) uintptr {
		h := createWindow(ex, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, w.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		w.c[id] = h
		return h
	}
	edit := uint32(WS_TABSTOP | ES_AUTOHSCROLL)
	mk(gcHelp, "STATIC", T("Put the card or disk into the target machine, let BIOS setup detect it (Auto), and copy the values it shows. The program works out what DOS will get. To be sure, check it with GEOM on the target machine (see Help with the disk profile)."), 0, 0)
	mk(gcProfileHelp, "BUTTON", T("Help with the disk profile..."), WS_TABSTOP, 0)
	mk(gcLblC, "STATIC", T("Cylinders:"), 0, 0)
	mk(gcC, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(gcLblH, "STATIC", T("Heads:"), 0, 0)
	mk(gcH, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(gcLblS, "STATIC", T("Sectors/track:"), 0, 0)
	mk(gcS, "EDIT", "63", edit, WS_EX_CLIENTEDGE)
	mk(gcLblMode, "STATIC", T("Mode:"), 0, 0)
	mk(gcMode, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, 0)
	mk(gcResult, "STATIC", "", 0, 0)
	mk(gcOK, "BUTTON", T("Use these values"), WS_TABSTOP|BS_DEFPUSHBUTTON, 0)
	mk(gcCancel, "BUTTON", T("Cancel"), WS_TABSTOP, 0)
	fillCombo(w.c[gcMode], TranslationModes(), ModeLBA)
	s := a.s
	cw, ch := clientSize(w.hwnd)
	m := s(14)
	move(w.c[gcHelp], m, m, cw-2*m, s(72))
	y := m + s(82)
	x := m
	for _, p := range []struct{ l, e, lw int }{{gcLblC, gcC, 70}, {gcLblH, gcH, 50}, {gcLblS, gcS, 100}} {
		lw := fitW(s(p.lw), s(6), w.c[p.l])
		move(w.c[p.l], x, y+s(4), lw, s(18))
		x += lw
		move(w.c[p.e], x, y, s(60), s(23))
		x += s(60) + s(14)
	}
	y += s(36)
	ml := fitW(s(70), s(6), w.c[gcLblMode])
	move(w.c[gcLblMode], m, y+s(4), ml, s(18))
	move(w.c[gcMode], m+ml, y, cw-2*m-ml, s(140))
	y += s(40)
	move(w.c[gcResult], m, y, cw-2*m, ch-y-m-s(38))
	ow := fitW(s(150), s(24), w.c[gcOK])
	cnw := fitW(s(100), s(24), w.c[gcCancel])
	move(w.c[gcOK], cw-m-ow-s(8)-cnw, ch-m-s(28), ow, s(28))
	hw := fitW(s(150), s(24), w.c[gcProfileHelp])
	move(w.c[gcProfileHelp], m, ch-m-s(28), hw, s(28))
	move(w.c[gcCancel], cw-m-cnw, ch-m-s(28), cnw, s(28))
	w.update()
	enable(a.boot.hwnd, false)
	pShowWindow.Call(w.hwnd, SW_SHOW)
	pSetFocus.Call(w.c[gcC])
}

func (w *calcWin) values() (Geometry, Geometry, bool) {
	setup := Geometry{atoiU(getText(w.c[gcC])), atoiU(getText(w.c[gcH])), atoiU(getText(w.c[gcS]))}
	if setup.C == 0 || setup.H == 0 || setup.S == 0 || setup.H > 255 || setup.S > 63 {
		return setup, Geometry{}, false
	}
	return setup, DOSGeometry(setup, w.mode()), true
}

func (w *calcWin) mode() int {
	return clampIdx(int(sendMsg(w.c[gcMode], CB_GETCURSEL, 0, 0)), len(TranslationModes()))
}

// result is what goes into the image window: the geometry DOS gets, with
// the image size; or the geometry it will get
// after the drive is set to type User with fewer cylinders (256 heads).
func (w *calcWin) result() (g Geometry, total int64, text string, ok bool) {
	setup, g, ok := w.values()
	if !ok {
		return g, 0, T("Type the cylinders, heads and sectors shown in BIOS setup."), false
	}
	text = T("DOS will most likely get: %d / %d / %d\nDisk size: %d sectors (%s)", g.C, g.H, g.S, setup.Sectors(), humanSize(setup.Sectors()*512))
	if g.H > 255 {
		c, ug, found := UserCylinders(setup, w.mode())
		if !found {
			return g, 0, text + "\n" + T("256 heads: MS-DOS cannot use this disk."), false
		}
		text += "\n" + T("256 heads: MS-DOS cannot use this. In BIOS setup, set the drive to type User with %d cylinders (heads and sectors unchanged); DOS then gets %d / %d / %d. \"Use these values\" takes this.", c, ug.C, ug.H, ug.S)
		if c > biosCylLimit {
			sg := DOSGeometry(Geometry{biosCylLimit, setup.H, setup.S}, w.mode())
			text += "\n" + T("Some old BIOSes cannot handle more than %d cylinders. If the drive is not found after this, use %d (DOS then gets %d / %d / %d).", biosCylLimit, biosCylLimit, sg.C, sg.H, sg.S)
		}
		return ug, ug.Sectors(), text, true
	}
	total = setup.Sectors()
	if p, err := PlanHDD(g); err == nil && len(p.Logical) > 0 {
		text += "\n" + T("The disk is bigger than one MS-DOS partition (2 GB): the rest becomes drives D:, E: ... in an extended partition.")
	}
	return g, total, text, true
}

func (w *calcWin) update() {
	_, _, text, ok := w.result()
	setText(w.c[gcResult], text)
	enable(w.c[gcOK], ok)
}

func (w *calcWin) close(use bool) {
	a := theApp
	g, total, _, ok := w.result()
	if a.boot != nil {
		enable(a.boot.hwnd, true)
		if use && ok {
			sendMsg(a.boot.c[bwKind], CB_SETCURSEL, uintptr(MediaHDD), 0)
			a.boot.setGeom(g, total)
		}
		pSetForegroundWindow.Call(a.boot.hwnd)
	}
	pDestroyWindow.Call(w.hwnd)
	a.calc = nil
}

func calcProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	w := a.calc
	switch msg {
	case WM_COMMAND:
		if w == nil {
			return 0
		}
		code := hiword(wParam)
		switch int(loword(wParam)) {
		case gcC, gcH, gcS:
			if code == EN_CHANGE {
				w.update()
			}
		case gcMode:
			if code == CBN_SELCHANGE {
				w.update()
			}
		case gcOK, 1:
			if code == 0 {
				if _, _, _, ok := w.result(); ok {
					w.close(true)
				}
			}
		case gcCancel, IDCANCEL:
			if code == 0 {
				w.close(false)
			}
		case gcProfileHelp:
			w.close(false)
			a.openGeomHelp()
		}
		return 0
	case WM_CLOSE:
		if w != nil {
			w.close(false)
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// ---- GEOM.COM ----

func (a *app) saveGeom() {
	p := fileDialog(a.hwnd, true, T("Save GEOM.COM"), "com", "GEOM.COM", T("DOS program (*.com)"), "*.com", T("All files"), "*.*")
	if p == "" {
		return
	}
	if err := os.WriteFile(p, geomCOM, 0644); err != nil {
		msgBox(a.hwnd, T("Could not save:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	msgBox(a.hwnd, T("Saved: %s\n\nRun GEOM on the target machine from a DOS boot floppy, with the card or disk in place; it shows the cylinders, heads and sectors to enter here.", p), appTitle(), MB_ICONINFORMATION)
}

func (a *app) geomToFloppy() {
	p := fileDialog(a.hwnd, false, T("Bootable DOS floppy image to put GEOM.COM on"), "", "", imgFilter()...)
	if p == "" {
		return
	}
	v, err := OpenImage(p, true)
	if err == nil {
		err = v.WriteFile("GEOM.COM", geomCOM)
		v.Close()
	}
	if err != nil {
		msgBox(a.hwnd, T("Did not work:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	msgBox(a.hwnd, T("GEOM.COM is now on %s.\n\nWrite the image to a floppy (WinImage, Greaseweazle), boot the target machine from it with the card or disk in place, and type GEOM.", filepath.Base(p)), appTitle(), MB_ICONINFORMATION)
}

// downloadFreeDOS fetches the FreeDOS kernel, FreeCOM and the tools the menu
// needs, then makes the image.
func (b *bootWin) downloadFreeDOS(ids []string) {
	theApp.fetchDrivers(b.hwnd, ids, b.create)
}

// fetchDrivers asks to download catalog entries, downloads them with a
// progress window, and calls then when all of them are here.
func (a *app) fetchDrivers(owner uintptr, ids []string, then func()) {
	var names []string
	for _, id := range ids {
		names = append(names, a.store.driverName(id))
	}
	if msgBox(owner, T("FreeDOS needs these files, which are not here yet:")+"\n\n• "+strings.Join(names, "\n• ")+"\n\n"+
		T("Download them now? They are kept in the drivers folder for next time."), appTitle(), MB_YESNO|MB_ICONQUESTION) != IDYES {
		return
	}
	store := a.store
	a.startJob(T("Downloading %d driver(s)...", len(ids)), func(Progress) error {
		var failed []string
		for _, id := range ids {
			if d := store.Find(id); d != nil {
				if r := store.Download(d); !r.OK {
					failed = append(failed, d.Name+": "+r.Message)
				}
			}
		}
		if len(failed) > 0 {
			return fmt.Errorf("%s", strings.Join(failed, "\n"))
		}
		return nil
	}, func(err error) {
		if a.boot != nil {
			a.boot.updateDrivers()
		}
		a.updateStatus()
		if err != nil {
			msgBox(owner, T("Failed:")+"\n"+err.Error()+"\n\n"+T("You can also browse for these files in the Drivers window."), appTitle(), MB_ICONERROR)
			return
		}
		then()
	})
}

// makeGeomFloppy makes a FreeDOS boot floppy that runs GEOM.COM at start.
func (a *app) makeGeomFloppy() {
	files, missing := FreeDOSFiles(nil, a.store)
	if len(missing) > 0 {
		a.fetchDrivers(a.hwnd, missing, a.makeGeomFloppy)
		return
	}
	p := fileDialog(a.hwnd, true, T("Save the geometry floppy image"), "img", "geom-freedos.img", imgFilter()...)
	if p == "" {
		return
	}
	r, err := MakeGeomFloppy(p, files)
	if err != nil {
		msgBox(a.hwnd, T("Could not create the image:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	_ = r
	msgBox(a.hwnd, T("The geometry floppy is ready: %s\n\nWrite it to a 1.44 MB floppy (WinImage, Greaseweazle) and boot the target machine from it with the card or disk in place. It shows the geometry by itself.", p), appTitle(), MB_ICONINFORMATION)
}

// ownInfo tells, kind by kind, what the new menu loads: the source's own
// driver or the program's.
func (b *bootWin) ownInfo() string {
	if b.machine == nil {
		if b.machineSrc == "" {
			return T("Choose a system source to see the drivers it uses.")
		}
		return T("No mouse, CD-ROM or sound driver was found on the system source.")
	}
	m := b.machine
	use := b.useOwn()
	var out []string
	line := func(kind string, g Group, programs string) {
		how := ""
		switch g.Found {
		case "rem":
			how = " " + T("(commented out on the source)")
		case "file":
			how = " " + T("(on the source, but not loaded there)")
		}
		switch {
		case !g.Empty() && use:
			out = append(out, kind+": "+g.Programs()+how)
		case !g.Empty():
			out = append(out, kind+": "+T("%s on the source, not used", g.Programs()))
		case programs != "":
			out = append(out, kind+": "+T("not on the source; the program's own (%s)", programs))
		}
	}
	line(T("Sound"), m.Sound, "SET BLASTER")
	line(T("Mouse"), m.Mouse, "CTMOUSE")
	line("CD-ROM", m.CDROM, "OAKCDROM, MSCDEX")
	line(T("Keyboard"), m.Keyboard, "")
	return strings.Join(out, "\n")
}

// showOwnLines shows the exact lines found on the source.
func (b *bootWin) showOwnLines() {
	m := b.machine
	if m == nil {
		return
	}
	var t []string
	add := func(kind string, g Group) {
		if g.Empty() {
			return
		}
		switch g.Found {
		case "rem":
			kind += " " + T("(commented out on the source)")
		case "file":
			kind += " " + T("(on the source, but not loaded there)")
		}
		t = append(t, kind+":")
		for _, l := range g.Config {
			t = append(t, "   CONFIG.SYS:   "+l)
		}
		for _, l := range g.Autoexec {
			t = append(t, "   AUTOEXEC.BAT: "+l)
		}
	}
	add(T("Sound"), m.Sound)
	add(T("Mouse"), m.Mouse)
	add("CD-ROM", m.CDROM)
	add(T("Keyboard"), m.Keyboard)
	if len(m.Other) > 0 {
		t = append(t, "", T("Also found, not taken over (network, tools):"))
		for _, l := range m.Other {
			t = append(t, "   "+l)
		}
	}
	if len(b.machineMiss) > 0 {
		t = append(t, "", T("Named, but not on the source: %s", strings.Join(b.machineMiss, ", ")))
	}
	t = append(t, "", T("High loading (LH, DEVICEHIGH, /L:) is left out here; each menu item adds it for its memory setting."))
	msgBox(b.hwnd, strings.Join(t, "\n"), T("Drivers of the system source"), MB_ICONINFORMATION)
}

// ---- the recipe: the pages' settings in the project ----

// settings reads the Machine and disk, System and Build pages.
func (b *bootWin) settings() *BuildSettings {
	o := b.options()
	bs := &BuildSettings{
		Kind:      o.Kind,
		Geom:      o.Geom,
		OnlyC:     o.OnlyC,
		System:    SystemNames[clampIdx(b.sysChoice, len(SystemNames))],
		Source:    strings.Trim(strings.TrimSpace(getText(b.c[bwSource])), `"`),
		CopyAll:   checked(b.c[bwCopyAll]),
		UseOwn:    checked(b.c[bwUseOwn]),
		WriteMenu: checked(b.c[bwWriteMenu]),
		AddGeom:   checked(b.c[bwAddGeom]),
		Label:     o.Label,
		Output:    o.Path,
	}
	if checked(b.c[bwAdvanced]) {
		bs.Total = o.Total
	}
	if b.setup != nil {
		opt := b.setupOpt
		bs.SetupDisks, bs.SetupOpt = append([]string(nil), b.setupPaths...), &opt
	}
	return bs
}

// applySettings puts a project's recipe onto the pages; it returns what
// could not be restored.
func (b *bootWin) applySettings(bs *BuildSettings) []string {
	if bs == nil {
		return nil
	}
	var notes []string
	sendMsg(b.c[bwKind], CB_SETCURSEL, uintptr(bs.Kind), 0)
	setCheck(b.c[bwAdvanced], bs.Total > 0)
	b.setGeom(bs.Geom, bs.Total)
	setCheck(b.c[bwOnlyC], bs.OnlyC)
	setText(b.c[bwLabel], bs.Label)
	setText(b.c[bwOut], bs.Output)
	setText(b.c[bwSource], bs.Source)
	setCheck(b.c[bwCopyAll], bs.CopyAll)
	setCheck(b.c[bwWriteMenu], bs.WriteMenu)
	setCheck(b.c[bwAddGeom], bs.AddGeom)
	k := sysSource
	for i, n := range SystemNames {
		if n == bs.System {
			k = i
		}
	}
	b.setup, b.setupPaths = nil, nil
	if bs.SetupOpt != nil {
		b.setupOpt = *bs.SetupOpt
	}
	if len(bs.SetupDisks) > 0 {
		if s, err := ReadSetupDisks(bs.SetupDisks); err == nil {
			b.setup, b.setupPaths = s, append([]string(nil), bs.SetupDisks...)
		} else if k == sysSetup {
			notes = append(notes, T("The Setup disks of the project cannot be read: %v", err))
		}
	}
	if k == sysSource && bs.Source != "" {
		if _, err := os.Stat(bs.Source); err != nil {
			notes = append(notes, T("The system source of the project is missing: %s", bs.Source))
		}
	}
	b.setSys(k)
	b.machineSrc = "\x00" // read the source's drivers again
	b.setupChanged()      // also follows the source and its drivers
	if b.machine != nil {
		setCheck(b.c[bwUseOwn], bs.UseOwn)
		b.applyOwn()
	}
	b.fillMenus(0)
	b.update()
	return notes
}

// starter sets up a starter card: the universal geometry and GEOM.COM.
func (b *bootWin) starter() {
	sendMsg(b.c[bwKind], CB_SETCURSEL, uintptr(MediaHDD), 0)
	b.setGeom(UniversalGeom, 0)
	sendMsg(b.c[bwProfile], CB_SETCURSEL, 0, 0) // the built-in starter profile
	setCheck(b.c[bwAddGeom], true)
	setCheck(b.c[bwOnlyC], false)
	b.update()
}
