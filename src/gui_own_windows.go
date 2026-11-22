//go:build windows

package main

import (
	"path/filepath"
	"strings"
)

// The "Add own driver" dialog of the Drivers window.

const (
	owLblFile = 10 + iota
	owFile
	owBrowse
	owLblMain
	owMain
	owLblName
	owName
	owLblGroup
	owGroup
	owLblWhere
	owWhere
	owLblLine
	owLine
	owLblItems
	owItems
	owOK
	owCancel
)

type ownWin struct {
	hwnd    uintptr
	c       map[int]uintptr
	zipped  []string // files of a zip source
	loading bool
}

func (a *app) openOwnDriver() {
	if a.own != nil || a.drv == nil {
		return
	}
	loadCommonControls()
	w := &ownWin{c: map[int]uintptr{}}
	a.own = w
	owner := a.hwnd
	w.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyDOSDeployOwn", T("Add own driver"), 0x00C00000|0x00080000, cwDefault, cwDefault, a.s(600), a.s(560), owner, 0)
	mk := func(id int, class, text string, style, ex uint32) uintptr {
		h := createWindow(ex, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, w.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		w.c[id] = h
		return h
	}
	edit := uint32(WS_TABSTOP | ES_AUTOHSCROLL)
	mk(owLblFile, "STATIC", T("Driver file: a .SYS, .COM or .EXE file, or a zip with the driver:"), 0, 0)
	mk(owFile, "EDIT", "", edit|ES_READONLY, WS_EX_CLIENTEDGE)
	mk(owBrowse, "BUTTON", T("Browse..."), WS_TABSTOP, 0)
	mk(owLblMain, "STATIC", T("File to load:"), 0, 0)
	mk(owMain, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, 0)
	mk(owLblName, "STATIC", T("Name:"), 0, 0)
	mk(owName, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(owLblGroup, "STATIC", T("Kind:"), 0, 0)
	mk(owGroup, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, 0)
	mk(owLblWhere, "STATIC", T("Loaded in:"), 0, 0)
	mk(owWhere, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, 0)
	mk(owLblLine, "STATIC", T("Line:"), 0, 0)
	mk(owLine, "EDIT", "", edit, WS_EX_CLIENTEDGE)
	mk(owLblItems, "STATIC", T("Menu items that load it:"), 0, 0)
	lv := mk(owItems, "SysListView32", "", WS_TABSTOP|LVS_REPORT|LVS_SINGLESEL|LVS_NOSORTHEADER|0x4000, WS_EX_CLIENTEDGE) // LVS_NOCOLUMNHEADER
	sendMsg(lv, LVM_SETEXTENDEDLISTVIEWSTYLE, 0, LVS_EX_CHECKBOXES|LVS_EX_FULLROWSELECT|LVS_EX_DOUBLEBUFFER)
	lvInsertColumn(lv, 0, "", a.s(500), false)
	mk(owOK, "BUTTON", T("Add"), WS_TABSTOP|BS_DEFPUSHBUTTON, 0)
	mk(owCancel, "BUTTON", T("Cancel"), WS_TABSTOP, 0)
	fillCombo(w.c[owGroup], OwnGroupNames(), len(OwnGroups)-1)
	fillCombo(w.c[owWhere], []string{"CONFIG.SYS", "AUTOEXEC.BAT"}, 0)
	for i, it := range a.proj.Items {
		title := it.Title
		if title == "" {
			title = it.ID
		}
		lvSetCell(lv, i, 0, title, iImageNone)
		lvSetChecked(lv, i, true)
	}

	s := a.s
	cw, ch := clientSize(w.hwnd)
	m := s(14)
	rh := s(23)
	bw := fitW(s(100), s(24), w.c[owBrowse])
	y := m
	move(w.c[owLblFile], m, y, cw-2*m, s(18))
	y += s(22)
	move(w.c[owFile], m, y, cw-2*m-bw-s(8), rh)
	move(w.c[owBrowse], cw-m-bw, y, bw, rh)
	y += rh + s(12)
	col := fitW(s(100), s(8), w.c[owLblMain], w.c[owLblName], w.c[owLblGroup], w.c[owLblWhere], w.c[owLblLine])
	for _, r := range [][2]int{{owLblMain, owMain}, {owLblName, owName}, {owLblGroup, owGroup}, {owLblWhere, owWhere}, {owLblLine, owLine}} {
		move(w.c[r[0]], m, y+s(4), col, s(18))
		ew := cw - 2*m - col
		if r[1] == owGroup || r[1] == owWhere {
			ew = s(220)
		}
		h := rh
		if r[1] == owMain || r[1] == owGroup || r[1] == owWhere {
			h = s(200)
		}
		move(w.c[r[1]], m+col, y, ew, h)
		y += rh + s(8)
	}
	y += s(4)
	move(w.c[owLblItems], m, y, cw-2*m, s(18))
	y += s(22)
	move(w.c[owItems], m, y, cw-2*m, ch-y-m-s(40))
	ow := fitW(s(100), s(24), w.c[owOK], w.c[owCancel])
	move(w.c[owOK], cw-m-2*ow-s(8), ch-m-s(28), ow, s(28))
	move(w.c[owCancel], cw-m-ow, ch-m-s(28), ow, s(28))
	enable(w.c[owMain], false)
	enable(w.c[owOK], false)
	enable(owner, false)
	pShowWindow.Call(w.hwnd, SW_SHOW)
	w.browse()
}

func (w *ownWin) source() string { return getText(w.c[owFile]) }

func (w *ownWin) main() string {
	if len(w.zipped) > 0 {
		return w.zipped[clampIdx(int(sendMsg(w.c[owMain], CB_GETCURSEL, 0, 0)), len(w.zipped))]
	}
	return strings.ToUpper(filepath.Base(w.source()))
}

func (w *ownWin) config() bool { return sendMsg(w.c[owWhere], CB_GETCURSEL, 0, 0) == 0 }

func (w *ownWin) browse() {
	p := fileDialog(w.hwnd, false, T("Driver file or zip"), "", "",
		T("Driver file or zip"), "*.sys;*.com;*.exe;*.zip", T("All files"), "*.*")
	if p == "" {
		return
	}
	w.zipped = nil
	if strings.EqualFold(filepath.Ext(p), ".zip") {
		names, err := ZipDriverFiles(p)
		if err != nil || len(names) == 0 {
			msgBox(w.hwnd, T("Did not work:")+"\n"+errText(err, T("the zip is empty")), appTitle(), MB_ICONERROR)
			return
		}
		w.zipped = names
	}
	setText(w.c[owFile], p)
	w.loading = true
	if len(w.zipped) > 0 {
		fillCombo(w.c[owMain], w.zipped, 0)
	} else {
		fillCombo(w.c[owMain], []string{strings.ToUpper(filepath.Base(p))}, 0)
	}
	enable(w.c[owMain], len(w.zipped) > 1)
	w.loading = false
	w.suggest(true)
	enable(w.c[owOK], true)
}

func errText(err error, def string) string {
	if err != nil {
		return err.Error()
	}
	return def
}

// suggest fills the line (and, for a new file, the name) from the file.
func (w *ownWin) suggest(name bool) {
	o := OwnDriver{Source: w.source(), Main: w.main()}
	line, cfg := SuggestOwnLine(o)
	if name {
		setText(w.c[owName], dosStem(o.Main))
	}
	where := 1
	if cfg {
		where = 0
	}
	w.loading = true
	sendMsg(w.c[owWhere], CB_SETCURSEL, uintptr(where), 0)
	setText(w.c[owLine], line)
	w.loading = false
}

// whereChanged keeps the line right for the file it goes into.
func (w *ownWin) whereChanged() {
	line := strings.TrimSpace(getText(w.c[owLine]))
	up := strings.ToUpper(line)
	if w.config() && !strings.HasPrefix(up, "DEVICE") && !strings.Contains(up, "=") {
		line = "DEVICE=" + line
	} else if !w.config() && strings.HasPrefix(up, "DEVICE=") {
		line = line[len("DEVICE="):]
	}
	setText(w.c[owLine], line)
}

func (w *ownWin) add() {
	a := theApp
	if w.source() == "" {
		return
	}
	line := strings.TrimSpace(getText(w.c[owLine]))
	if line == "" {
		msgBox(w.hwnd, T("Type the line that loads the driver."), appTitle(), MB_ICONINFORMATION)
		return
	}
	var items []int
	for i := range a.proj.Items {
		if lvChecked(w.c[owItems], i) {
			items = append(items, i)
		}
	}
	o := OwnDriver{
		Name:   strings.TrimSpace(getText(w.c[owName])),
		Group:  OwnGroups[clampIdx(int(sendMsg(w.c[owGroup], CB_GETCURSEL, 0, 0)), len(OwnGroups))],
		Source: w.source(),
		Main:   w.main(),
		Config: w.config(),
		Line:   line,
	}
	d, err := a.store.AddOwnDriver(o)
	if err != nil {
		msgBox(w.hwnd, T("Did not work:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	a.proj.AddOwnLine(d.ID, items, o.Config, line)
	a.menuEdit()
	a.loadProject()
	w.close()
	if a.drv != nil {
		a.drv.refreshFocus(len(a.store.Catalog) - 1)
		setText(a.drv.c[dvStatus], T("Added: %s", d.Name))
	}
	a.updateStatus()
	if a.boot != nil {
		a.boot.updateDrivers()
	}
}

func (w *ownWin) close() {
	a := theApp
	enable(a.hwnd, true)
	pDestroyWindow.Call(w.hwnd)
	a.own = nil
	pSetForegroundWindow.Call(a.hwnd)
}

func ownProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	w := a.own
	switch msg {
	case WM_COMMAND:
		if w == nil || w.loading {
			return 0
		}
		code := hiword(wParam)
		switch int(loword(wParam)) {
		case owBrowse:
			w.browse()
		case owMain:
			if code == CBN_SELCHANGE {
				w.suggest(true)
			}
		case owWhere:
			if code == CBN_SELCHANGE {
				w.whereChanged()
			}
		case owOK, 1:
			if code == 0 {
				w.add()
			}
		case owCancel, IDCANCEL:
			if code == 0 {
				w.close()
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
