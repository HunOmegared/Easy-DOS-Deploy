//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"unsafe"
)

// The main window: the steps of the project on the left, the page of the
// chosen step on the right. The boot menu editor is the main window's own
// controls; the other pages are panels (child windows).

const (
	pageDisk = iota
	pageSystem
	pageDrivers
	pageMenu
	pageBuild
	pageCount
)

const (
	WM_MEASUREITEM      = 0x002C
	WM_DRAWITEM         = 0x002B
	LBS_OWNERDRAWFIXED  = 0x0010
	LBS_HASSTRINGS      = 0x0040
	WS_EX_CONTROLPARENT = 0x00010000
	WS_CLIPCHILDREN     = 0x02000000
	ODS_SELECTED        = 0x0001
	DT_LEFT             = 0x0000
	DT_VCENTER          = 0x0004
	DT_SINGLELINE       = 0x0020
	DT_NOPREFIX         = 0x0800
	DT_END_ELLIPSIS     = 0x8000
	TRANSPARENT         = 1
	COLOR_WINDOW        = 5
	COLOR_GRAYTEXT      = 17
	LB_SETITEMHEIGHT    = 0x01A0
	BS_RADIOBUTTON      = 0x0004
)

var (
	pFillRect         = user32.NewProc("FillRect")
	pDrawTextW        = user32.NewProc("DrawTextW")
	pGetSysColor      = user32.NewProc("GetSysColor")
	pGetSysColorBrush = user32.NewProc("GetSysColorBrush")
	pInvalidateRect   = user32.NewProc("InvalidateRect")
	pSetTextColor     = gdi32.NewProc("SetTextColor")
	pSetBkMode        = gdi32.NewProc("SetBkMode")
	pCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	pImageListDraw    = comctl32.NewProc("ImageList_Draw")
	pCreateFontBold   = gdi32.NewProc("CreateFontW")
)

type measureItem struct {
	ctlType, ctlID, itemID, itemWidth, itemHeight uint32
	itemData                                      uintptr
}

type drawItem struct {
	ctlType, ctlID, itemID, itemAction, itemState uint32
	hwndItem                                      uintptr
	hdc                                           uintptr
	rc                                            rect
	itemData                                      uintptr
}

func rgbColor(r, g, b uint32) uint32 { return r | g<<8 | b<<16 }

// makeFontWeight is makeFont with a weight (700: bold).
func makeFontWeight(face string, pt, dpi, weight int) uintptr {
	h := -(pt*dpi + 36) / 72
	f, _, _ := pCreateFontBold.Call(uintptr(int32(h)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(u16(face))))
	if f == 0 {
		return makeFont(face, pt, dpi)
	}
	return f
}

// newPanel makes a page: a hidden child of the main window that passes
// the keyboard on (Tab) to its controls.
func (a *app) newPanel(class string) uintptr {
	return createWindow(WS_EX_CONTROLPARENT, class, "", WS_CHILD|WS_CLIPCHILDREN, 0, 0, 10, 10, a.hwnd, 0)
}

// createPages makes the pages that are panels.
func (a *app) createPages() {
	a.createBootPages()
	a.createDriversPage()
	a.layoutPages()
}

func (a *app) destroyPages() {
	if a.boot != nil {
		for _, p := range a.boot.panels {
			pDestroyWindow.Call(p)
		}
		a.boot = nil
	}
	if a.drv != nil {
		pDestroyWindow.Call(a.drv.hwnd)
		a.drv = nil
	}
}

// pagePanel is the panel of a page (0: the editor's own controls).
func (a *app) pagePanel(p int) uintptr {
	switch {
	case a.boot != nil && p == pageDisk:
		return a.boot.panels[bpDisk]
	case a.boot != nil && p == pageSystem:
		return a.boot.panels[bpSystem]
	case a.boot != nil && p == pageBuild:
		return a.boot.panels[bpBuild]
	case a.drv != nil && p == pageDrivers:
		return a.drv.hwnd
	}
	return 0
}

// menuPageIDs are the editor's controls, shown only on the menu page.
var menuPageIDs = []int{idLblTarget, idTarget, idLblTimeout, idTimeout, idLblDefault, idDefault, idLblCodepage, idCodepage,
	idLblColor, idColor, idLblCfgName, idCfgName, idLblAutoName, idAutoName, idList, idAdd, idBlocks, idCopy, idDel, idUp, idDown,
	idLblID, idID, idLblTitle, idTitle, idLblConfig, idConfig, idLblAutoexec, idAutoexec, idHint, idPreview, idSaveDir, idWriteImg, idMenuHead, idStatus}

// showPage switches to a page.
func (a *app) showPage(p int) {
	if p < 0 || p >= pageCount {
		return
	}
	a.commit()
	a.page = p
	a.showMenuView()
	for q := 0; q < pageCount; q++ {
		if h := a.pagePanel(q); h != 0 {
			v := uintptr(0)
			if q == p {
				v = SW_SHOW
			}
			pShowWindow.Call(h, v)
		}
	}
	switch p {
	case pageDrivers:
		a.drv.refresh()
	case pageBuild:
		a.boot.fillMenus(a.boot.menuChoice())
		a.boot.updateDrivers()
	}
	sendMsg(a.c[idSide], LB_SETCURSEL, uintptr(p), 0)
	a.updateSide()
}

// updateSide redraws the steps with their current state.
func (a *app) updateSide() {
	if h := a.c[idSide]; h != 0 {
		pInvalidateRect.Call(h, 0, 1)
	}
}

// layoutPages places the panels next to the steps.
func (a *app) layoutPages() {
	w, h := clientSize(a.hwnd)
	x := a.sideWidth()
	for q := 0; q < pageCount; q++ {
		if p := a.pagePanel(q); p != 0 {
			move(p, x, 0, w-x, h)
		}
	}
}

func (a *app) sideWidth() int { return a.s(236) }

// stepTitles are the names of the pages, in order.
func stepTitles() []string {
	return []string{T("Machine and disk"), T("System"), T("Drivers"), T("Boot menu"), T("Build")}
}

// stepState is the icon and the one-line summary of a step.
func (a *app) stepState(p int) (int, string) {
	b := a.boot
	switch p {
	case pageDisk:
		if b == nil {
			return iconDisk, ""
		}
		o := b.options()
		if o.Kind != MediaHDD {
			return iconOK, floppyFormats[o.Kind].name
		}
		if o.Geom.C == 0 {
			return iconWarn, T("Geometry missing")
		}
		pl, err := PlanHDD(o.Geom)
		if err != nil {
			return iconWarn, T("Check the geometry")
		}
		if o.OnlyC {
			pl.OnlyC()
		}
		var drives []string
		for i := range pl.Drives() {
			drives = append(drives, fmt.Sprintf("%c:", 'C'+i))
		}
		return iconOK, fmt.Sprintf("%d/%d/%d, %s", o.Geom.C, o.Geom.H, o.Geom.S, strings.Join(drives, " "))
	case pageSystem:
		if b == nil {
			return iconWarn, ""
		}
		switch {
		case checked(b.c[bwBlank]):
			return iconOK, T("Empty: you install DOS")
		case checked(b.c[bwFreeDOS]):
			return iconOK, T("FreeDOS, downloaded")
		case b.setupMode() && b.setup != nil:
			v := b.setup.Version
			if v == "" {
				v = "MS-DOS"
			}
			return iconOK, T("%s, Setup disks", strings.TrimPrefix(v, "Microsoft "))
		case b.setupMode():
			return iconWarn, T("Setup disks not given")
		}
		if src := b.options().Source; src != "" {
			return iconOK, filepath.Base(src)
		}
		return iconWarn, T("No system chosen")
	case pageDrivers:
		fd := b != nil && checked(b.c[bwFreeDOS])
		dl, br := DriverNeeds(a.proj, a.store, fd)
		switch {
		case len(br) > 0:
			return iconBrowse, T("%d chosen, %d to browse for", len(a.proj.Drivers), len(br))
		case len(dl) > 0:
			return iconDownload, T("%d chosen, %d to download", len(a.proj.Drivers), len(dl))
		}
		return iconOK, T("%d chosen, all here", len(a.proj.Drivers))
	case pageMenu:
		for _, x := range a.proj.Validate() {
			if x.Fatal {
				return iconWarn, x.Msg
			}
		}
		if len(a.proj.Items) == 1 {
			return iconMenu, T("1 menu item")
		}
		return iconMenu, T("%d menu items", len(a.proj.Items))
	case pageBuild:
		return iconBuild, T("Image file, then the card")
	}
	return iconOK, ""
}

func (a *app) measureSide(mi *measureItem) {
	mi.itemHeight = uint32(a.s(52))
}

// drawSide draws one step: icon, title, summary.
func (a *app) drawSide(di *drawItem) {
	if di.itemID >= pageCount {
		return
	}
	p := int(di.itemID)
	sel := di.itemState&ODS_SELECTED != 0
	bg, _, _ := pGetSysColorBrush.Call(COLOR_WINDOW)
	title, sub := rgbColor(0x20, 0x20, 0x20), rgbColor(0x70, 0x70, 0x70)
	if sel {
		bg, _, _ = pCreateSolidBrush.Call(uintptr(rgbColor(0xE6, 0xF1, 0xFB)))
		defer pDeleteObject.Call(bg)
		title, sub = rgbColor(0x0C, 0x44, 0x7C), rgbColor(0x18, 0x5F, 0xA5)
	}
	pFillRect.Call(di.hdc, uintptr(unsafe.Pointer(&di.rc)), bg)
	if sel {
		bar := di.rc
		bar.right = bar.left + int32(a.s(4))
		accent, _, _ := pCreateSolidBrush.Call(uintptr(rgbColor(0x18, 0x5F, 0xA5)))
		pFillRect.Call(di.hdc, uintptr(unsafe.Pointer(&bar)), accent)
		pDeleteObject.Call(accent)
	}
	icon, text := a.stepState(p)
	is := a.s(24)
	x := int(di.rc.left) + a.s(14)
	top := int(di.rc.top)
	h := int(di.rc.bottom - di.rc.top)
	pImageListDraw.Call(a.sideIcons, uintptr(icon), di.hdc, uintptr(x), uintptr(top+(h-is)/2), 0)
	tx := x + is + a.s(12)
	pSetBkMode.Call(di.hdc, TRANSPARENT)
	line := func(s string, font uintptr, col uint32, y0, y1 int) {
		old, _, _ := pSelectObject.Call(di.hdc, font)
		pSetTextColor.Call(di.hdc, uintptr(col))
		r := rect{int32(tx), int32(y0), di.rc.right - int32(a.s(8)), int32(y1)}
		pDrawTextW.Call(di.hdc, uintptr(unsafe.Pointer(u16(s))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), DT_LEFT|DT_VCENTER|DT_SINGLELINE|DT_NOPREFIX|DT_END_ELLIPSIS)
		pSelectObject.Call(di.hdc, old)
	}
	num := fmt.Sprintf("%d. ", p+1)
	if p == pageBuild {
		num = ""
	}
	line(num+stepTitles()[p], a.bold, title, top+a.s(6), top+h/2+a.s(1))
	line(text, a.font, sub, top+h/2-a.s(1), top+h-a.s(6))
}

// fillSide puts the steps into the list.
func (a *app) fillSide() {
	h := a.c[idSide]
	sendMsg(h, LB_RESETCONTENT, 0, 0)
	for _, t := range stepTitles() {
		sendMsg(h, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(t))))
	}
	sendMsg(h, LB_SETCURSEL, uintptr(a.page), 0)
}

// setMenuFrom remembers which ready menu the project's menu is (an ID
// such as "games-ms").
func (a *app) setMenuFrom(id string) {
	a.menuFrom = strings.TrimSuffix(strings.TrimSuffix(id, "-ms"), "-fd")
	a.menuEdited = false
}

// The menu page has a simple view (the ready menus) and the full editor.
var simpleMenuIDs = []int{idMenuIntro, idTpl0, idTpl1, idTpl2, idTpl3, idTplOwn, idTplNote0, idTplNote1, idTplNote2, idTplNote3, idTplOwnNote, idEditMenu}

// editorIDs are the controls of the full editor (not the heading).
func editorIDs() []int {
	var out []int
	for _, id := range menuPageIDs {
		if id != idMenuHead {
			out = append(out, id)
		}
	}
	return append(out, idSimpleView)
}

// showMenuView shows the simple view or the editor of the menu page.
func (a *app) showMenuView() {
	on := a.page == pageMenu
	show := func(ids []int, v bool) {
		for _, id := range ids {
			x := uintptr(0)
			if v {
				x = SW_SHOW
			}
			pShowWindow.Call(a.c[id], x)
		}
	}
	show([]int{idMenuHead}, on)
	show(editorIDs(), on && a.set.MenuEditor)
	show(simpleMenuIDs, on && !a.set.MenuEditor)
	if on && !a.set.MenuEditor {
		a.fillSimpleMenu()
	}
}

// fillSimpleMenu shows which ready menu is chosen and what each offers.
func (a *app) fillSimpleMenu() {
	suffix := "-ms"
	if a.proj.Target == TargetFreeDOS {
		suffix = "-fd"
	}
	sel := -1
	for i, id := range wizMenus {
		t := TemplateByID(id + suffix)
		var titles []string
		for _, it := range t.Make().Items {
			titles = append(titles, it.Title)
		}
		setText(a.c[idTplNote0+i], T(t.Desc)+"\n"+T("Menu items:")+" "+strings.Join(titles, " • "))
		if id == a.menuFrom && !a.menuEdited {
			sel = i
		}
	}
	own := a.menuFrom == "" || a.menuEdited
	var titles []string
	for _, it := range a.proj.Items {
		titles = append(titles, title(it))
	}
	setText(a.c[idTplOwnNote], T("Edited in the editor or read from a project file.")+"\n"+T("Menu items:")+" "+strings.Join(titles, " • "))
	v := uintptr(0)
	if own {
		v = SW_SHOW
		sel = len(wizMenus)
	}
	pShowWindow.Call(a.c[idTplOwn], v)
	pShowWindow.Call(a.c[idTplOwnNote], v)
	for i := 0; i <= len(wizMenus); i++ {
		setCheck(a.c[idTpl0+i], i == sel)
	}
	a.layoutSimpleMenu()
}

func (a *app) layoutSimpleMenu() {
	w, _ := clientSize(a.hwnd)
	s := a.s
	x := a.sideWidth() + s(14)
	cw := w - x - s(14)
	lh := s(18)
	move(a.c[idMenuIntro], x, s(46), cw, 2*lh)
	y := s(46) + 2*lh + s(14)
	for i := 0; i <= len(wizMenus); i++ {
		if i == len(wizMenus) && !(a.menuFrom == "" || a.menuEdited) {
			break
		}
		move(a.c[idTpl0+i], x, y, cw, s(23))
		y += s(23)
		move(a.c[idTplNote0+i], x+s(22), y, cw-s(22), 2*lh)
		y += 2*lh + s(12)
	}
	bw := fitW(s(160), s(24), a.c[idEditMenu])
	move(a.c[idEditMenu], x, y+s(4), bw, s(30))
}

func (a *app) simpleMenuCommand(id int) {
	switch {
	case id == idEditMenu:
		a.set.MenuEditor = true
		saveSettings(a.data, a.set)
		a.showPage(pageMenu)
	case id == idSimpleView:
		a.commit()
		a.set.MenuEditor = false
		saveSettings(a.data, a.set)
		a.showPage(pageMenu)
	case id == idTplOwn:
		a.fillSimpleMenu()
	default:
		a.readyMenu(id - idTpl0)
	}
}

// readyMenu replaces the menu with a ready one (c: index into wizMenus).
func (a *app) readyMenu(c int) {
	if c < 0 || c >= len(wizMenus) {
		return
	}
	if wizMenus[c] == a.menuFrom && !a.menuEdited {
		return
	}
	if a.menuEdited && msgBox(a.hwnd, T("Replace your edited menu with this ready one?"), appTitle(), MB_YESNO|MB_ICONQUESTION) != IDYES {
		a.fillSimpleMenu()
		return
	}
	suffix := "-ms"
	t := a.proj.Target
	if a.boot != nil {
		t = a.boot.sourceTarget()
	}
	if t == TargetFreeDOS {
		suffix = "-fd"
	}
	// only the menu changes; the project (and its file) stays
	a.proj, a.sel = TemplateByID(wizMenus[c]+suffix).Make(), 1
	a.setMenuFrom(wizMenus[c])
	a.loadProject()
	a.setDirty()
	if a.boot != nil {
		a.boot.applyOwn()
		a.boot.fillMenus(0)
	}
	a.fillSimpleMenu()
	a.updateSide()
}
