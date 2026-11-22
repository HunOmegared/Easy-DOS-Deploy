//go:build windows

package main

import (
	"fmt"
	"strings"
	"unsafe"
)

// The start-up wizard: what does the user want to do?

const (
	bsCommandLink = 0x0000000E
	bcmSetNote    = 0x1609
)

const (
	wzHead = 10 + iota
	wzFloppy
	wzCard
	wzEmu
	wzExisting
	wzEditor
	wzShowStart
	// page 2: system and menu
	wzHead2
	wzLblDOS
	wzDOS // radio button: MS-DOS
	wzDOSFree
	wzDOSNote
	wzDOSFreeNote
	wzLblMenu
	wzMenu0 // radio buttons of the ready menus (len(wizMenus))
	wzMenu1
	wzMenu2
	wzMenu3
	wzMenuNote0
	wzMenuNote1
	wzMenuNote2
	wzMenuNote3
	wzBack
	wzNext
	// page 3: existing system
	wzHead3
	wzFromImage
	wzFromDir
	wzBack3
	// page 4: card - is there a working one?
	wzHead4
	wzHasCard
	wzHasBackup
	wzFreeDOS
	wzSetup
	wzBootFloppy
	wzBack4
	// every page: the steps and an introduction
	wzSide
	wzSub1
	wzSub2
	wzSub3
	wzSub4
	wzStarter // page 1: a starter card
)

type wizTask int

const (
	taskFloppy wizTask = iota
	taskCard
	taskEmu
	taskExisting
	taskStarter
)

// menu choices on page 2: template IDs without the -ms/-fd ending
var wizMenus = []string{"plain", "everyday", "games", "net"}

type wizardWin struct {
	hwnd uintptr
	c    map[int]uintptr
	task wizTask
	page int
	menu int // the ready menu chosen on page 2
}

func (a *app) wizardHwnd() uintptr {
	if a.wiz != nil {
		return a.wiz.hwnd
	}
	return 0
}

func (a *app) openWizard() {
	if a.wiz != nil {
		pSetForegroundWindow.Call(a.wiz.hwnd)
		return
	}
	w := &wizardWin{c: map[int]uintptr{}, menu: 1}
	a.wiz = w
	w.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyDOSDeployWizard", appTitle(), 0x00C00000|0x00080000, cwDefault, cwDefault, a.s(900), a.s(680), a.hwnd, 0)
	mk := func(id int, class, text string, style uint32, note string) uintptr {
		h := createWindow(0, class, text, WS_CHILD|style, 0, 0, 10, 10, w.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		if note != "" {
			sendMsg(h, bcmSetNote, 0, uintptr(unsafe.Pointer(u16(note))))
		}
		w.c[id] = h
		return h
	}
	head := func(id int, text string) { sendMsg(mk(id, "STATIC", text, 0, ""), WM_SETFONT, a.head, 1) }
	mk(wzSide, "LISTBOX", "", WS_VISIBLE|LBS_OWNERDRAWFIXED|LBS_HASSTRINGS|LBS_NOINTEGRALHEIGHT, "")
	enable(w.c[wzSide], false) // shows the way; the buttons move along it

	link := uint32(WS_TABSTOP | bsCommandLink)
	head(wzHead, T("What would you like to do?"))
	mk(wzSub1, "STATIC", T("Easy DOS Deploy builds a bootable DOS system for an old PC: on a CF card or an IDE disk, on a floppy, or as an image for an emulator. Choose what you would like to do; everything can be changed later in the main window."), 0, "")
	mk(wzCard, "BUTTON", T("Build a DOS system on a CF card or IDE disk"), link, T("The geometry of the target machine, the whole disk in DOS drives, the DOS, its drivers and the boot menu. Written with a card reader or a USB-IDE adapter."))
	mk(wzStarter, "BUTTON", T("Make a starter card that boots on almost any PC"), link, T("No floppy needed: a CF card of about 500 MB with a geometry every IDE BIOS can take, and GEOM.COM to find the machine's own geometry. Also a helper for old IDE disks."))
	mk(wzEmu, "BUTTON", T("Make a disk image for an emulator"), link, T("A hard disk image for 86Box or PCem."))
	mk(wzFloppy, "BUTTON", T("Make a bootable floppy"), link, T("A disk image you can write to a floppy with WinImage or Greaseweazle."))
	mk(wzExisting, "BUTTON", T("Change the boot menu of an existing system"), link, T("Read the menu from a disk image or a drive, edit it and write it back."))
	mk(wzEditor, "BUTTON", T("Just open the main window"), link, T("All steps on one screen: disk, system, drivers, boot menu and build."))
	mk(wzShowStart, "BUTTON", T("Show this at start"), WS_TABSTOP|BS_AUTOCHECKBOX, "")
	setCheck(w.c[wzShowStart], !a.set.HideWizard)

	head(wzHead2, T("DOS and boot menu"))
	mk(wzSub2, "STATIC", T("The boot menu is what DOS shows at start-up: you choose what is loaded, such as the memory manager, the mouse, the CD-ROM or the sound card. Choose a ready one; it can be edited later."), 0, "")
	sendMsg(mk(wzLblDOS, "STATIC", T("System"), 0, ""), WM_SETFONT, a.bold, 1)
	mk(wzDOS, "BUTTON", "MS-DOS 6.x", WS_TABSTOP|WS_GROUP|BS_RADIOBUTTON, "")
	mk(wzDOSNote, "STATIC", T("You need your own MS-DOS: a card that boots it, a backup of one, a boot floppy image or the install disks. The program cannot download it, because it belongs to Microsoft."), 0, "")
	mk(wzDOSFree, "BUTTON", "FreeDOS", BS_RADIOBUTTON, "")
	mk(wzDOSFreeNote, "STATIC", T("Free and MS-DOS compatible; most DOS programs and games run on it. The program can download it, so an empty card is enough."), 0, "")
	setCheck(w.c[wzDOS], true)
	sendMsg(mk(wzLblMenu, "STATIC", T("Boot menu"), 0, ""), WM_SETFONT, a.bold, 1)
	for i, t := range []string{T("Plain DOS"), T("Everyday: DOS / + mouse and CD-ROM / everything"), T("Games: everything / EMS / no EMS / clean"), T("Network with Easy Commander / plain DOS")} {
		style := uint32(BS_RADIOBUTTON)
		if i == 0 {
			style |= WS_TABSTOP | WS_GROUP
		}
		mk(wzMenu0+i, "BUTTON", t, style, "")
		mk(wzMenuNote0+i, "STATIC", "", 0, "")
	}
	mk(wzBack, "BUTTON", T("< Back"), WS_TABSTOP, "")
	mk(wzNext, "BUTTON", T("Next >"), WS_TABSTOP|BS_DEFPUSHBUTTON, "")

	head(wzHead3, T("Where is the system whose menu you want to change?"))
	mk(wzSub3, "STATIC", T("The program reads its CONFIG.SYS and AUTOEXEC.BAT into the boot menu step. After editing, write the menu back from there (Write to disk image or Save to folder)."), 0, "")
	mk(wzFromImage, "BUTTON", T("In a disk image"), link, T("A floppy or hard disk image, a card backup or a fixed-size VHD."))
	mk(wzFromDir, "BUTTON", T("On a drive or in a folder"), link, T("For example the CF card in the card reader, by its drive letter."))
	mk(wzBack3, "BUTTON", T("< Back"), WS_TABSTOP, "")

	head(wzHead4, "") // worded for the DOS of page 2 (setSourceTexts)
	mk(wzSub4, "STATIC", "", 0, "")
	mk(wzHasCard, "BUTTON", T("A card or disk that already boots DOS on that machine"), link, T("Back it up first: geometry, boot code and system are taken from it (recommended)."))
	mk(wzHasBackup, "BUTTON", T("A backup of such a card (an image file)"), link, T("Made earlier with this program or another tool. For a different card, change the geometry afterwards."))
	mk(wzFreeDOS, "BUTTON", T("Nothing yet: FreeDOS, downloaded by the program"), link, T("For an empty card: the program downloads FreeDOS (kernel and FreeCOM, GNU GPL) and makes a bootable system."))
	mk(wzSetup, "BUTTON", "", link, "")
	mk(wzBootFloppy, "BUTTON", "", link, "")
	mk(wzBack4, "BUTTON", T("< Back"), WS_TABSTOP, "")

	w.layout()
	w.show(1)
	enable(a.hwnd, false)
	pShowWindow.Call(w.hwnd, SW_SHOW)
}

// steps are the names of the wizard's steps for the chosen task.
func (w *wizardWin) steps() []string {
	first := T("What to do")
	rest := T("Main window: the rest")
	switch {
	case w.page == 1:
		return []string{first, T("The next steps depend on it")}
	case w.task == taskExisting:
		return []string{first, T("Where the system is"), rest}
	case w.task == taskCard || w.task == taskStarter:
		return []string{first, T("DOS and boot menu"), T("Where DOS comes from"), rest}
	}
	return []string{first, T("DOS and boot menu"), rest}
}

// stepIndex is the place of the current page among the steps.
func (w *wizardWin) stepIndex() int {
	switch w.page {
	case 1:
		return 0
	case 4:
		return 2
	}
	return 1
}

func (w *wizardWin) fillSide() {
	h := w.c[wzSide]
	sendMsg(h, LB_RESETCONTENT, 0, 0)
	for _, t := range w.steps() {
		sendMsg(h, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(t))))
	}
}

// drawStep draws a step of the wizard: done, current or to come.
func (w *wizardWin) drawStep(di *drawItem) {
	a := theApp
	steps := w.steps()
	if int(di.itemID) >= len(steps) {
		return
	}
	i, cur := int(di.itemID), w.stepIndex()
	bg, _, _ := pGetSysColorBrush.Call(COLOR_WINDOW)
	col := rgbColor(0x80, 0x80, 0x80)
	font := a.font
	if i == cur {
		bg, _, _ = pCreateSolidBrush.Call(uintptr(rgbColor(0xE6, 0xF1, 0xFB)))
		defer pDeleteObject.Call(bg)
		col, font = rgbColor(0x0C, 0x44, 0x7C), a.bold
	} else if i < cur {
		col = rgbColor(0x20, 0x20, 0x20)
	}
	pFillRect.Call(di.hdc, uintptr(unsafe.Pointer(&di.rc)), bg)
	is := a.s(24)
	x := int(di.rc.left) + a.s(12)
	h := int(di.rc.bottom - di.rc.top)
	switch {
	case i < cur:
		pImageListDraw.Call(a.sideIcons, iconOK, di.hdc, uintptr(x), uintptr(int(di.rc.top)+(h-is)/2), 0)
	case i == cur:
		pImageListDraw.Call(a.sideIcons, iconBuild, di.hdc, uintptr(x), uintptr(int(di.rc.top)+(h-is)/2), 0)
	}
	pSetBkMode.Call(di.hdc, TRANSPARENT)
	old, _, _ := pSelectObject.Call(di.hdc, font)
	pSetTextColor.Call(di.hdc, uintptr(col))
	r := rect{int32(x + is + a.s(10)), di.rc.top, di.rc.right - int32(a.s(6)), di.rc.bottom}
	text := fmt.Sprintf("%d. %s", i+1, steps[i])
	pDrawTextW.Call(di.hdc, uintptr(unsafe.Pointer(u16(text))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX|DT_END_ELLIPSIS)
	pSelectObject.Call(di.hdc, old)
}

func (w *wizardWin) layout() {
	a := theApp
	s := a.s
	cw, ch := clientSize(w.hwnd)
	side := s(230)
	move(w.c[wzSide], 0, 0, side, ch)
	m := s(20)
	x0 := side + m
	lw := cw - x0 - m
	bw := fitW(s(110), s(24), w.c[wzBack], w.c[wzNext])
	by := ch - m - s(30)
	// heading and introduction of a page; returns the first free y
	top := func(head, sub int) int {
		move(w.c[head], x0, m, lw, s(30))
		move(w.c[sub], x0, m+s(36), lw, s(54))
		return m + s(36) + s(64)
	}

	y := top(wzHead, wzSub1)
	for _, id := range []int{wzCard, wzStarter, wzEmu, wzFloppy, wzExisting, wzEditor} {
		move(w.c[id], x0, y, lw, s(64))
		y += s(68)
	}
	move(w.c[wzShowStart], x0, by+s(4), fitW(s(300), s(28), w.c[wzShowStart]), s(24))

	y = top(wzHead2, wzSub2)
	in := x0 + s(22)
	move(w.c[wzLblDOS], x0, y, lw, s(20))
	y += s(24)
	for _, r := range [][2]int{{wzDOS, wzDOSNote}, {wzDOSFree, wzDOSFreeNote}} {
		move(w.c[r[0]], x0, y, lw, s(22))
		move(w.c[r[1]], in, y+s(22), lw-s(22), s(36))
		y += s(62)
	}
	y += s(6)
	move(w.c[wzLblMenu], x0, y, lw, s(20))
	y += s(24)
	for i := range wizMenus {
		move(w.c[wzMenu0+i], x0, y, lw, s(22))
		move(w.c[wzMenuNote0+i], in, y+s(22), lw-s(22), s(36))
		y += s(62)
	}
	move(w.c[wzBack], cw-m-2*bw-s(8), by, bw, s(30))
	move(w.c[wzNext], cw-m-bw, by, bw, s(30))

	y = top(wzHead3, wzSub3)
	move(w.c[wzFromImage], x0, y, lw, s(68))
	move(w.c[wzFromDir], x0, y+s(76), lw, s(68))
	move(w.c[wzBack3], cw-m-bw, by, bw, s(30))

	y = top(wzHead4, wzSub4)
	for _, id := range w.sourceChoices() {
		move(w.c[id], x0, y, lw, s(68))
		y += s(72)
	}
	move(w.c[wzBack4], cw-m-bw, by, bw, s(30))
}

// freeDOS is true when FreeDOS is chosen on page 2.
func (w *wizardWin) freeDOS() bool { return checked(w.c[wzDOSFree]) }

// sourceChoices are the "where should DOS come from" choices for the DOS of
// page 2, in order: FreeDOS can be downloaded, MS-DOS cannot.
func (w *wizardWin) sourceChoices() []int {
	if w.freeDOS() {
		return []int{wzFreeDOS, wzHasCard, wzHasBackup, wzSetup, wzBootFloppy}
	}
	return []int{wzSetup, wzHasCard, wzHasBackup, wzBootFloppy}
}

// setSourceTexts words page 4 for the chosen DOS.
func (w *wizardWin) setSourceTexts() {
	link := func(id int, title, note string) {
		setText(w.c[id], title)
		sendMsg(w.c[id], bcmSetNote, 0, uintptr(unsafe.Pointer(u16(note))))
	}
	setup := T("An empty image with the right geometry; install DOS on it in 86Box with its own Setup.")
	if w.freeDOS() {
		setText(w.c[wzHead4], T("Where should FreeDOS come from?"))
		setText(w.c[wzSub4], T("The program can download FreeDOS, so an empty card is enough. If you already have a FreeDOS system, it can be used instead."))
		link(wzSetup, T("FreeDOS install disks or CD"), setup)
		link(wzBootFloppy, T("A FreeDOS boot floppy image"), T("A small bootable system from the floppy: its system files and tools are copied."))
	} else {
		setText(w.c[wzHead4], T("Where should MS-DOS come from?"))
		setText(w.c[wzSub4], T("MS-DOS belongs to Microsoft, so it comes from your own disks or from a system that already works. The Setup disks are the easiest way for a new system."))
		link(wzSetup, T("MS-DOS Setup disks (disk images)"), T("The program installs MS-DOS from the images of your Setup disks into C:\\DOS, as Setup would."))
		link(wzBootFloppy, T("An MS-DOS boot floppy image"), T("A small bootable system from the floppy: its system files and tools are copied."))
	}
}

func (w *wizardWin) show(page int) {
	w.page = page
	if page == 4 {
		w.setSourceTexts()
		w.layout()
	}
	menu := []int{wzLblMenu, wzMenu0, wzMenu1, wzMenu2, wzMenu3, wzMenuNote0, wzMenuNote1, wzMenuNote2, wzMenuNote3}
	pages := map[int][]int{
		1: {wzHead, wzSub1, wzFloppy, wzCard, wzStarter, wzEmu, wzExisting, wzEditor, wzShowStart},
		2: append([]int{wzHead2, wzSub2, wzLblDOS, wzDOS, wzDOSNote, wzDOSFree, wzDOSFreeNote, wzBack, wzNext}, menu...),
		3: {wzHead3, wzSub3, wzFromImage, wzFromDir, wzBack3},
		4: append([]int{wzHead4, wzSub4, wzBack4}, w.sourceChoices()...),
	}
	if page != 4 {
		pShowWindow.Call(w.c[wzFreeDOS], 0) // not on every page 4
	}
	for p, ids := range pages {
		for _, id := range ids {
			v := uintptr(0)
			if p == page {
				v = SW_SHOW
			}
			pShowWindow.Call(w.c[id], v)
		}
	}
	if page == 2 {
		w.describe()
	}
	w.fillSide()
}

func (w *wizardWin) templateID() string {
	suffix := "-ms"
	if w.freeDOS() {
		suffix = "-fd"
	}
	return wizMenus[clampIdx(w.menu, len(wizMenus))] + suffix
}

// describe shows what each ready menu offers, for the chosen DOS.
func (w *wizardWin) describe() {
	suffix := "-ms"
	if w.freeDOS() {
		suffix = "-fd"
	}
	for i, id := range wizMenus {
		t := TemplateByID(id + suffix)
		var titles []string
		for _, it := range t.Make().Items {
			titles = append(titles, it.Title)
		}
		setText(w.c[wzMenuNote0+i], T(t.Desc)+"\n"+T("Menu items:")+" "+strings.Join(titles, " • "))
		setCheck(w.c[wzMenu0+i], i == w.menu)
	}
}

func (w *wizardWin) close() {
	a := theApp
	hide := !checked(w.c[wzShowStart])
	if hide != a.set.HideWizard {
		a.set.HideWizard = hide
		saveSettings(a.data, a.set)
	}
	enable(a.hwnd, true)
	pDestroyWindow.Call(w.hwnd)
	a.wiz = nil
	pSetForegroundWindow.Call(a.hwnd)
}

// next applies the chosen DOS and menu, then goes on with the task.
func (w *wizardWin) next() {
	a := theApp
	task := w.task
	tid := w.templateID()
	if a.menuEdited && !a.askSave() {
		return
	}
	a.proj, a.path, a.dirty, a.sel = TemplateByID(tid).Make(), "", false, 1
	a.setMenuFrom(tid)
	a.loadProject()
	if task == taskCard || task == taskStarter {
		w.show(4)
		return
	}
	fd := w.freeDOS()
	w.close()
	switch task {
	case taskFloppy:
		a.openBootWinWith(MediaF144, nil)
	case taskEmu:
		// a geometry every BIOS and emulator agrees on, about 500 MB
		a.openBootWinWith(MediaHDD, &Geometry{1015, 16, 63})
	}
	if fd && a.boot != nil {
		// FreeDOS needs no source: the program downloads it
		a.boot.setSys(sysFree)
		a.boot.update()
		a.boot.followSource()
	}
}

func wizardProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	w := a.wiz
	starter := w != nil && w.task == taskStarter
	switch msg {
	case WM_MEASUREITEM:
		if mi := *(**measureItem)(unsafe.Pointer(&lParam)); mi.ctlID == wzSide {
			mi.itemHeight = uint32(a.s(46))
			return 1
		}
	case WM_DRAWITEM:
		if di := *(**drawItem)(unsafe.Pointer(&lParam)); di.ctlID == wzSide && w != nil {
			w.drawStep(di)
			return 1
		}
	case WM_COMMAND:
		if w == nil {
			return 0
		}
		code := hiword(wParam)
		switch int(loword(wParam)) {
		case wzFloppy, wzCard, wzEmu:
			w.task = wizTask(int(loword(wParam)) - wzFloppy)
			w.show(2)
		case wzStarter:
			w.task = taskStarter
			w.show(2)
		case wzExisting:
			w.task = taskExisting
			w.show(3)
		case wzEditor:
			w.close()
			a.showPage(pageDisk)
		case wzBack, wzBack3:
			w.show(1)
		case wzBack4:
			w.show(2)
		case wzHasCard:
			w.close()
			a.backupCard()
		case wzHasBackup:
			w.close()
			a.openBootWinWith(MediaHDD, nil)
			if starter {
				a.boot.starter()
			}
			if p := fileDialog(a.boot.hwnd, false, T("Card backup or hard disk image"), "", "", imgFilter()...); p != "" {
				setCheck(a.boot.c[bwCopyAll], true)
				a.boot.applyDiskInfo(p)
			}
		case wzFreeDOS:
			w.close()
			a.openBootWinWith(MediaHDD, nil)
			if starter {
				a.boot.starter()
			}
			a.boot.setSys(sysFree)
			a.boot.update()
			a.boot.followSource()
		case wzSetup:
			fd := w.freeDOS()
			w.close()
			a.openBootWinWith(MediaHDD, nil)
			if starter {
				a.boot.starter()
			}
			if fd {
				a.boot.setSys(sysBlank)
				a.boot.update()
				break
			}
			// MS-DOS: the program installs it from the disks
			a.boot.setSys(sysSetup)
			a.openSetupDisks()
		case wzBootFloppy:
			w.close()
			a.openBootWinWith(MediaHDD, nil)
			if starter {
				a.boot.starter()
			}
			if p := fileDialog(a.boot.hwnd, false, T("DOS boot floppy image"), "", "", imgFilter()...); p != "" {
				a.boot.setSys(sysSource)
				setText(a.boot.c[bwSource], p)
				setCheck(a.boot.c[bwCopyAll], true)
				a.boot.followSource()
			}
		case wzNext:
			w.next()
		case wzDOS, wzDOSFree:
			if code == BN_CLICKED {
				setCheck(w.c[wzDOS], int(loword(wParam)) == wzDOS)
				setCheck(w.c[wzDOSFree], int(loword(wParam)) == wzDOSFree)
				w.describe()
			}
		case wzMenu0, wzMenu1, wzMenu2, wzMenu3:
			if code == BN_CLICKED {
				w.menu = int(loword(wParam)) - wzMenu0
				w.describe()
			}
		case wzFromImage:
			w.close()
			a.readFromImage()
		case wzFromDir:
			w.close()
			a.readFromDir()
		case IDCANCEL:
			if code == 0 {
				w.close()
			}
		case 1: // Enter
			if code == 0 && w.page == 2 {
				w.next()
			}
		}
		return 0
	case WM_CLOSE:
		if w != nil {
			w.close()
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}
