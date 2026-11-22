//go:build windows

package main

// "Help with the disk profile": the three ways to find the geometry the
// target machine's BIOS gives to DOS, each with its buttons.

const (
	ghIntro = 10 + iota
	ghText1
	ghBios
	ghText2
	ghFDFloppy
	ghOnFloppy
	ghSaveCom
	ghText3
	ghBackup
	ghText4
	ghClose
)

type geomHelpWin struct {
	hwnd uintptr
	c    map[int]uintptr
}

func (a *app) openGeomHelp() {
	if a.geomHelp != nil {
		pSetForegroundWindow.Call(a.geomHelp.hwnd)
		return
	}
	w := &geomHelpWin{c: map[int]uintptr{}}
	a.geomHelp = w
	w.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyDOSDeployGeomHelp", T("Help with the disk profile"), 0x00C00000|0x00080000, cwDefault, cwDefault, a.s(640), a.s(600), a.hwnd, 0)
	mk := func(id int, class, text string, style uint32) uintptr {
		h := createWindow(0, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, w.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		w.c[id] = h
		return h
	}
	mk(ghIntro, "STATIC", T("The disk profile is the geometry (cylinders, heads, sectors) that the BIOS of the target machine gives to DOS. It must match exactly, or the disk will not boot there. There are three ways to find it."), 0)
	sendMsg(mk(ghText1, "STATIC", T("1. BIOS setup: put the card or disk into the target machine, let BIOS setup detect it (Auto) and type the values it shows. The program works out what DOS gets from them."), 0), WM_SETFONT, a.font, 1)
	mk(ghBios, "BUTTON", T("From BIOS setup..."), WS_TABSTOP)
	mk(ghText2, "STATIC", T("2. GEOM: the most exact way. Boot the target machine from a floppy with GEOM.COM, with the card or disk in place: it shows the geometry DOS gets."), 0)
	mk(ghFDFloppy, "BUTTON", T("FreeDOS floppy with GEOM..."), WS_TABSTOP)
	mk(ghOnFloppy, "BUTTON", T("GEOM onto my boot floppy..."), WS_TABSTOP)
	mk(ghSaveCom, "BUTTON", T("Save GEOM.COM..."), WS_TABSTOP)
	mk(ghText3, "STATIC", T("3. A working disk: if a card or disk already boots DOS on that machine, back it up first (Disk image menu) and read the geometry from the backup."), 0)
	mk(ghBackup, "BUTTON", T("From a disk backup..."), WS_TABSTOP)
	mk(ghText4, "STATIC", T("Then save it as a disk profile: next time you only choose it from the list."), 0)
	mk(ghClose, "BUTTON", T("Close"), WS_TABSTOP|BS_DEFPUSHBUTTON)

	s := a.s
	cw, ch := clientSize(w.hwnd)
	m := s(16)
	tw := cw - 2*m
	bh := s(28)
	y := m
	move(w.c[ghIntro], m, y, tw, s(54))
	y += s(64)
	row := func(text int, buttons ...int) {
		move(w.c[text], m, y, tw, s(54))
		y += s(56)
		x := m + s(20)
		for _, id := range buttons {
			bw := fitW(s(120), s(24), w.c[id])
			move(w.c[id], x, y, bw, bh)
			x += bw + s(8)
		}
		y += bh + s(18)
	}
	row(ghText1, ghBios)
	row(ghText2, ghFDFloppy, ghOnFloppy, ghSaveCom)
	row(ghText3, ghBackup)
	move(w.c[ghText4], m, y, tw, s(40))
	cb := fitW(s(100), s(24), w.c[ghClose])
	move(w.c[ghClose], cw-m-cb, ch-m-bh, cb, bh)
	pShowWindow.Call(w.hwnd, SW_SHOW)
}

func (w *geomHelpWin) close() {
	a := theApp
	pDestroyWindow.Call(w.hwnd)
	a.geomHelp = nil
	pSetForegroundWindow.Call(a.hwnd)
}

func geomHelpProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	w := a.geomHelp
	switch msg {
	case WM_COMMAND:
		if w == nil || hiword(wParam) != 0 {
			return 0
		}
		id := int(loword(wParam))
		if id == ghClose || id == IDCANCEL || id == 1 {
			w.close()
			return 0
		}
		// the help closes, then the chosen tool opens
		w.close()
		switch id {
		case ghBios:
			a.showPage(pageDisk)
			a.openCalc()
		case ghFDFloppy:
			a.makeGeomFloppy()
		case ghOnFloppy:
			a.geomToFloppy()
		case ghSaveCom:
			a.saveGeom()
		case ghBackup:
			a.showPage(pageDisk)
			a.boot.fromImage()
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
