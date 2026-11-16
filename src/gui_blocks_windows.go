//go:build windows

package main

import (
	"strings"
)

// The "menu item from building blocks" window.

const (
	bkPreset = 10 + iota
	bkID
	bkTitle
	bkMemory
	bkMouse
	bkCD
	bkCDDrv
	bkCDLetter
	bkSound
	bkBlaster
	bkPnP
	bkNet
	bkNetCard
	bkNetArgs
	bkEC
	bkPreview
	bkOK
	bkCancel
	bkLblPreset
	bkLblID
	bkLblTitle
	bkLblMemory
	bkLblCDDrv
	bkLblLetter
	bkLblBlaster
	bkLblCard
	bkLblArgs
	bkLblPreview
)

type blockWin struct {
	hwnd    uintptr
	c       map[int]uintptr
	edit    int // index into Items being edited, -1 = new item
	loading bool
}

func (a *app) openBlocks(edit int) {
	if a.blocks != nil {
		pSetForegroundWindow.Call(a.blocks.hwnd)
		return
	}
	a.commit()
	w := &blockWin{c: map[int]uintptr{}, edit: edit}
	a.blocks = w
	title := T("New menu item")
	if edit >= 0 {
		title = T("Building blocks of %s", a.proj.Items[edit].ID)
	}
	w.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyDOSDeployBlocks", title, 0x00C00000|0x00080000, cwDefault, cwDefault, a.s(720), a.s(600), a.hwnd, 0)
	mk := func(id int, class, text string, style, ex uint32) uintptr {
		h := createWindow(ex, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, w.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		w.c[id] = h
		return h
	}
	edit1 := uint32(WS_TABSTOP | ES_AUTOHSCROLL)
	combo := uint32(WS_TABSTOP | CBS_DROPDOWNLIST | WS_VSCROLL)
	check := uint32(WS_TABSTOP | BS_AUTOCHECKBOX)
	mk(bkLblPreset, "STATIC", T("Start from:"), 0, 0)
	mk(bkPreset, "COMBOBOX", "", combo, 0)
	mk(bkLblID, "STATIC", T("ID:"), 0, 0)
	sendMsg(mk(bkID, "EDIT", "", edit1|ES_UPPERCASE, WS_EX_CLIENTEDGE), EM_SETLIMIT, 8, 0)
	mk(bkLblTitle, "STATIC", T("Text in the menu:"), 0, 0)
	cue(mk(bkTitle, "EDIT", "", edit1, WS_EX_CLIENTEDGE), T("automatic"))
	mk(bkLblMemory, "STATIC", T("Memory:"), 0, 0)
	mk(bkMemory, "COMBOBOX", "", combo, 0)
	mk(bkMouse, "BUTTON", T("Mouse (CuteMouse)"), check, 0)
	mk(bkCD, "BUTTON", "CD-ROM", check, 0)
	mk(bkLblCDDrv, "STATIC", T("Driver:"), 0, 0)
	mk(bkCDDrv, "COMBOBOX", "", combo, 0)
	mk(bkLblLetter, "STATIC", T("Letter:"), 0, 0)
	sendMsg(mk(bkCDLetter, "EDIT", "", edit1|ES_UPPERCASE, WS_EX_CLIENTEDGE), EM_SETLIMIT, 1, 0)
	mk(bkSound, "BUTTON", T("Sound (Sound Blaster compatible)"), check, 0)
	mk(bkLblBlaster, "STATIC", "BLASTER:", 0, 0)
	mk(bkBlaster, "EDIT", "", edit1|ES_UPPERCASE, WS_EX_CLIENTEDGE)
	mk(bkPnP, "BUTTON", T("ISA PnP card: set it up with UNISOUND"), check, 0)
	mk(bkNet, "BUTTON", T("Network (packet driver + mTCP)"), check, 0)
	mk(bkLblCard, "STATIC", T("Card:"), 0, 0)
	mk(bkNetCard, "COMBOBOX", "", combo, 0)
	mk(bkLblArgs, "STATIC", T("Parameters:"), 0, 0)
	mk(bkNetArgs, "EDIT", "", edit1, WS_EX_CLIENTEDGE)
	mk(bkEC, "BUTTON", T("Start Easy Commander"), check, 0)
	mk(bkLblPreview, "STATIC", T("The lines this makes (for %s):", a.proj.Target.String()), 0, 0)
	h := mk(bkPreview, "EDIT", "", ES_MULTILINE|ES_READONLY|ES_AUTOVSCROLL|ES_AUTOHSCROLL|WS_VSCROLL|WS_HSCROLL, WS_EX_CLIENTEDGE)
	sendMsg(h, WM_SETFONT, a.mono, 1)
	mk(bkOK, "BUTTON", T("OK"), WS_TABSTOP|BS_DEFPUSHBUTTON, 0)
	mk(bkCancel, "BUTTON", T("Cancel"), WS_TABSTOP, 0)

	var presets []string
	for _, p := range Presets {
		presets = append(presets, T(p.Name))
	}
	presets = append(presets, T("(as set below)"))
	fillCombo(w.c[bkPreset], presets, 0)
	fillCombo(w.c[bkMemory], MemoryNames(), 2)
	fillCombo(w.c[bkCDDrv], []string{T("OAKCDROM.SYS (from Windows 98)"), T("UDVD2 (free, for UDMA controllers)")}, 0)
	fillCombo(w.c[bkNetCard], NetCardNames(), 0)
	w.layout()

	var bs BlockSet
	if edit >= 0 {
		it := a.proj.Items[edit]
		if it.Blocks != nil {
			bs = *it.Blocks
		} else {
			bs = Presets[0].Blocks
		}
		sendMsg(w.c[bkPreset], CB_SETCURSEL, uintptr(len(Presets)), 0)
		setText(w.c[bkID], it.ID)
		if it.Blocks == nil || it.Title != it.Blocks.Title() {
			setText(w.c[bkTitle], it.Title)
		}
	} else {
		bs = Presets[0].Blocks
		setText(w.c[bkID], a.proj.UniqueID(Presets[0].ID))
	}
	w.set(bs)
	enable(a.hwnd, false)
	pShowWindow.Call(w.hwnd, SW_SHOW)
}

func (w *blockWin) layout() {
	a := theApp
	s := a.s
	cw, ch := clientSize(w.hwnd)
	m, rh, lh := s(12), s(23), s(18)
	lp, bp, cp := s(6), s(24), s(28) // padding of labels, buttons, check boxes
	y := m
	col := fitW(s(110), lp, w.c[bkLblPreset], w.c[bkLblID], w.c[bkLblMemory])
	row := func(lbl, ctl, cwid, extra int) {
		move(w.c[lbl], m, y+s(4), col, lh)
		move(w.c[ctl], m+col, y, cwid, rh+extra)
	}
	row(bkLblPreset, bkPreset, cw-2*m-col, s(200))
	y += rh + s(8)
	move(w.c[bkLblID], m, y+s(4), col, lh)
	move(w.c[bkID], m+col, y, s(100), rh)
	x := m + col + s(120)
	tl := fitW(s(120), lp, w.c[bkLblTitle])
	move(w.c[bkLblTitle], x, y+s(4), tl, lh)
	move(w.c[bkTitle], x+tl, y, cw-m-x-tl, rh)
	y += rh + s(8)
	row(bkLblMemory, bkMemory, s(300), s(120))
	y += rh + s(10)
	// the check boxes on the left, their settings in a second column
	cb := fitW(s(300), cp, w.c[bkMouse], w.c[bkSound], w.c[bkNet])
	c2 := m + cb + s(10)
	move(w.c[bkMouse], m, y, cb, rh)
	y += rh + s(4)
	x = m
	cdw := fitW(s(110), cp, w.c[bkCD])
	move(w.c[bkCD], x, y, cdw, rh)
	x += cdw + s(10)
	l1 := fitW(s(60), lp, w.c[bkLblCDDrv])
	move(w.c[bkLblCDDrv], x, y+s(4), l1, lh)
	x += l1
	move(w.c[bkCDDrv], x, y, s(280), rh+s(80))
	x += s(280) + s(20)
	l2 := fitW(s(60), lp, w.c[bkLblLetter])
	move(w.c[bkLblLetter], x, y+s(4), l2, lh)
	move(w.c[bkCDLetter], x+l2, y, s(36), rh)
	y += rh + s(4)
	move(w.c[bkSound], m, y, cb, rh)
	l3 := fitW(s(70), lp, w.c[bkLblBlaster])
	move(w.c[bkLblBlaster], c2, y+s(4), l3, lh)
	move(w.c[bkBlaster], c2+l3, y, cw-m-c2-l3, rh)
	y += rh + s(2)
	move(w.c[bkPnP], m+s(24), y, cw-2*m-s(24), rh)
	y += rh + s(4)
	move(w.c[bkNet], m, y, cb, rh)
	l4 := fitW(s(50), lp, w.c[bkLblCard])
	move(w.c[bkLblCard], c2, y+s(4), l4, lh)
	move(w.c[bkNetCard], c2+l4, y, cw-m-c2-l4, rh+s(80))
	y += rh + s(2)
	l5 := fitW(s(90), lp, w.c[bkLblArgs])
	move(w.c[bkLblArgs], m+s(24), y+s(4), l5, lh)
	move(w.c[bkNetArgs], m+s(24)+l5, y, s(180), rh)
	ecx := m + s(24) + l5 + s(196)
	if ecx < c2 {
		ecx = c2
	}
	move(w.c[bkEC], ecx, y, fitW(s(300), cp, w.c[bkEC]), rh)
	y += rh + s(10)
	move(w.c[bkLblPreview], m, y, cw-2*m, lh)
	y += lh + s(2)
	move(w.c[bkPreview], m, y, cw-2*m, ch-y-2*m-s(28))
	bw := fitW(s(110), bp, w.c[bkOK], w.c[bkCancel])
	move(w.c[bkOK], cw-m-2*bw-s(8), ch-m-s(28), bw, s(28))
	move(w.c[bkCancel], cw-m-bw, ch-m-s(28), bw, s(28))
}

func checked(h uintptr) bool { return sendMsg(h, BM_GETCHECK, 0, 0) == 1 }

func setCheck(h uintptr, on bool) {
	v := uintptr(0)
	if on {
		v = 1
	}
	sendMsg(h, BM_SETCHECK, v, 0)
}

func indexOf(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return 0
}

func (w *blockWin) set(b BlockSet) {
	b.Normalize()
	w.loading = true
	sendMsg(w.c[bkMemory], CB_SETCURSEL, uintptr(indexOf(MemoryChoices, b.Memory)), 0)
	setCheck(w.c[bkMouse], b.Mouse)
	setCheck(w.c[bkCD], b.CDROM)
	sendMsg(w.c[bkCDDrv], CB_SETCURSEL, uintptr(indexOf(CDDrivers, b.CDDriver)), 0)
	setText(w.c[bkCDLetter], b.CDLetter)
	setCheck(w.c[bkSound], b.Sound)
	setText(w.c[bkBlaster], b.Blaster)
	setCheck(w.c[bkPnP], b.PnP)
	setCheck(w.c[bkNet], b.Net)
	ids := make([]string, len(netCards))
	for i, c := range netCards {
		ids[i] = c.id
	}
	sendMsg(w.c[bkNetCard], CB_SETCURSEL, uintptr(indexOf(ids, b.NetCard)), 0)
	setText(w.c[bkNetArgs], b.NetArgs)
	setCheck(w.c[bkEC], b.EC)
	w.loading = false
	w.update()
}

func (w *blockWin) get() BlockSet {
	b := BlockSet{
		Memory:   MemoryChoices[clampIdx(int(sendMsg(w.c[bkMemory], CB_GETCURSEL, 0, 0)), len(MemoryChoices))],
		Mouse:    checked(w.c[bkMouse]),
		CDROM:    checked(w.c[bkCD]),
		CDDriver: CDDrivers[clampIdx(int(sendMsg(w.c[bkCDDrv], CB_GETCURSEL, 0, 0)), len(CDDrivers))],
		CDLetter: getText(w.c[bkCDLetter]),
		Sound:    checked(w.c[bkSound]),
		Blaster:  getText(w.c[bkBlaster]),
		PnP:      checked(w.c[bkPnP]),
		Net:      checked(w.c[bkNet]),
		NetCard:  netCards[clampIdx(int(sendMsg(w.c[bkNetCard], CB_GETCURSEL, 0, 0)), len(netCards))].id,
		NetArgs:  getText(w.c[bkNetArgs]),
		EC:       checked(w.c[bkEC]),
	}
	b.Normalize()
	return b
}

func clampIdx(i, n int) int {
	if i < 0 || i >= n {
		return 0
	}
	return i
}

// update enables the sub-fields and shows the lines.
func (w *blockWin) update() {
	if w.loading {
		return
	}
	a := theApp
	b := w.get()
	for _, id := range []int{bkCDDrv, bkCDLetter} {
		enable(w.c[id], b.CDROM)
	}
	for _, id := range []int{bkBlaster, bkPnP} {
		enable(w.c[id], b.Sound)
	}
	for _, id := range []int{bkNetCard, bkNetArgs, bkEC} {
		enable(w.c[id], b.Net)
	}
	cfg, auto, drv := b.BuildFor(a.proj.Target, a.proj.Machine)
	var t strings.Builder
	t.WriteString("[" + a.proj.ConfigFile() + "]\r\n")
	for _, l := range cfg {
		t.WriteString(l + "\r\n")
	}
	t.WriteString("\r\n[" + a.proj.AutoexecFile() + "]\r\n")
	for _, l := range auto {
		t.WriteString(l + "\r\n")
	}
	if len(drv) > 0 {
		var names []string
		for _, id := range drv {
			if d := a.store.Find(id); d != nil {
				st := ""
				if !a.store.Ready(d) {
					st = " (" + T("missing") + ")"
				}
				names = append(names, d.Name+st)
			}
		}
		t.WriteString("\r\n" + T("Drivers:") + "\r\n  " + strings.Join(names, "\r\n  ") + "\r\n")
	}
	setText(w.c[bkPreview], t.String())
}

func (w *blockWin) presetChosen() {
	a := theApp
	i := int(sendMsg(w.c[bkPreset], CB_GETCURSEL, 0, 0))
	if i < 0 || i >= len(Presets) {
		return
	}
	w.set(Presets[i].Blocks)
	if w.edit < 0 {
		setText(w.c[bkID], a.proj.UniqueID(Presets[i].ID))
	}
	setText(w.c[bkTitle], "")
}

func (w *blockWin) close() {
	a := theApp
	enable(a.hwnd, true)
	pDestroyWindow.Call(w.hwnd)
	a.blocks = nil
	pSetForegroundWindow.Call(a.hwnd)
}

func (w *blockWin) ok() {
	a := theApp
	p := a.proj
	id := strings.ToUpper(strings.TrimSpace(getText(w.c[bkID])))
	if err := ValidID(id); err != nil {
		msgBox(w.hwnd, err.Error(), appTitle(), MB_ICONWARNING)
		return
	}
	for i, it := range p.Items {
		if it.ID == id && i != w.edit {
			msgBox(w.hwnd, T("The ID %s is used twice.", id), appTitle(), MB_ICONWARNING)
			return
		}
	}
	b := w.get()
	it, drv := ItemFromBlocks(id, strings.TrimSpace(getText(w.c[bkTitle])), b, p.Target, p.Machine)
	if w.edit >= 0 {
		old := &p.Items[w.edit]
		if !old.BlocksMatch(p.Target, p.Machine) {
			if msgBox(w.hwnd, T("The lines of %s were edited by hand. Replace them with the lines of the building blocks?", old.ID), appTitle(), MB_YESNO|MB_ICONQUESTION) != IDYES {
				return
			}
		}
		p.Items[w.edit] = it
		a.sel = w.edit + 1
	} else {
		if len(p.Items) >= MaxItems {
			msgBox(w.hwnd, T("At most %d menu items are allowed (there are %d).", MaxItems, len(p.Items)), appTitle(), MB_ICONWARNING)
			return
		}
		pos := len(p.Items)
		if a.sel >= 1 && a.sel <= len(p.Items) {
			pos = a.sel
		}
		p.Items = append(p.Items[:pos], append([]Item{it}, p.Items[pos:]...)...)
		if len(p.Items) > 1 && p.Default >= pos {
			p.Default++
		}
		a.sel = pos + 1
	}
	p.AddDrivers(drv)
	w.close()
	a.setDirty()
	a.selectRow(a.sel)
	if a.drv != nil {
		a.drv.refresh()
	}
}

func blocksProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	w := a.blocks
	switch msg {
	case WM_COMMAND:
		if w == nil {
			return 0
		}
		code := hiword(wParam)
		switch int(loword(wParam)) {
		case bkPreset:
			if code == CBN_SELCHANGE {
				w.presetChosen()
			}
		case bkMemory, bkCDDrv, bkNetCard:
			if code == CBN_SELCHANGE {
				if int(loword(wParam)) == bkNetCard {
					// new card: its usual parameters
					c := netCards[clampIdx(int(sendMsg(w.c[bkNetCard], CB_GETCURSEL, 0, 0)), len(netCards))]
					setText(w.c[bkNetArgs], c.args)
				}
				w.markCustom()
				w.update()
			}
		case bkMouse, bkCD, bkSound, bkPnP, bkNet, bkEC:
			if code == BN_CLICKED {
				w.markCustom()
				w.update()
			}
		case bkCDLetter, bkBlaster, bkNetArgs:
			if code == EN_CHANGE {
				w.update()
			}
		case bkOK:
			w.ok()
		case bkCancel:
			w.close()
		case IDCANCEL:
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

// markCustom switches the preset box to "(as set below)" after a change.
func (w *blockWin) markCustom() {
	if !w.loading {
		sendMsg(w.c[bkPreset], CB_SETCURSEL, uintptr(len(Presets)), 0)
	}
}
