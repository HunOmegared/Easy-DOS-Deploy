//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"
)

// The MS-DOS Setup disks dialog of the image window: which disk images (or
// folders) and SETUP's choices for the optional programs.

const (
	suHelp = 10 + iota
	suList
	suAdd
	suAddDir
	suRemove
	suInfo
	suLblBackup
	suBackup
	suLblAV
	suAV
	suLblUndel
	suUndel
	suDrvSpace
	suOK
	suCancel
)

type setupWin struct {
	hwnd  uintptr
	c     map[int]uintptr
	paths []string
	disks *SetupDisks
	err   error
}

func compChoices() []string {
	return []string{T("None"), "MS-DOS", "Windows", T("MS-DOS and Windows")}
}

func (a *app) openSetupDisks() {
	if a.setupDlg != nil || a.boot == nil {
		return
	}
	w := &setupWin{c: map[int]uintptr{}}
	a.setupDlg = w
	w.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyDOSDeploySetup", T("MS-DOS Setup disks"), 0x00C00000|0x00080000, cwDefault, cwDefault, a.s(620), a.s(520), a.boot.hwnd, 0)
	mk := func(id int, class, text string, style, ex uint32) uintptr {
		h := createWindow(ex, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, w.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		w.c[id] = h
		return h
	}
	mk(suHelp, "STATIC", T("Give the images of your MS-DOS 6.x Setup disks (or folders with their files), in any order. The program expands them into C:\\DOS as Setup would, without running Setup on the target machine."), 0, 0)
	mk(suList, "LISTBOX", "", WS_TABSTOP|LBS_NOTIFY|LBS_NOINTEGRALHEIGHT|WS_VSCROLL|WS_HSCROLL, WS_EX_CLIENTEDGE)
	mk(suAdd, "BUTTON", T("Add images..."), WS_TABSTOP, 0)
	mk(suAddDir, "BUTTON", T("Add a folder..."), WS_TABSTOP, 0)
	mk(suRemove, "BUTTON", T("Remove"), WS_TABSTOP, 0)
	mk(suInfo, "STATIC", "", 0, 0)
	mk(suLblBackup, "STATIC", T("Backup:"), 0, 0)
	mk(suBackup, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, 0)
	mk(suLblAV, "STATIC", T("Anti-Virus:"), 0, 0)
	mk(suAV, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, 0)
	mk(suLblUndel, "STATIC", T("Undelete:"), 0, 0)
	mk(suUndel, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, 0)
	mk(suDrvSpace, "BUTTON", T("DriveSpace (disk compression; not needed on a CF card)"), WS_TABSTOP|BS_AUTOCHECKBOX, 0)
	mk(suOK, "BUTTON", T("OK"), WS_TABSTOP|BS_DEFPUSHBUTTON, 0)
	mk(suCancel, "BUTTON", T("Cancel"), WS_TABSTOP, 0)

	b := a.boot
	opt := DefaultSetupOptions()
	switch {
	case b.setup != nil:
		w.paths, opt = append([]string(nil), b.setupPaths...), b.setupOpt
	case len(a.set.SetupDisks) > 0:
		w.paths = append([]string(nil), a.set.SetupDisks...)
		if a.set.SetupOpt != nil {
			opt = *a.set.SetupOpt
		}
	}
	fillCombo(w.c[suBackup], compChoices(), opt.Backup)
	fillCombo(w.c[suAV], compChoices(), opt.AntiVirus)
	fillCombo(w.c[suUndel], compChoices(), opt.Undelete)
	setCheck(w.c[suDrvSpace], opt.DriveSpace)

	s := a.s
	cw, ch := clientSize(w.hwnd)
	m := s(14)
	move(w.c[suHelp], m, m, cw-2*m, s(54))
	y := m + s(60)
	bw := fitW(s(130), s(24), w.c[suAdd], w.c[suAddDir], w.c[suRemove])
	move(w.c[suList], m, y, cw-2*m-bw-s(10), s(110))
	move(w.c[suAdd], cw-m-bw, y, bw, s(26))
	move(w.c[suAddDir], cw-m-bw, y+s(32), bw, s(26))
	move(w.c[suRemove], cw-m-bw, y+s(64), bw, s(26))
	y += s(118)
	move(w.c[suInfo], m, y, cw-2*m, s(90))
	y += s(98)
	col := fitW(s(100), s(8), w.c[suLblBackup], w.c[suLblAV], w.c[suLblUndel])
	for _, r := range [][2]int{{suLblBackup, suBackup}, {suLblAV, suAV}, {suLblUndel, suUndel}} {
		move(w.c[r[0]], m, y+s(4), col, s(18))
		move(w.c[r[1]], m+col, y, s(220), s(140))
		y += s(30)
	}
	move(w.c[suDrvSpace], m, y+s(2), cw-2*m, s(24))
	ow := fitW(s(100), s(24), w.c[suOK], w.c[suCancel])
	move(w.c[suOK], cw-m-2*ow-s(8), ch-m-s(28), ow, s(28))
	move(w.c[suCancel], cw-m-ow, ch-m-s(28), ow, s(28))
	w.reload()
	enable(b.hwnd, false)
	pShowWindow.Call(w.hwnd, SW_SHOW)
	if len(w.paths) == 0 {
		w.addImages()
	}
}

func (w *setupWin) options() SetupOptions {
	sel := func(id int) int { return clampIdx(int(sendMsg(w.c[id], CB_GETCURSEL, 0, 0)), 4) }
	return SetupOptions{Backup: sel(suBackup), AntiVirus: sel(suAV), Undelete: sel(suUndel), DriveSpace: checked(w.c[suDrvSpace])}
}

// reload reads the disks again and tells what was found.
func (w *setupWin) reload() {
	lb := w.c[suList]
	sendMsg(lb, LB_RESETCONTENT, 0, 0)
	for _, p := range w.paths {
		sendMsg(lb, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(p))))
	}
	w.disks, w.err = nil, nil
	if len(w.paths) == 0 {
		setText(w.c[suInfo], T("No disks yet."))
		enable(w.c[suOK], false)
		return
	}
	w.disks, w.err = ReadSetupDisks(w.paths)
	enable(w.c[suOK], w.err == nil)
	if w.err != nil {
		setText(w.c[suInfo], T("Error: %s", w.err.Error()))
		return
	}
	w.showInfo()
}

func (w *setupWin) showInfo() {
	if w.disks == nil {
		return
	}
	d := w.disks
	var nums []string
	for _, n := range d.Found {
		nums = append(nums, strconv.Itoa(n))
	}
	ver := d.Version
	if ver == "" {
		ver = "MS-DOS"
	}
	lines := []string{T("%s, disks found: %s", ver, strings.Join(nums, ", "))}
	files, missing := d.DOSFiles(w.options())
	lines = append(lines, T("%d files go into C:\\DOS.", len(files)))
	lines = append(lines, missing...)
	lines = append(lines, d.Notes...)
	setText(w.c[suInfo], strings.Join(lines, "\n"))
}

func (w *setupWin) addImages() {
	if p := fileDialog(w.hwnd, false, T("MS-DOS Setup disk image"), "", "", imgFilter()...); p != "" {
		w.add(p)
		// the other disks are usually next to it with similar names
		if len(w.paths) == 1 {
			w.addSiblings(p)
		}
		w.reload()
	}
}

// addSiblings adds the other disks of a set from the same folder: the
// files whose name differs from the first one only in its digits.
func (w *setupWin) addSiblings(first string) {
	dir, base := filepath.Dir(first), filepath.Base(first)
	mask := func(s string) string {
		return strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return '#'
			}
			return r
		}, strings.ToUpper(s))
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		if !e.IsDir() && e.Name() != base && mask(e.Name()) == mask(base) {
			w.add(filepath.Join(dir, e.Name()))
		}
	}
}

func (w *setupWin) add(p string) {
	for _, x := range w.paths {
		if strings.EqualFold(x, p) {
			return
		}
	}
	w.paths = append(w.paths, p)
}

func (w *setupWin) close(ok bool) {
	a := theApp
	b := a.boot
	if ok && w.disks != nil && b != nil {
		opt := w.options()
		b.setup, b.setupPaths, b.setupOpt = w.disks, append([]string(nil), w.paths...), opt
		a.set.SetupDisks, a.set.SetupOpt = b.setupPaths, &opt
		saveSettings(a.data, a.set)
	}
	if b != nil {
		enable(b.hwnd, true)
	}
	pDestroyWindow.Call(w.hwnd)
	a.setupDlg = nil
	if b != nil {
		b.setupChanged()
		pSetForegroundWindow.Call(b.hwnd)
		if b.calcAfter {
			b.calcAfter = false
			if atoiU(getText(b.c[bwC])) == 0 {
				a.openCalc()
			}
		}
	}
}

func setupProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	w := a.setupDlg
	switch msg {
	case WM_COMMAND:
		if w == nil {
			return 0
		}
		code := hiword(wParam)
		switch int(loword(wParam)) {
		case suAdd:
			w.addImages()
		case suAddDir:
			if p := folderDialog(hwnd, T("Folder with the files of a Setup disk")); p != "" {
				w.add(p)
				w.reload()
			}
		case suRemove:
			if i := int(sendMsg(w.c[suList], LB_GETCURSEL, 0, 0)); i >= 0 && i < len(w.paths) {
				w.paths = append(w.paths[:i], w.paths[i+1:]...)
				w.reload()
			}
		case suBackup, suAV, suUndel:
			if code == CBN_SELCHANGE {
				w.showInfo()
			}
		case suDrvSpace:
			if code == BN_CLICKED {
				w.showInfo()
			}
		case suOK, 1:
			if code == 0 && w.disks != nil {
				w.close(true)
			}
		case suCancel, IDCANCEL:
			if code == 0 {
				w.close(false)
			}
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
