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
	bwPlan
	bwSource
	bwSrcBrowse
	bwCopyAll
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
)

type bootWin struct {
	hwnd    uintptr
	c       map[int]uintptr
	loading bool
	srcPath string
	srcDOS  DOSKind
}

func (a *app) openBootWin() {
	if a.boot != nil {
		pShowWindow.Call(a.boot.hwnd, SW_SHOW)
		pSetForegroundWindow.Call(a.boot.hwnd)
		return
	}
	b := &bootWin{c: map[int]uintptr{}}
	a.boot = b
	b.hwnd = createWindow(0, "EasyRSBBoot", T("New bootable disk image"), WS_OVERLAPPEDWINDOW&^0x00050000, cwDefault, cwDefault, a.s(700), a.s(650), 0, 0) // no owner, so it does not stay above the main window
	mk := func(id int, class, text string, style, ex uint32) uintptr {
		h := createWindow(ex, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, b.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		b.c[id] = h
		return h
	}
	edit := uint32(WS_TABSTOP | ES_AUTOHSCROLL)
	mk(bwLblKind, "STATIC", T("Type:"), 0, 0)
	mk(bwKind, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, 0)
	mk(bwLblProfile, "STATIC", T("Card profile:"), 0, 0)
	mk(bwProfile, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWN|WS_VSCROLL|ES_AUTOHSCROLL, 0)
	mk(bwSaveProf, "BUTTON", T("Save profile"), WS_TABSTOP, 0)
	mk(bwDelProf, "BUTTON", T("Delete profile"), WS_TABSTOP, 0)
	mk(bwLblC, "STATIC", T("Cylinders:"), 0, 0)
	mk(bwC, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(bwLblH, "STATIC", T("Heads:"), 0, 0)
	mk(bwH, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(bwLblS, "STATIC", T("Sectors/track:"), 0, 0)
	mk(bwS, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(bwFromImg, "BUTTON", T("From a card backup..."), WS_TABSTOP, 0)
	mk(bwLblTotal, "STATIC", T("Image size in sectors (empty: cylinders × heads × sectors):"), 0, 0)
	mk(bwTotal, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(bwHelp, "STATIC", T("Enter the geometry DOS gets from the target machine's BIOS (MSD.EXE, Disk Drives), not the one in BIOS setup."), 0, 0)
	mk(bwPlan, "STATIC", "", 0, 0)
	mk(bwLblSource, "STATIC", T("System source: a bootable disk image or card backup (empty: not bootable):"), 0, 0)
	mk(bwSource, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(bwSrcBrowse, "BUTTON", T("Browse..."), WS_TABSTOP, 0)
	mk(bwCopyAll, "BUTTON", T("Copy all other files of the source too (e.g. the DOS folder)"), WS_TABSTOP|BS_AUTOCHECKBOX, 0)
	mk(bwLblMenu, "STATIC", T("Boot menu:"), 0, 0)
	mk(bwMenuKind, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, 0)
	mk(bwMenuEdit, "BUTTON", T("Edit..."), WS_TABSTOP, 0)
	mk(bwDrvInfo, "STATIC", "", 0, 0)
	mk(bwDrvBtn, "BUTTON", T("Drivers..."), WS_TABSTOP, 0)
	mk(bwLblLabel, "STATIC", T("Volume label:"), 0, 0)
	sendMsg(mk(bwLabel, "EDIT", "", edit|ES_UPPERCASE, WS_EX_CLIENTEDGE), EM_SETLIMIT, 11, 0)
	mk(bwLblOut, "STATIC", T("New image file:"), 0, 0)
	mk(bwOut, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(bwOutBrowse, "BUTTON", T("Browse..."), WS_TABSTOP, 0)
	mk(bwCreate, "BUTTON", T("Create"), WS_TABSTOP|BS_DEFPUSHBUTTON, 0)
	mk(bwClose, "BUTTON", T("Close"), WS_TABSTOP, 0)
	fillCombo(b.c[bwKind], MediaNames(), 0)
	b.fillProfiles("")
	b.fillMenus(menuEditor)
	b.layout()
	b.update()
	pShowWindow.Call(b.hwnd, SW_SHOW)
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

func (b *bootWin) fillMenus(sel int) {
	a := theApp
	var titles []string
	for _, it := range a.proj.Items {
		titles = append(titles, title(it))
	}
	items := []string{
		T("Plain DOS"),
		T("Everyday: DOS / + mouse and CD-ROM / everything"),
		T("Games: everything / EMS / no EMS / clean"),
		T("Network with Easy Commander / plain DOS"),
		T("The menu in the editor: %s", strings.Join(titles, "; ")),
		T("Keep the menu of the system source (write no menu)"),
	}
	fillCombo(b.c[bwMenuKind], items, sel)
	b.updateDrivers()
}

func (b *bootWin) menuChoice() int {
	return clampIdx(int(sendMsg(b.c[bwMenuKind], CB_GETCURSEL, 0, 0)), menuKeep+1)
}

// sourceTarget is the DOS of the system source, or the editor's.
func (b *bootWin) sourceTarget() Target {
	a := theApp
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

// menuChosen loads a ready menu into the editor.
func (b *bootWin) menuChosen() {
	a := theApp
	c := b.menuChoice()
	if c < menuEditor {
		if !a.askSave() {
			b.fillMenus(menuEditor)
			return
		}
		suffix := "-ms"
		if b.sourceTarget() == TargetFreeDOS {
			suffix = "-fd"
		}
		a.proj, a.path, a.dirty, a.sel = TemplateByID(wizMenus[c]+suffix).Make(), "", false, 1
		a.loadProject()
	}
	b.updateDrivers()
}

// followSource switches the editor's menu to the DOS of a new source.
func (b *bootWin) followSource() {
	a := theApp
	t := b.sourceTarget()
	if t != a.proj.Target {
		from := a.proj.Target
		a.proj.Target = t
		a.targetChanged(from)
		a.setDirty()
		a.loadProject()
	}
	b.fillMenus(b.menuChoice())
}

func (b *bootWin) updateDrivers() {
	a := theApp
	if b.menuChoice() == menuKeep {
		setText(b.c[bwDrvInfo], T("No menu and no drivers are written; the files of the source stay as they are."))
		enable(b.c[bwDrvBtn], false)
		return
	}
	enable(b.c[bwDrvBtn], true)
	miss := MissingDrivers(a.proj, a.store)
	switch {
	case len(a.proj.Drivers) == 0:
		setText(b.c[bwDrvInfo], T("This menu needs no extra drivers."))
	case len(miss) == 0:
		setText(b.c[bwDrvInfo], T("All %d drivers of this menu are here.", len(a.proj.Drivers)))
	default:
		setText(b.c[bwDrvInfo], T("Missing drivers: %s", strings.Join(miss, "; ")))
	}
}

func (b *bootWin) fillProfiles(sel string) {
	a := theApp
	cb := b.c[bwProfile]
	sendMsg(cb, CB_RESETCONTENT, 0, 0)
	idx := -1
	for i, p := range a.set.Profiles {
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
	w, h := clientSize(b.hwnd)
	s := a.s
	m := s(10)
	rh := s(23)
	lh := s(18)
	cw := w - 2*m
	y := m
	lbl := func(id, x, ww int) { move(b.c[id], x, y+s(4), ww, lh) }
	lbl(bwLblKind, m, s(110))
	move(b.c[bwKind], m+s(110), y, s(220), rh+s(160))
	y += rh + s(10)
	lbl(bwLblProfile, m, s(110))
	move(b.c[bwProfile], m+s(110), y, cw-s(110)-2*s(116), rh+s(160))
	move(b.c[bwSaveProf], w-m-2*s(110)-s(6), y, s(110), rh)
	move(b.c[bwDelProf], w-m-s(110), y, s(110), rh)
	y += rh + s(8)
	x := m
	for _, p := range []struct{ l, e, lw int }{{bwLblC, bwC, 70}, {bwLblH, bwH, 50}, {bwLblS, bwS, 100}} {
		move(b.c[p.l], x, y+s(4), s(p.lw), lh)
		x += s(p.lw)
		move(b.c[p.e], x, y, s(60), rh)
		x += s(60) + s(14)
	}
	move(b.c[bwFromImg], x, y, w-m-x, rh)
	y += rh + s(6)
	move(b.c[bwHelp], m, y, cw, 2*lh)
	y += 2*lh + s(4)
	move(b.c[bwLblTotal], m, y+s(4), cw-s(130), lh)
	move(b.c[bwTotal], w-m-s(120), y, s(120), rh)
	y += rh + s(6)
	move(b.c[bwPlan], m, y, cw, 2*lh)
	y += 2*lh + s(10)
	move(b.c[bwLblSource], m, y, cw, lh)
	y += lh + s(2)
	move(b.c[bwSource], m, y, cw-s(110), rh)
	move(b.c[bwSrcBrowse], w-m-s(100), y, s(100), rh)
	y += rh + s(6)
	move(b.c[bwCopyAll], m, y, cw, rh)
	y += rh
	y += s(6)
	move(b.c[bwLblMenu], m, y+s(4), s(110), lh)
	move(b.c[bwMenuKind], m+s(110), y, cw-s(110)-s(110), rh+s(160))
	move(b.c[bwMenuEdit], w-m-s(100), y, s(100), rh)
	y += rh + s(6)
	move(b.c[bwDrvInfo], m+s(110), y+s(4), cw-s(110)-s(110), 2*lh)
	move(b.c[bwDrvBtn], w-m-s(100), y, s(100), rh)
	y += 2*lh + s(10)
	move(b.c[bwLblLabel], m, y+s(4), s(110), lh)
	move(b.c[bwLabel], m+s(110), y, s(140), rh)
	y += rh + s(10)
	move(b.c[bwLblOut], m, y, cw, lh)
	y += lh + s(2)
	move(b.c[bwOut], m, y, cw-s(110), rh)
	move(b.c[bwOutBrowse], w-m-s(100), y, s(100), rh)
	move(b.c[bwCreate], w-m-2*s(110)-s(8), h-m-s(28), s(110), s(28))
	move(b.c[bwClose], w-m-s(110), h-m-s(28), s(110), s(28))
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
	}
	if t, err := strconv.ParseInt(strings.TrimSpace(getText(b.c[bwTotal])), 10, 64); err == nil {
		o.Total = t
	}
	return o
}

// update enables the hard disk fields and shows the partition plan.
func (b *bootWin) update() {
	if b.loading {
		return
	}
	o := b.options()
	hdd := o.Kind == MediaHDD
	for _, id := range []int{bwProfile, bwSaveProf, bwDelProf, bwC, bwH, bwS, bwTotal, bwFromImg} {
		enable(b.c[id], hdd)
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
	t := p.Describe()
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
	if total > 0 {
		setText(b.c[bwTotal], strconv.FormatInt(total, 10))
	} else {
		setText(b.c[bwTotal], "")
	}
	b.loading = false
	b.update()
}

func (b *bootWin) loadProfile() {
	a := theApp
	i := int(sendMsg(b.c[bwProfile], CB_GETCURSEL, 0, 0))
	if i < 0 || i >= len(a.set.Profiles) {
		return
	}
	p := a.set.Profiles[i]
	sendMsg(b.c[bwKind], CB_SETCURSEL, uintptr(MediaHDD), 0)
	b.setGeom(Geometry{p.C, p.H, p.S}, p.Total)
}

func (b *bootWin) saveProfile() {
	a := theApp
	name := strings.TrimSpace(getText(b.c[bwProfile]))
	o := b.options()
	if name == "" {
		msgBox(b.hwnd, T("Type a name for the profile into the profile box first (for example: Integra 486, 1 GB CF)."), appTitle(), MB_ICONINFORMATION)
		return
	}
	if _, err := PlanHDD(o.Geom); err != nil {
		msgBox(b.hwnd, err.Error(), appTitle(), MB_ICONWARNING)
		return
	}
	p := GeomProfile{Name: name, C: o.Geom.C, H: o.Geom.H, S: o.Geom.S, Total: o.Total}
	found := false
	for i := range a.set.Profiles {
		if a.set.Profiles[i].Name == name {
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
	i := int(sendMsg(b.c[bwProfile], CB_GETCURSEL, 0, 0))
	if i < 0 || i >= len(a.set.Profiles) {
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
		setText(b.c[bwSource], p)
		b.followSource()
	}
	msgBox(b.hwnd, T("Read from the image:\n\nGeometry (as FDISK recorded it): %d / %d / %d\nSize: %d sectors (%s)\nVolume: %s\nSystem: %s\n\nCheck that heads and sectors match what MSD shows on the target machine.",
		di.Geom.C, di.Geom.H, di.Geom.S, di.Total, humanSize(di.Total*512), di.Desc, di.DOS), appTitle(), MB_ICONINFORMATION)
}

func (b *bootWin) create() {
	a := theApp
	o := b.options()
	if o.Path == "" {
		msgBox(b.hwnd, T("Choose where to save the new image."), appTitle(), MB_ICONINFORMATION)
		return
	}
	if _, err := os.Stat(o.Path); err == nil {
		if msgBox(b.hwnd, T("%s already exists. Overwrite it?", o.Path), appTitle(), MB_YESNO|MB_ICONQUESTION) != IDYES {
			return
		}
	}
	if o.Source != "" && filepath.Clean(o.Source) == filepath.Clean(o.Path) {
		msgBox(b.hwnd, T("The system source and the new image must be different files."), appTitle(), MB_ICONWARNING)
		return
	}
	withMenu := b.menuChoice() != menuKeep
	if withMenu && (!a.ready() || !a.checkDrivers()) {
		return
	}
	r, err := CreateBootImage(o)
	if err != nil {
		msgBox(b.hwnd, T("Could not create the image:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	if withMenu {
		r2, err := WriteToImage(o.Path, a.proj, a.store, true)
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
		if err != nil {
			msgBox(b.hwnd, reportText(r, T("Error: %s", err.Error())), appTitle(), MB_ICONERROR)
			return
		}
	}
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
	case 0x0006: // WM_ACTIVATE
		if b != nil && len(b.c) > 0 && loword(wParam) != 0 {
			sel := b.menuChoice()
			if sel < menuEditor && a.dirty {
				sel = menuEditor // edited in the editor since
			}
			b.fillMenus(sel)
		}
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
		case bwC, bwH, bwS, bwTotal:
			if code == EN_CHANGE {
				b.update()
			}
		case bwMenuKind:
			if code == CBN_SELCHANGE {
				b.menuChosen()
			}
		case bwMenuEdit:
			if b.menuChoice() == menuKeep {
				b.fillMenus(menuEditor)
			}
			pSetForegroundWindow.Call(a.hwnd)
		case bwDrvBtn:
			a.openDrivers()
		case bwSaveProf:
			b.saveProfile()
		case bwDelProf:
			b.deleteProfile()
		case bwFromImg:
			b.fromImage()
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
		case bwClose:
			pShowWindow.Call(hwnd, 0)
		case IDCANCEL:
			if code == 0 {
				pShowWindow.Call(hwnd, 0)
			}
		}
		return 0
	case WM_CLOSE:
		pShowWindow.Call(hwnd, 0)
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
	p.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyRSBPick", title, WS_OVERLAPPEDWINDOW&^0x00050000, cwDefault, cwDefault, a.s(560), a.s(320), a.hwnd, 0)
	mk := func(id int, class, text string, style, ex uint32) {
		h := createWindow(ex, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, p.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		p.c[id] = h
	}
	mk(pkHead, "STATIC", T("Only removable, USB and SD disks are listed; the Windows disk never. Insert the card in the reader, then Refresh."), 0, 0)
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
	move(p.c[pkRefresh], m, h-m-bh, s(110), bh)
	move(p.c[pkOK], w-m-2*s(100)-s(8), h-m-bh, s(100), bh)
	move(p.c[pkCancel], w-m-s(100), h-m-bh, s(100), bh)
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
		sendMsg(lb, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(T("(no card found)")))))
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
	j.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyRSBJob", appTitle(), 0x00C00000|0x00080000, cwDefault, cwDefault, a.s(460), a.s(130), a.hwnd, 0) // WS_CAPTION|WS_SYSMENU
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
	tmp := filepath.Join(os.TempDir(), "easyrsb-restore.bmp.json")
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
	a.pickCard(T("Back up a card to an image file"), func(d PhysDisk) {
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
	a.pickCard(T("Write image to card"), func(d PhysDisk) {
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
	cw.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyRSBConfirm", title, 0x00C00000|0x00080000, cwDefault, cwDefault, a.s(600), a.s(470), a.hwnd, 0)
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
	move(cw.c[cfOK], w-m-2*s(110)-s(8), h-m-s(28), s(110), s(28))
	move(cw.c[cfCancel], w-m-s(110), h-m-s(28), s(110), s(28))
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
