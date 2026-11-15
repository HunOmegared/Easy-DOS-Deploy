//go:build windows

package main

import "unsafe"

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
	wzDOS
	wzLblMenu
	wzMenu
	wzMenuDesc
	wzBack
	wzNext
	// page 3: existing system
	wzHead3
	wzFromImage
	wzFromDir
	wzBack3
)

type wizTask int

const (
	taskFloppy wizTask = iota
	taskCard
	taskEmu
)

// menu choices on page 2: template IDs without the -ms/-fd ending
var wizMenus = []string{"plain", "everyday", "games", "net"}

type wizardWin struct {
	hwnd uintptr
	c    map[int]uintptr
	task wizTask
	page int
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
	w := &wizardWin{c: map[int]uintptr{}}
	a.wiz = w
	w.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyRSBWizard", appTitle(), 0x00C00000|0x00080000, cwDefault, cwDefault, a.s(640), a.s(560), a.hwnd, 0)
	mk := func(id int, class, text string, style uint32, note string) uintptr {
		h := createWindow(0, class, text, WS_CHILD|style, 0, 0, 10, 10, w.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		if note != "" {
			sendMsg(h, bcmSetNote, 0, uintptr(unsafe.Pointer(u16(note))))
		}
		w.c[id] = h
		return h
	}
	link := uint32(WS_TABSTOP | bsCommandLink)
	mk(wzHead, "STATIC", T("What would you like to do?"), 0, "")
	mk(wzFloppy, "BUTTON", T("Make a bootable floppy"), link, T("A disk image you can write to a floppy with WinImage or Greaseweazle."))
	mk(wzCard, "BUTTON", T("Make a bootable CF card or IDE disk"), link, T("With a card reader or a USB-IDE adapter, using the geometry of the target machine."))
	mk(wzEmu, "BUTTON", T("Make a disk image for an emulator"), link, T("A hard disk image for 86Box or PCem."))
	mk(wzExisting, "BUTTON", T("Change the boot menu of an existing system"), link, T("Read the menu from a disk image or a drive, edit it and write it back."))
	mk(wzEditor, "BUTTON", T("Just open the editor"), link, T("The full CONFIG.SYS / AUTOEXEC.BAT editor, starting with a plain DOS menu."))
	mk(wzShowStart, "BUTTON", T("Show this at start"), WS_TABSTOP|BS_AUTOCHECKBOX, "")
	setCheck(w.c[wzShowStart], !a.set.HideWizard)

	mk(wzHead2, "STATIC", T("Which DOS, and what should the boot menu offer?"), 0, "")
	mk(wzLblDOS, "STATIC", T("System:"), 0, "")
	mk(wzDOS, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, "")
	mk(wzLblMenu, "STATIC", T("Boot menu:"), 0, "")
	mk(wzMenu, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, "")
	mk(wzMenuDesc, "STATIC", "", 0, "")
	mk(wzBack, "BUTTON", T("< Back"), WS_TABSTOP, "")
	mk(wzNext, "BUTTON", T("Next >"), WS_TABSTOP|BS_DEFPUSHBUTTON, "")
	fillCombo(w.c[wzDOS], []string{"MS-DOS 6.x " + T("(system from your own boot disk or card)"), "FreeDOS"}, 0)
	fillCombo(w.c[wzMenu], []string{
		T("Plain DOS"),
		T("Everyday: DOS / + mouse and CD-ROM / everything"),
		T("Games: everything / EMS / no EMS / clean"),
		T("Network with Easy Commander / plain DOS"),
	}, 0)

	mk(wzHead3, "STATIC", T("Where is the system whose menu you want to change?"), 0, "")
	mk(wzFromImage, "BUTTON", T("In a disk image"), link, T("A floppy or hard disk image, a card backup or a fixed-size VHD."))
	mk(wzFromDir, "BUTTON", T("On a drive or in a folder"), link, T("For example the CF card in the card reader, by its drive letter."))
	mk(wzBack3, "BUTTON", T("< Back"), WS_TABSTOP, "")

	w.layout()
	w.show(1)
	enable(a.hwnd, false)
	pShowWindow.Call(w.hwnd, SW_SHOW)
}

func (w *wizardWin) layout() {
	a := theApp
	s := a.s
	cw, ch := clientSize(w.hwnd)
	m := s(16)
	lw := cw - 2*m
	move(w.c[wzHead], m, m, lw, s(24))
	y := m + s(34)
	for _, id := range []int{wzFloppy, wzCard, wzEmu, wzExisting, wzEditor} {
		move(w.c[id], m, y, lw, s(70))
		y += s(76)
	}
	move(w.c[wzShowStart], m, ch-m-s(24), s(300), s(24))

	move(w.c[wzHead2], m, m, lw, s(24))
	move(w.c[wzLblDOS], m, m+s(48), s(110), s(20))
	move(w.c[wzDOS], m+s(110), m+s(44), lw-s(110), s(200))
	move(w.c[wzLblMenu], m, m+s(88), s(110), s(20))
	move(w.c[wzMenu], m+s(110), m+s(84), lw-s(110), s(200))
	move(w.c[wzMenuDesc], m+s(110), m+s(120), lw-s(110), s(200))
	move(w.c[wzBack], cw-m-2*s(110)-s(8), ch-m-s(28), s(110), s(28))
	move(w.c[wzNext], cw-m-s(110), ch-m-s(28), s(110), s(28))

	move(w.c[wzHead3], m, m, lw, s(24))
	move(w.c[wzFromImage], m, m+s(40), lw, s(70))
	move(w.c[wzFromDir], m, m+s(116), lw, s(70))
	move(w.c[wzBack3], cw-m-s(110), ch-m-s(28), s(110), s(28))
}

func (w *wizardWin) show(page int) {
	w.page = page
	pages := map[int][]int{
		1: {wzHead, wzFloppy, wzCard, wzEmu, wzExisting, wzEditor, wzShowStart},
		2: {wzHead2, wzLblDOS, wzDOS, wzLblMenu, wzMenu, wzMenuDesc, wzBack, wzNext},
		3: {wzHead3, wzFromImage, wzFromDir, wzBack3},
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
}

func (w *wizardWin) templateID() string {
	m := clampIdx(int(sendMsg(w.c[wzMenu], CB_GETCURSEL, 0, 0)), len(wizMenus))
	suffix := "-ms"
	if sendMsg(w.c[wzDOS], CB_GETCURSEL, 0, 0) == 1 {
		suffix = "-fd"
	}
	return wizMenus[m] + suffix
}

func (w *wizardWin) describe() {
	t := TemplateByID(w.templateID())
	desc := T(t.Desc)
	p := t.Make()
	desc += "\n\n" + T("Menu items:")
	for _, it := range p.Items {
		desc += "\n• " + it.Title
	}
	desc += "\n\n" + T("You can change everything later in the editor (New..., Building blocks...).")
	setText(w.c[wzMenuDesc], desc)
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
	w.close()
	if !a.askSave() {
		return
	}
	a.proj, a.path, a.dirty, a.sel = TemplateByID(tid).Make(), "", false, 1
	a.loadProject()
	switch task {
	case taskFloppy:
		a.openBootWinWith(MediaF144, nil)
	case taskCard:
		if msgBox(a.hwnd, T("Is there a card or disk that already boots DOS on the target machine?\n\nYes: back it up first; its geometry and system are then taken from the backup (recommended).\nNo: enter the geometry by hand (MSD.EXE on the target machine shows it)."), appTitle(), MB_YESNO|MB_ICONQUESTION) == IDYES {
			a.backupCard()
		} else {
			a.openBootWinWith(MediaHDD, nil)
		}
	case taskEmu:
		// a geometry every BIOS and emulator agrees on, about 500 MB
		a.openBootWinWith(MediaHDD, &Geometry{1015, 16, 63})
	}
}

func wizardProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	w := a.wiz
	switch msg {
	case WM_COMMAND:
		if w == nil {
			return 0
		}
		code := hiword(wParam)
		switch int(loword(wParam)) {
		case wzFloppy, wzCard, wzEmu:
			w.task = wizTask(int(loword(wParam)) - wzFloppy)
			w.show(2)
		case wzExisting:
			w.show(3)
		case wzEditor:
			w.close()
		case wzBack, wzBack3:
			w.show(1)
		case wzNext:
			w.next()
		case wzDOS, wzMenu:
			if code == CBN_SELCHANGE {
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
