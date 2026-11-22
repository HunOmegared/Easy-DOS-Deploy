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
