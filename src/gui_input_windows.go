//go:build windows

package main

import (
	"strings"
	"unsafe"
)

// inputBox asks for one line of text in a small modal window.

const (
	ibPrompt = 10 + iota
	ibEdit
	ibHint
	ibOK
	ibCancel
)

type inputWin struct {
	hwnd uintptr
	c    map[int]uintptr
	done bool
	ok   bool
	text string
}

var curInput *inputWin

func inputBox(owner uintptr, title, prompt, hint, initial string) (string, bool) {
	a := theApp
	w := &inputWin{c: map[int]uintptr{}}
	curInput = w
	w.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyDOSDeployInput", title, 0x00C00000|0x00080000, cwDefault, cwDefault, a.s(460), a.s(190), owner, 0)
	mk := func(id int, class, text string, style, ex uint32) uintptr {
		h := createWindow(ex, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, w.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		w.c[id] = h
		return h
	}
	mk(ibPrompt, "STATIC", prompt, 0, 0)
	mk(ibEdit, "EDIT", initial, WS_TABSTOP|ES_AUTOHSCROLL, WS_EX_CLIENTEDGE)
	mk(ibHint, "STATIC", hint, 0, 0)
	mk(ibOK, "BUTTON", T("OK"), WS_TABSTOP|BS_DEFPUSHBUTTON, 0)
	mk(ibCancel, "BUTTON", T("Cancel"), WS_TABSTOP, 0)
	s := a.s
	cw, ch := clientSize(w.hwnd)
	m := s(14)
	move(w.c[ibPrompt], m, m, cw-2*m, s(18))
	move(w.c[ibEdit], m, m+s(22), cw-2*m, s(24))
	move(w.c[ibHint], m, m+s(52), cw-2*m, s(18))
	bw := fitW(s(90), s(24), w.c[ibOK], w.c[ibCancel])
	move(w.c[ibOK], cw-m-2*bw-s(8), ch-m-s(28), bw, s(28))
	move(w.c[ibCancel], cw-m-bw, ch-m-s(28), bw, s(28))
	enable(owner, false)
	pShowWindow.Call(w.hwnd, SW_SHOW)
	pSetFocus.Call(w.c[ibEdit])
	sendMsg(w.c[ibEdit], 0x00B1, 0, ^uintptr(0)) // EM_SETSEL: all
	var m2 msgT
	for !w.done {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m2)), 0, 0, 0)
		if int32(r) <= 0 {
			pPostQuitMessage.Call(0)
			break
		}
		if r, _, _ := pIsDialogMessageW.Call(w.hwnd, uintptr(unsafe.Pointer(&m2))); r != 0 {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m2)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m2)))
	}
	enable(owner, true)
	pDestroyWindow.Call(w.hwnd)
	pSetForegroundWindow.Call(owner)
	curInput = nil
	return w.text, w.ok
}

func inputProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	w := curInput
	switch msg {
	case WM_COMMAND:
		if w == nil || hiword(wParam) != 0 {
			return 0
		}
		switch int(loword(wParam)) {
		case ibOK, 1:
			w.text = strings.TrimSpace(getText(w.c[ibEdit]))
			if w.text == "" {
				return 0
			}
			w.ok, w.done = true, true
		case ibCancel, IDCANCEL:
			w.done = true
		}
		return 0
	case WM_CLOSE:
		if w != nil {
			w.done = true
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// destBox asks for a DOS destination: a drive from the list and a folder.
const (
	dbPrompt = 10 + iota
	dbLblDrive
	dbDrive
	dbLblFolder
	dbFolder
	dbHint
	dbOK
	dbCancel
)

func destBox(owner uintptr, prompt string, drives []string, drive int, folder, hint string) (int, string, bool) {
	a := theApp
	w := &inputWin{c: map[int]uintptr{}}
	curInput = w
	w.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyDOSDeployDest", T("Destination on the DOS disk"), 0x00C00000|0x00080000, cwDefault, cwDefault, a.s(520), a.s(250), owner, 0)
	mk := func(id int, class, text string, style, ex uint32) uintptr {
		h := createWindow(ex, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, w.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		w.c[id] = h
		return h
	}
	mk(dbPrompt, "STATIC", prompt, 0, 0)
	mk(dbLblDrive, "STATIC", T("Drive:"), 0, 0)
	mk(dbDrive, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, 0)
	mk(dbLblFolder, "STATIC", T("Folder:"), 0, 0)
	mk(dbFolder, "EDIT", folder, WS_TABSTOP|ES_AUTOHSCROLL|ES_UPPERCASE, WS_EX_CLIENTEDGE)
	mk(dbHint, "STATIC", hint, 0, 0)
	mk(dbOK, "BUTTON", T("OK"), WS_TABSTOP|BS_DEFPUSHBUTTON, 0)
	mk(dbCancel, "BUTTON", T("Cancel"), WS_TABSTOP, 0)
	fillCombo(w.c[dbDrive], drives, clampIdx(drive, len(drives)))
	s := a.s
	cw, ch := clientSize(w.hwnd)
	m := s(14)
	col := fitW(s(70), s(8), w.c[dbLblDrive], w.c[dbLblFolder])
	move(w.c[dbPrompt], m, m, cw-2*m, s(36))
	y := m + s(42)
	move(w.c[dbLblDrive], m, y+s(4), col, s(18))
	move(w.c[dbDrive], m+col, y, s(220), s(160))
	y += s(32)
	move(w.c[dbLblFolder], m, y+s(4), col, s(18))
	move(w.c[dbFolder], m+col, y, cw-2*m-col, s(24))
	y += s(30)
	move(w.c[dbHint], m+col, y, cw-2*m-col, s(36))
	bw := fitW(s(90), s(24), w.c[dbOK], w.c[dbCancel])
	move(w.c[dbOK], cw-m-2*bw-s(8), ch-m-s(28), bw, s(28))
	move(w.c[dbCancel], cw-m-bw, ch-m-s(28), bw, s(28))
	enable(owner, false)
	pShowWindow.Call(w.hwnd, SW_SHOW)
	pSetFocus.Call(w.c[dbFolder])
	var m2 msgT
	for !w.done {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m2)), 0, 0, 0)
		if int32(r) <= 0 {
			pPostQuitMessage.Call(0)
			break
		}
		if r, _, _ := pIsDialogMessageW.Call(w.hwnd, uintptr(unsafe.Pointer(&m2))); r != 0 {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m2)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m2)))
	}
	d := int(sendMsg(w.c[dbDrive], CB_GETCURSEL, 0, 0))
	enable(owner, true)
	pDestroyWindow.Call(w.hwnd)
	pSetForegroundWindow.Call(owner)
	curInput = nil
	return d, strings.Trim(strings.TrimSpace(w.text), `\`), w.ok
}

func destProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	w := curInput
	switch msg {
	case WM_COMMAND:
		if w == nil || hiword(wParam) != 0 {
			return 0
		}
		switch int(loword(wParam)) {
		case dbOK, 1:
			t := strings.Trim(strings.TrimSpace(getText(w.c[dbFolder])), `\`)
			if t != "" {
				if _, _, err := splitDest(`C:\` + t); err != nil {
					msgBox(hwnd, err.Error(), appTitle(), MB_ICONWARNING)
					return 0
				}
			}
			w.text, w.ok, w.done = t, true, true
		case dbCancel, IDCANCEL:
			w.done = true
		}
		return 0
	case WM_CLOSE:
		if w != nil {
			w.done = true
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}
