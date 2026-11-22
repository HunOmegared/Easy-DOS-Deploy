//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// appTitle is the name of the program; a name, so it is not translated.
func appTitle() string { return "Easy DOS Deploy" }

const cwDefault = -0x80000000 // CW_USEDEFAULT

// control IDs of the main window
const (
	idList = 100 + iota
	idID
	idTitle
	idConfig
	idAutoexec
	idTarget
	idTimeout
	idDefault
	idCodepage
	idColor
	idCfgName
	idAutoName
	idAdd
	idCopy
	idDel
	idUp
	idDown
	idPreview
	idSaveDir
	idWriteImg
	idStatus
	idLblConfig
	idLblAutoexec
	idLblID
	idLblTitle
	idHint
	idLblTarget
	idLblTimeout
	idLblDefault
	idLblCodepage
	idLblColor
	idLblCfgName
	idLblAutoName
	idBlocks
)

// menu IDs
const (
	mOpen = 2000 + iota
	mSave
	mSaveAs
	mExit
	mReadDir
	mReadImg
	mPreview
	mSaveDir
	mWriteImg
	mDrivers
	mDrvFolder
	mAbout
	mLangExport
	mLangFolder
	mLangReload
	mNewImage
	mWizard
	mGeomSave
	mGeomFloppy
	mGeomFDFloppy
	mBackupCard
	mWriteCard
	mTemplate = 3000 // + index
	mLanguage = 3500 // + index into langMenu
)

type app struct {
	hwnd    uintptr
	font    uintptr
	mono    uintptr
	dpi     int
	c       map[int]uintptr
	proj    *Project
	path    string
	dirty   bool
	sel     int // 0 = common start, 1..n = items, n+1 = common end
	loading bool
	store   *Store
	exeDir  string
	data    string
	set     Settings

	langMenu []*Language

	drv      *drvWin
	edit     *editWin
	boot     *bootWin
	pick     *pickWin
	job      *job
	confirm  *confirmWin
	blocks   *blockWin
	wiz      *wizardWin
	calc     *calcWin
	setupDlg *setupWin
	preview  uintptr
	prevC    map[int]uintptr
}

var theApp *app

func (a *app) s(n int) int { return n * a.dpi / 96 }

func (a *app) langDirs() []string {
	dirs := []string{filepath.Join(a.exeDir, "lang")}
	if a.data != a.exeDir {
		dirs = append(dirs, filepath.Join(a.data, "lang"))
	}
	return dirs
}

func (a *app) loadLanguageFiles() []error {
	var errs []error
	for _, d := range a.langDirs() {
		errs = append(errs, LoadLanguages(d)...)
	}
	return errs
}

func runGUI() {
	runtime.LockOSThread()
	pSetProcessDPIAware.Call()
	pCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED, for the folder dialog

	exe, _ := os.Executable()
	a := &app{c: map[int]uintptr{}, dpi: screenDPI(), exeDir: filepath.Dir(exe)}
	a.data = dataDir(a.exeDir)
	a.set = loadSettings(a.data)
	langErrs := a.loadLanguageFiles()
	// English unless the user chose another language in the Language menu
	if a.set.Lang == "" || !SetLanguage(a.set.Lang) {
		SetLanguage("en")
	}
	a.store = NewStore(filepath.Join(a.data, "drivers"))
	a.font = makeFont("Segoe UI", 9, a.dpi)
	a.mono = makeFont("Consolas", 10, a.dpi)
	theApp = a
	a.proj = DefaultProject(TargetMSDOS)

	registerClass("EasyDOSDeployMain", syscall.NewCallback(mainProc))
	registerClass("EasyDOSDeployPreview", syscall.NewCallback(previewProc))
	registerClass("EasyDOSDeployDrivers", syscall.NewCallback(driversProc))
	registerClass("EasyDOSDeployDrvEdit", syscall.NewCallback(editProc))
	registerClass("EasyDOSDeployBoot", syscall.NewCallback(bootProc))
	registerClass("EasyDOSDeployPick", syscall.NewCallback(pickProc))
	registerClass("EasyDOSDeployJob", syscall.NewCallback(jobProc))
	registerClass("EasyDOSDeployConfirm", syscall.NewCallback(confirmProc))
	registerClass("EasyDOSDeployBlocks", syscall.NewCallback(blocksProc))
	registerClass("EasyDOSDeployWizard", syscall.NewCallback(wizardProc))
	registerClass("EasyDOSDeployCalc", syscall.NewCallback(calcProc))
	registerClass("EasyDOSDeploySetup", syscall.NewCallback(setupProc))

	a.hwnd = createWindow(0, "EasyDOSDeployMain", appTitle(), WS_OVERLAPPEDWINDOW,
		cwDefault, cwDefault, a.s(980), a.s(680), 0, 0)
	a.buildMenu()
	a.build()
	a.applyTexts()
	a.loadProject()
	pShowWindow.Call(a.hwnd, SW_SHOW)
	pUpdateWindow.Call(a.hwnd)

	if err := a.store.Reload(); err != nil {
		msgBox(a.hwnd, err.Error(), appTitle(), MB_ICONWARNING)
	}
	if len(langErrs) > 0 {
		var t []string
		for _, e := range langErrs {
			t = append(t, e.Error())
		}
		msgBox(a.hwnd, T("Some language files could not be read:")+"\n\n"+strings.Join(t, "\n"), appTitle(), MB_ICONWARNING)
	}
	a.handleArgs(os.Args[1:])
	if len(os.Args) == 1 && !a.set.HideWizard {
		a.openWizard()
	}

	var m msgT
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if m.message == WM_KEYDOWN && m.wParam == 'S' && ctrlDown() {
			a.saveProject(false)
			continue
		}
		handled := false
		for _, h := range []uintptr{a.hwnd, a.preview, a.drvHwnd(), a.editHwnd(), a.bootHwnd(), a.pickHwnd(), a.confirmHwnd(), a.blocksHwnd(), a.wizardHwnd(), a.calcHwnd()} {
			if h != 0 {
				if r, _, _ := pIsDialogMessageW.Call(h, uintptr(unsafe.Pointer(&m))); r != 0 {
					handled = true
					break
				}
			}
		}
		if !handled {
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}
}

func (a *app) drvHwnd() uintptr {
	if a.drv != nil {
		return a.drv.hwnd
	}
	return 0
}

func (a *app) bootHwnd() uintptr {
	if a.boot != nil {
		return a.boot.hwnd
	}
	return 0
}

func (a *app) calcHwnd() uintptr {
	if a.calc != nil {
		return a.calc.hwnd
	}
	return 0
}

func (a *app) blocksHwnd() uintptr {
	if a.blocks != nil {
		return a.blocks.hwnd
	}
	return 0
}

func (a *app) confirmHwnd() uintptr {
	if a.confirm != nil {
		return a.confirm.hwnd
	}
	return 0
}

func (a *app) pickHwnd() uintptr {
	if a.pick != nil {
		return a.pick.hwnd
	}
	return 0
}

// handleArgs opens a project given on the command line, or restores the
// project after a restart as administrator.
func (a *app) handleArgs(args []string) {
	restore, path := "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--restore":
			if i+1 < len(args) {
				restore = args[i+1]
				i++
			}
		case "--path":
			if i+1 < len(args) {
				path = args[i+1]
				i++
			}
		default:
			a.openProjectFile(args[i])
			return
		}
	}
	if restore != "" {
		if p, err := LoadProject(restore); err == nil {
			a.proj, a.path, a.dirty, a.sel = p, path, path == "", 1
			a.loadProject()
			os.Remove(restore)
		}
	}
}

func (a *app) editHwnd() uintptr {
	if a.edit != nil {
		return a.edit.hwnd
	}
	return 0
}

func (a *app) buildMenu() {
	old, _, _ := pGetMenu.Call(a.hwnd)
	bar, _, _ := pCreateMenu.Call()
	add := func(menu uintptr, id int, text string, checked bool) {
		f := uintptr(MF_STRING)
		if checked {
			f |= MF_CHECKED
		}
		pAppendMenuW.Call(menu, f, uintptr(id), uintptr(unsafe.Pointer(u16(text))))
	}
	sep := func(menu uintptr) { pAppendMenuW.Call(menu, MF_SEPARATOR, 0, 0) }
	popup := func(text string) uintptr {
		m, _, _ := pCreatePopupMenu.Call()
		pAppendMenuW.Call(bar, MF_POPUP, m, uintptr(unsafe.Pointer(u16(text))))
		return m
	}
	file := popup(T("&File"))
	add(file, mWizard, T("Start &wizard..."), false)
	sep(file)
	tm, _, _ := pCreatePopupMenu.Call()
	for i, t := range Templates {
		add(tm, mTemplate+i, T(t.Name), false)
	}
	pAppendMenuW.Call(file, MF_POPUP, tm, uintptr(unsafe.Pointer(u16(T("&New from template")))))
	sep(file)
	add(file, mOpen, T("&Open project..."), false)
	add(file, mSave, T("&Save project")+"\tCtrl+S", false)
	add(file, mSaveAs, T("Save project &as..."), false)
	sep(file)
	add(file, mExit, T("E&xit"), false)
	rd := popup(T("&Import"))
	add(rd, mReadImg, T("Existing menu from &disk image..."), false)
	add(rd, mReadDir, T("Existing menu from &folder..."), false)
	out := popup(T("&Output"))
	add(out, mPreview, T("&Preview"), false)
	add(out, mSaveDir, T("Save to &folder..."), false)
	add(out, mWriteImg, T("Write to disk &image..."), false)
	dk := popup(T("Dis&k image"))
	add(dk, mNewImage, T("&New bootable disk image..."), false)
	sep(dk)
	add(dk, mBackupCard, T("&Back up a card to an image file..."), false)
	add(dk, mWriteCard, T("&Write an image to a card..."), false)
	sep(dk)
	add(dk, mGeomSave, T("Save &GEOM.COM (shows the geometry on the target machine)..."), false)
	add(dk, mGeomFloppy, T("Put GEOM.COM onto a boot &floppy image..."), false)
	add(dk, mGeomFDFloppy, T("&Make a FreeDOS floppy with GEOM.COM..."), false)
	dr := popup(T("&Drivers"))
	add(dr, mDrivers, T("&Manage drivers..."), false)
	add(dr, mDrvFolder, T("Open driver &folder"), false)
	lm := popup(T("&Language"))
	a.langMenu = LanguageList()
	for i, l := range a.langMenu {
		add(lm, mLanguage+i, l.Name, l.Code == CurrentLanguage())
	}
	sep(lm)
	add(lm, mLangExport, T("&Make a translation file..."), false)
	add(lm, mLangReload, T("&Reload language files"), false)
	add(lm, mLangFolder, T("Open language &folder"), false)
	help := popup(T("&Help"))
	add(help, mAbout, T("&About and tips"), false)
	pSetMenu.Call(a.hwnd, bar)
	if old != 0 {
		pDestroyMenu.Call(old)
	}
	pDrawMenuBar.Call(a.hwnd)
}

func (a *app) ctl(id int, class string, style, ex uint32) uintptr {
	h := createWindow(ex, class, "", WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, a.hwnd, uintptr(id))
	sendMsg(h, WM_SETFONT, a.font, 1)
	a.c[id] = h
	return h
}

func cue(h uintptr, s string) { sendMsg(h, EM_SETCUE, 1, uintptr(unsafe.Pointer(u16(s)))) }

func fillCombo(h uintptr, items []string, sel int) {
	sendMsg(h, CB_RESETCONTENT, 0, 0)
	for _, s := range items {
		sendMsg(h, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(s))))
	}
	sendMsg(h, CB_SETCURSEL, uintptr(sel), 0)
}

func (a *app) build() {
	edit := uint32(WS_TABSTOP | ES_AUTOHSCROLL)
	multi := uint32(WS_TABSTOP | ES_MULTILINE | ES_AUTOVSCROLL | ES_AUTOHSCROLL | ES_WANTRETURN | WS_VSCROLL | WS_HSCROLL)
	combo := uint32(WS_TABSTOP | CBS_DROPDOWNLIST | WS_VSCROLL)

	for _, id := range []int{idLblTarget, idLblTimeout, idLblDefault, idLblCodepage, idLblColor, idLblCfgName, idLblAutoName,
		idLblID, idLblTitle, idLblConfig, idLblAutoexec, idHint, idStatus} {
		a.ctl(id, "STATIC", 0, 0)
	}
	a.ctl(idTarget, "COMBOBOX", combo, 0)
	a.ctl(idTimeout, "EDIT", edit, WS_EX_CLIENTEDGE)
	a.ctl(idDefault, "COMBOBOX", combo, 0)
	a.ctl(idCodepage, "COMBOBOX", combo, 0)
	a.ctl(idColor, "EDIT", edit, WS_EX_CLIENTEDGE)
	a.ctl(idCfgName, "EDIT", edit|ES_UPPERCASE, WS_EX_CLIENTEDGE)
	a.ctl(idAutoName, "EDIT", edit|ES_UPPERCASE, WS_EX_CLIENTEDGE)

	a.ctl(idList, "LISTBOX", WS_TABSTOP|LBS_NOTIFY|LBS_NOINTEGRALHEIGHT|WS_VSCROLL, WS_EX_CLIENTEDGE)
	for _, id := range []int{idAdd, idBlocks, idCopy, idDel, idUp, idDown} {
		a.ctl(id, "BUTTON", WS_TABSTOP, 0)
	}
	sendMsg(a.ctl(idID, "EDIT", edit|ES_UPPERCASE, WS_EX_CLIENTEDGE), EM_SETLIMIT, 8, 0)
	a.ctl(idTitle, "EDIT", edit, WS_EX_CLIENTEDGE)
	sendMsg(a.ctl(idConfig, "EDIT", multi, WS_EX_CLIENTEDGE), WM_SETFONT, a.mono, 1)
	sendMsg(a.ctl(idAutoexec, "EDIT", multi, WS_EX_CLIENTEDGE), WM_SETFONT, a.mono, 1)

	a.ctl(idPreview, "BUTTON", WS_TABSTOP, 0)
	a.ctl(idSaveDir, "BUTTON", WS_TABSTOP, 0)
	a.ctl(idWriteImg, "BUTTON", WS_TABSTOP, 0)
	a.layout()
}

// applyTexts (re)sets every text of the main window in the current language.
func (a *app) applyTexts() {
	a.loading = true
	defer func() { a.loading = false }()
	set := func(id int, s string) { setText(a.c[id], s) }
	set(idLblTarget, T("System:"))
	set(idLblTimeout, T("Timeout (s):"))
	set(idLblDefault, T("Default:"))
	set(idLblCodepage, T("Accents:"))
	set(idLblColor, T("Menu colour:"))
	set(idLblCfgName, T("CONFIG file:"))
	set(idLblAutoName, T("AUTOEXEC file:"))
	set(idLblID, T("ID:"))
	set(idLblTitle, T("Text in the menu:"))
	set(idAdd, T("New..."))
	set(idBlocks, T("Building blocks..."))
	set(idCopy, T("Copy"))
	set(idDel, T("Delete"))
	set(idUp, T("Up"))
	set(idDown, T("Down"))
	set(idPreview, T("Preview"))
	set(idSaveDir, T("Save to folder..."))
	set(idWriteImg, T("Write to disk image..."))
	cue(a.c[idTimeout], T("forever"))
	cue(a.c[idColor], T("e.g. 15,1"))
	tgt := 0
	cp := 0
	if a.proj != nil {
		tgt, cp = int(a.proj.Target), int(a.proj.Codepage)
	}
	fillCombo(a.c[idTarget], []string{"MS-DOS 6.x", "FreeDOS"}, tgt)
	fillCombo(a.c[idCodepage], []string{T("ASCII (no accents, safe)"), T("852 (Central European)"), T("437 (US)")}, cp)
	a.layout()
}

func (a *app) setLanguage(code string) {
	a.commit()
	if !SetLanguage(code) {
		return
	}
	a.set.Lang = code
	saveSettings(a.data, a.set)
	if !a.store.Custom {
		a.store.Reload() // built-in driver descriptions in the new language
	}
	// the small windows are rebuilt in the new language when next opened
	if a.preview != 0 {
		pDestroyWindow.Call(a.preview)
		a.preview = 0
	}
	if a.edit != nil {
		a.edit.close()
	}
	if a.drv != nil {
		pDestroyWindow.Call(a.drv.hwnd)
		a.drv = nil
	}
	if a.boot != nil {
		pDestroyWindow.Call(a.boot.hwnd)
		a.boot = nil
	}
	a.buildMenu()
	a.applyTexts()
	a.loadProject()
}

func (a *app) layout() {
	w, h := clientSize(a.hwnd)
	s := a.s
	m := s(10)
	rowH := s(23)
	lblH := s(18)
	x := m
	y := m
	place := func(lbl, lw, cw int, id int, comboExtra int) {
		lw = fitW(lw, s(6), a.c[lbl])
		move(a.c[lbl], x, y+s(4), lw, lblH)
		x += lw
		move(a.c[id], x, y, cw, rowH+comboExtra)
		x += cw + s(14)
	}
	place(idLblTarget, s(70), s(110), idTarget, s(200))
	place(idLblTimeout, s(98), s(60), idTimeout, 0)
	place(idLblDefault, s(90), s(240), idDefault, s(200))
	x, y = m, y+rowH+s(8)
	place(idLblCodepage, s(70), s(200), idCodepage, s(200))
	place(idLblColor, s(84), s(64), idColor, 0)
	place(idLblCfgName, s(80), s(104), idCfgName, 0)
	place(idLblAutoName, s(96), s(104), idAutoName, 0)

	top := y + rowH + s(12)
	bottomH := s(30)
	listBtns := []int{idAdd, idBlocks, idCopy, idDel, idUp, idDown}
	bw := (s(330) - 2*s(4)) / 3
	for _, id := range listBtns {
		bw = fitW(bw, s(20), a.c[id])
	}
	listW := 3*bw + 2*s(4)
	btnH := s(26)
	listH := h - top - bottomH - 2*btnH - s(4) - s(3)*m
	move(a.c[idList], m, top, listW, listH)
	for i, id := range listBtns {
		move(a.c[id], m+(i%3)*(bw+s(4)), top+listH+s(6)+(i/3)*(btnH+s(4)), bw, btnH)
	}

	ex := m + listW + m
	ew := w - ex - m
	yy := top
	lw1 := fitW(s(72), s(6), a.c[idLblID])
	lw2 := fitW(s(116), s(6), a.c[idLblTitle])
	move(a.c[idLblID], ex, yy+s(4), lw1, lblH)
	move(a.c[idID], ex+lw1, yy, s(90), rowH)
	tx := ex + lw1 + s(108)
	move(a.c[idLblTitle], tx, yy+s(4), lw2, lblH)
	move(a.c[idTitle], tx+lw2, yy, ex+ew-tx-lw2, rowH)
	yy += rowH + s(10)
	rest := h - yy - bottomH - 2*m - 2*lblH - s(26)
	cfgH := rest * 45 / 100
	autoH := rest - cfgH
	move(a.c[idLblConfig], ex, yy, ew, lblH)
	yy += lblH + s(2)
	move(a.c[idConfig], ex, yy, ew, cfgH)
	yy += cfgH + s(8)
	move(a.c[idLblAutoexec], ex, yy, ew, lblH)
	yy += lblH + s(2)
	move(a.c[idAutoexec], ex, yy, ew, autoH)
	yy += autoH + s(4)
	move(a.c[idHint], ex, yy, ew, lblH)

	by := h - m - bottomH
	bw2 := fitW(s(150), s(24), a.c[idWriteImg], a.c[idSaveDir], a.c[idPreview])
	move(a.c[idWriteImg], w-m-bw2, by, bw2, bottomH)
	move(a.c[idSaveDir], w-m-2*bw2-s(8), by, bw2, bottomH)
	move(a.c[idPreview], w-m-3*bw2-s(16), by, bw2, bottomH)
	move(a.c[idStatus], m, by+s(6), w-3*bw2-4*m, bottomH)
}

// ---- project <-> controls ----

func (a *app) itemCount() int { return len(a.proj.Items) }

func (a *app) current() *Item {
	switch {
	case a.sel <= 0:
		return &a.proj.Start
	case a.sel > a.itemCount():
		return &a.proj.End
	}
	return &a.proj.Items[a.sel-1]
}

func (a *app) loadProject() {
	a.loading = true
	p := a.proj
	sendMsg(a.c[idTarget], CB_SETCURSEL, uintptr(p.Target), 0)
	sendMsg(a.c[idCodepage], CB_SETCURSEL, uintptr(p.Codepage), 0)
	if p.Timeout >= 0 {
		setText(a.c[idTimeout], strconv.Itoa(p.Timeout))
	} else {
		setText(a.c[idTimeout], "")
	}
	setText(a.c[idColor], p.MenuColor)
	setText(a.c[idCfgName], p.ConfigName)
	setText(a.c[idAutoName], p.AutoexecName)
	a.loading = false
	if a.sel > a.itemCount()+1 {
		a.sel = 1
	}
	if a.itemCount() > 0 && a.sel == 0 {
		a.sel = 1
	}
	a.refreshList()
	a.loadEditor()
	a.refreshNames()
	a.updateStatus()
	a.updateTitle()
	if a.drv != nil {
		a.drv.refresh()
	}
}

func (a *app) refreshNames() {
	cue(a.c[idCfgName], a.defaultConfigName())
	cue(a.c[idAutoName], "AUTOEXEC.BAT")
	cfg := a.proj.ConfigFile()
	auto := a.proj.AutoexecFile()
	switch {
	case a.sel <= 0:
		setText(a.c[idLblConfig], T("%s lines for EVERY menu item, BEFORE the item's own lines:", cfg))
		setText(a.c[idLblAutoexec], T("%s lines for every menu item, BEFORE the branching:", auto))
	case a.sel > a.itemCount():
		setText(a.c[idLblConfig], T("%s lines for every menu item, AFTER the item's own lines:", cfg))
		setText(a.c[idLblAutoexec], T("%s lines for every menu item, at the END:", auto))
	default:
		setText(a.c[idLblConfig], T("%s lines for this menu item only:", cfg))
		setText(a.c[idLblAutoexec], T("%s lines for this menu item only:", auto))
	}
}

func (a *app) defaultConfigName() string {
	if a.proj.Target == TargetFreeDOS {
		return "FDCONFIG.SYS"
	}
	return "CONFIG.SYS"
}

func (a *app) listText(i int) string {
	n := a.itemCount()
	switch {
	case i == 0:
		return T("» Common part (before every item)")
	case i == n+1:
		return T("» Common part (after every item)")
	}
	it := a.proj.Items[i-1]
	mark := ""
	if i-1 == a.proj.Default {
		mark = "  " + T("[default]")
	}
	return fmt.Sprintf("%d. %s – %s%s", i, it.ID, title(it), mark)
}

func (a *app) refreshList() {
	lb := a.c[idList]
	topIdx := sendMsg(lb, LB_GETTOPINDEX, 0, 0)
	sendMsg(lb, LB_RESETCONTENT, 0, 0)
	for i := 0; i <= a.itemCount()+1; i++ {
		sendMsg(lb, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(a.listText(i)))))
	}
	sendMsg(lb, LB_SETTOPINDEX, topIdx, 0)
	sendMsg(lb, LB_SETCURSEL, uintptr(a.sel), 0)

	var names []string
	for _, it := range a.proj.Items {
		names = append(names, it.ID+" – "+title(it))
	}
	fillCombo(a.c[idDefault], names, a.proj.Default)

	isItem := a.sel >= 1 && a.sel <= a.itemCount()
	enable(a.c[idDel], isItem)
	enable(a.c[idBlocks], isItem)
	enable(a.c[idCopy], isItem && a.itemCount() < MaxItems)
	enable(a.c[idAdd], a.itemCount() < MaxItems)
	enable(a.c[idUp], isItem && a.sel > 1)
	enable(a.c[idDown], isItem && a.sel < a.itemCount())
}

func (a *app) loadEditor() {
	a.loading = true
	defer func() { a.loading = false }()
	it := a.current()
	isItem := a.sel >= 1 && a.sel <= a.itemCount()
	enable(a.c[idID], isItem)
	enable(a.c[idTitle], isItem)
	if isItem {
		setText(a.c[idID], it.ID)
		setText(a.c[idTitle], it.Title)
	} else {
		setText(a.c[idID], "")
		setText(a.c[idTitle], "")
	}
	setText(a.c[idConfig], JoinLines(it.Config))
	setText(a.c[idAutoexec], JoinLines(it.Autoexec))
	hint := ""
	if isItem && a.proj.Target == TargetFreeDOS {
		hint = T("FreeDOS: these lines get the \"%d?\" prefix, and %%CONFIG%% will be %d.", a.sel, a.sel)
	} else if isItem {
		hint = T("MS-DOS: this is the [%s] block, and %%CONFIG%% will be %s.", it.ID, it.ID)
	}
	setText(a.c[idHint], hint)
}

// commit copies the editor fields back into the project.
func (a *app) commit() {
	if a.loading || a.proj == nil {
		return
	}
	it := a.current()
	isItem := a.sel >= 1 && a.sel <= a.itemCount()
	changed := false
	if isItem {
		id := strings.ToUpper(strings.TrimSpace(getText(a.c[idID])))
		t := strings.TrimSpace(getText(a.c[idTitle]))
		if id != it.ID || t != it.Title {
			changed = true
		}
		it.ID, it.Title = id, t
	}
	cfg := SplitLines(getText(a.c[idConfig]))
	auto := SplitLines(getText(a.c[idAutoexec]))
	if strings.Join(cfg, "\n") != strings.Join(it.Config, "\n") || strings.Join(auto, "\n") != strings.Join(it.Autoexec, "\n") {
		changed = true
	}
	it.Config, it.Autoexec = cfg, auto
	if changed {
		a.setDirty()
	}
}

func (a *app) readSettings() {
	if a.loading {
		return
	}
	p := a.proj
	oldTarget := p.Target
	p.Target = Target(sendMsg(a.c[idTarget], CB_GETCURSEL, 0, 0))
	if p.Target != oldTarget {
		a.targetChanged(oldTarget)
	}
	p.Codepage = Codepage(sendMsg(a.c[idCodepage], CB_GETCURSEL, 0, 0))
	if d := int(sendMsg(a.c[idDefault], CB_GETCURSEL, 0, 0)); d >= 0 && d < len(p.Items) {
		p.Default = d
	}
	t := strings.TrimSpace(getText(a.c[idTimeout]))
	if t == "" {
		p.Timeout = -1
	} else if n, err := strconv.Atoi(t); err == nil && n >= 0 {
		p.Timeout = n
	}
	p.MenuColor = strings.TrimSpace(getText(a.c[idColor]))
	p.ConfigName = strings.TrimSpace(getText(a.c[idCfgName]))
	p.AutoexecName = strings.TrimSpace(getText(a.c[idAutoName]))
	a.setDirty()
}

// targetChanged rebuilds the items made of building blocks for the new DOS
// and adjusts the common lines of a project that still has the defaults.
func (a *app) targetChanged(from Target) {
	p := a.proj
	cs, ce := commonStart(from), commonEnd(from)
	if strings.Join(p.Start.Config, "\n") == strings.Join(cs.Config, "\n") && strings.Join(p.Start.Autoexec, "\n") == strings.Join(cs.Autoexec, "\n") &&
		strings.Join(p.End.Config, "\n") == strings.Join(ce.Config, "\n") {
		p.Start, p.End = commonStart(p.Target), commonEnd(p.Target)
		if p.Target == TargetFreeDOS && p.AutoexecName == "" {
			p.AutoexecName = "FDAUTO.BAT"
		} else if p.Target == TargetMSDOS && p.AutoexecName == "FDAUTO.BAT" {
			p.AutoexecName = ""
		}
		a.loading = true
		setText(a.c[idAutoName], p.AutoexecName)
		a.loading = false
	}
	kept := p.RebuildBlockItems(from)
	if len(kept) > 0 {
		msgBox(a.hwnd, T("These menu items were edited by hand, so their lines were not changed for %s: %s\n\nCheck the paths in them (for example C:\\DOS and C:\\FREEDOS\\BIN).", p.Target.String(), strings.Join(kept, ", ")), appTitle(), MB_ICONINFORMATION)
	}
}

// autoChanged is for changes the program makes by itself (the image window
// following the source): an unsaved project made from a template stays
// "unchanged", so choosing another menu does not ask to save it.
func (a *app) autoChanged() {
	if a.path != "" {
		a.setDirty()
		return
	}
	a.updateStatus()
}

func (a *app) setDirty() {
	if !a.dirty {
		a.dirty = true
		a.updateTitle()
	}
	a.updateStatus()
}

func (a *app) updateTitle() {
	name := T("untitled project")
	if a.path != "" {
		name = filepath.Base(a.path)
	}
	star := ""
	if a.dirty {
		star = "*"
	}
	setText(a.hwnd, fmt.Sprintf("%s%s – %s (%s)", star, name, appTitle(), a.proj.Target))
}

func (a *app) updateStatus() {
	ps := a.proj.Validate()
	if t := strings.TrimSpace(getText(a.c[idTimeout])); t != "" {
		if n, err := strconv.Atoi(t); err != nil || n < 0 {
			ps = append([]Problem{{true, T("The timeout must be a whole number (empty: forever).")}}, ps...)
		}
	}
	msg := T("OK.")
	for _, x := range ps {
		if x.Fatal {
			msg = T("Error: %s", x.Msg)
			break
		}
	}
	if !HasFatal(ps) && len(ps) > 0 {
		msg = T("Note: %s", ps[0].Msg)
	}
	if miss := MissingDrivers(a.proj, a.store); len(miss) > 0 && !HasFatal(ps) {
		msg += "  " + T("Missing drivers: %d (Drivers menu).", len(miss))
	}
	setText(a.c[idStatus], msg)
}

// ---- item operations ----

func (a *app) selectRow(i int) {
	a.sel = i
	a.refreshList()
	a.loadEditor()
	a.refreshNames()
}

func (a *app) addItem(copyCur bool) {
	a.commit()
	p := a.proj
	if len(p.Items) >= MaxItems {
		return
	}
	var it Item
	if copyCur && a.sel >= 1 && a.sel <= len(p.Items) {
		src := p.Items[a.sel-1]
		it = Item{ID: p.UniqueID(src.ID), Title: src.Title + " " + T("(copy)"),
			Config: append([]string(nil), src.Config...), Autoexec: append([]string(nil), src.Autoexec...)}
	} else {
		it = Item{ID: p.UniqueID(fmt.Sprintf("MENU%d", len(p.Items)+1)), Title: T("New menu item")}
	}
	pos := len(p.Items)
	if a.sel >= 1 && a.sel <= len(p.Items) {
		pos = a.sel // after the current one
	}
	p.Items = append(p.Items[:pos], append([]Item{it}, p.Items[pos:]...)...)
	if len(p.Items) > 1 && p.Default >= pos {
		p.Default++
	}
	a.setDirty()
	a.selectRow(pos + 1)
	pSetFocus.Call(a.c[idTitle])
	sendMsg(a.c[idTitle], 0x00B1, 0, ^uintptr(0)) // EM_SETSEL all
}

func (a *app) deleteItem() {
	p := a.proj
	i := a.sel - 1
	if i < 0 || i >= len(p.Items) {
		return
	}
	if msgBox(a.hwnd, T("Delete menu item %s?", p.Items[i].ID), appTitle(), MB_YESNO|MB_ICONQUESTION) != IDYES {
		return
	}
	p.Items = append(p.Items[:i], p.Items[i+1:]...)
	if p.Default == i {
		p.Default = 0
	} else if p.Default > i {
		p.Default--
	}
	a.sel = i
	if a.sel < 1 && len(p.Items) > 0 {
		a.sel = 1
	}
	a.setDirty()
	a.selectRow(a.sel)
}

func (a *app) moveItem(d int) {
	a.commit()
	p := a.proj
	i := a.sel - 1
	j := i + d
	if i < 0 || j < 0 || i >= len(p.Items) || j >= len(p.Items) {
		return
	}
	p.Items[i], p.Items[j] = p.Items[j], p.Items[i]
	switch p.Default {
	case i:
		p.Default = j
	case j:
		p.Default = i
	}
	a.setDirty()
	a.selectRow(j + 1)
}

// ---- files ----

func projFilter() []string {
	return []string{T("Boot menu project (*.bmp.json)"), "*.bmp.json;*.json", T("All files"), "*.*"}
}

func imgFilter() []string {
	return []string{T("Disk images (*.img, *.ima, *.vhd, *.hdd, *.dsk)"), "*.img;*.ima;*.vhd;*.hdd;*.dsk;*.vfd;*.flp", T("All files"), "*.*"}
}

func (a *app) askSave() bool { return a.askSaveFrom(a.hwnd) }

// askSaveFrom asks over the given window (so the main window does not come
// to the front when another window asks).
func (a *app) askSaveFrom(owner uintptr) bool {
	if !a.dirty {
		return true
	}
	switch msgBox(owner, T("The project has changed. Save it?"), appTitle(), MB_YESNOCANCEL|MB_ICONQUESTION) {
	case IDYES:
		return a.saveProject(false)
	case IDNO:
		return true
	}
	return false
}

func (a *app) saveProject(as bool) bool {
	a.commit()
	a.readSettingsQuiet()
	path := a.path
	if path == "" || as {
		path = fileDialog(a.hwnd, true, T("Save project"), "json", "menu.bmp.json", projFilter()...)
		if path == "" {
			return false
		}
	}
	if err := a.proj.Save(path); err != nil {
		msgBox(a.hwnd, T("Could not save:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return false
	}
	a.path = path
	a.dirty = false
	a.updateTitle()
	return true
}

// readSettingsQuiet reads the settings row without marking the project dirty.
func (a *app) readSettingsQuiet() {
	d := a.dirty
	a.readSettings()
	a.dirty = d
}

func (a *app) openProjectFile(path string) {
	p, err := LoadProject(path)
	if err != nil {
		msgBox(a.hwnd, T("Could not open:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	a.proj, a.path, a.dirty, a.sel = p, path, false, 1
	a.loadProject()
}

func (a *app) setProject(p *Project, notes []string, what string) {
	a.proj, a.path, a.dirty, a.sel = p, "", true, 1
	a.loadProject()
	text := T("Read from %s: %d menu items, %s.", what, len(p.Items), p.Target.String())
	if len(notes) > 0 {
		text += "\n\n" + T("Notes:") + "\n• " + strings.Join(notes, "\n• ")
	}
	text += "\n\n" + T("Check the imported text before writing it back.")
	msgBox(a.hwnd, text, appTitle(), MB_ICONINFORMATION)
}

func (a *app) readFromImage() {
	if !a.askSave() {
		return
	}
	path := fileDialog(a.hwnd, false, T("Disk image to read the menu from"), "", "", imgFilter()...)
	if path == "" {
		return
	}
	p, notes, err := ReadFromImage(path)
	if err != nil {
		msgBox(a.hwnd, T("Could not read it:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	a.setProject(p, notes, filepath.Base(path))
}

func (a *app) readFromDir() {
	if !a.askSave() {
		return
	}
	dir := folderDialog(a.hwnd, T("Choose the folder with CONFIG.SYS and AUTOEXEC.BAT (for example the root of a mounted disk image):"))
	if dir == "" {
		return
	}
	p, notes, err := ReadFromFolder(dir)
	if err != nil {
		msgBox(a.hwnd, T("Could not read it:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	a.setProject(p, notes, dir)
}

// ready validates before output; false if there are fatal problems.
func (a *app) ready() bool {
	a.commit()
	a.readSettingsQuiet()
	a.refreshList()
	ps := a.proj.Validate()
	if !HasFatal(ps) {
		return true
	}
	var bad []string
	for _, x := range ps {
		if x.Fatal {
			bad = append(bad, "• "+x.Msg)
		}
	}
	msgBox(a.hwnd, T("Please fix these first:")+"\n\n"+strings.Join(bad, "\n"), appTitle(), MB_ICONWARNING)
	return false
}

// checkDrivers asks what to do with missing driver files. Returns false to stop.
func (a *app) checkDrivers() bool {
	miss := MissingDrivers(a.proj, a.store)
	if len(miss) == 0 {
		return true
	}
	r := msgBox(a.hwnd, T("These files are not here yet:")+"\n\n• "+strings.Join(miss, "\n• ")+"\n\n"+
		T("Yes: continue without them\nNo: open the Drivers window (download or browse)\nCancel: go back"),
		appTitle(), MB_YESNOCANCEL|MB_ICONWARNING)
	switch r {
	case IDYES:
		return true
	case IDNO:
		a.openDrivers()
	}
	return false
}

func reportText(r *Report, head string) string {
	t := head
	if r != nil {
		if len(r.Done) > 0 {
			t += "\n\n" + T("Done:") + "\n• " + strings.Join(r.Done, "\n• ")
		}
		if len(r.Warnings) > 0 {
			t += "\n\n" + T("Warnings:") + "\n• " + strings.Join(r.Warnings, "\n• ")
		}
	}
	return t
}

func (a *app) writeImage() {
	if !a.ready() || !a.checkDrivers() {
		return
	}
	path := fileDialog(a.hwnd, false, T("Disk image to write to"), "", "", imgFilter()...)
	if path == "" {
		return
	}
	if msgBox(a.hwnd, T("Writing to:\n%s\n\nThe existing %s and %s are kept as .BAK.\nImportant: the image must not be attached in the emulator (86Box) while writing, and keep a copy of any image that matters.\n\nGo ahead?",
		path, a.proj.ConfigFile(), a.proj.AutoexecFile()), appTitle(), MB_YESNO|MB_ICONQUESTION) != IDYES {
		return
	}
	r, err := WriteToImage(path, a.proj, a.store, true)
	if err != nil {
		msgBox(a.hwnd, reportText(r, T("Error: %s", err.Error())), appTitle(), MB_ICONERROR)
		return
	}
	icon := uint32(MB_ICONINFORMATION)
	if len(r.Warnings) > 0 {
		icon = MB_ICONWARNING
	}
	msgBox(a.hwnd, reportText(r, T("The disk image is ready.")), appTitle(), icon)
}

func (a *app) saveToDir() {
	if !a.ready() {
		return
	}
	withDrv := false
	if len(a.proj.Drivers) > 0 {
		switch msgBox(a.hwnd, T("Copy the selected drivers into the folder too (DRIVERS, MTCP, EC subfolders)?"), appTitle(), MB_YESNOCANCEL|MB_ICONQUESTION) {
		case IDYES:
			withDrv = true
			if !a.checkDrivers() {
				return
			}
		case IDCANCEL:
			return
		}
	}
	dir := folderDialog(a.hwnd, T("Where should the files go? (For example the root of a disk image mounted with ImDisk/OSFMount.)"))
	if dir == "" {
		return
	}
	r, err := WriteToFolder(dir, a.proj, a.store, withDrv)
	if err != nil {
		msgBox(a.hwnd, reportText(r, T("Error: %s", err.Error())), appTitle(), MB_ICONERROR)
		return
	}
	msgBox(a.hwnd, reportText(r, T("Saved to: %s", dir)), appTitle(), MB_ICONINFORMATION)
}

func (a *app) about() {
	msgBox(a.hwnd, appTitle()+"\n\n"+T(`Makes a CONFIG.SYS / FDCONFIG.SYS and AUTOEXEC.BAT boot menu for MS-DOS 6.x and FreeDOS, and can write it straight into a disk image.

Tips:
• The two "Common part" rows in the list run for every menu item, before and after the item's own lines.
• A menu item ID is at most 8 letters/digits: it is the CONFIG.SYS block name and the AUTOEXEC.BAT label.
• The boot menu appears before any codepage is loaded, so ASCII mode (no accents) is the safe choice.
• Disk images: raw .img/.ima (floppy, or hard disk with MBR) and fixed-size .vhd; FAT12/16/32.
• Drivers are kept in the "drivers" folder; the list can be edited in the Drivers window.
• Translations: Language > Make a translation file.

Always try the menu in 86Box before using it on a real machine.`), appTitle(), MB_ICONINFORMATION)
}

func (a *app) exportLanguage() {
	dir := filepath.Join(a.data, "lang")
	os.MkdirAll(dir, 0755)
	name := "xx.lng"
	if c := CurrentLanguage(); c != "en" {
		name = c + ".lng"
	}
	path := fileDialog(a.hwnd, true, T("Save translation file"), "lng", filepath.Join(dir, name),
		T("Translation file (*.lng)"), "*.lng", T("All files"), "*.*")
	if path == "" {
		return
	}
	if err := WriteLanguageTemplate(path, CurrentLanguage()); err != nil {
		msgBox(a.hwnd, T("Could not save:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	msgBox(a.hwnd, T("Saved: %s\n\nFill in the TR: lines with a text editor (UTF-8), name the file after the language code (for example de.lng), keep it in the lang folder, then choose Language > Reload language files.", path), appTitle(), MB_ICONINFORMATION)
	shellOpen(dir)
}

func (a *app) onCommand(id int, code uint32) {
	switch {
	case id >= mTemplate && id < mTemplate+len(Templates):
		if a.askSave() {
			a.proj, a.path, a.dirty, a.sel = Templates[id-mTemplate].Make(), "", false, 1
			a.loadProject()
		}
		return
	case id >= mLanguage && id < mLanguage+len(a.langMenu):
		a.setLanguage(a.langMenu[id-mLanguage].Code)
		return
	}
	switch id {
	case idList:
		if code == LBN_SELCHANGE {
			a.commit()
			i := int(sendMsg(a.c[idList], LB_GETCURSEL, 0, 0))
			if i >= 0 {
				a.selectRow(i)
			}
		}
	case idID, idTitle:
		if code == EN_KILLFOCUS {
			a.commit()
			a.refreshList()
		} else if code == EN_CHANGE && !a.loading {
			a.commit()
		}
	case idConfig, idAutoexec:
		if code == EN_KILLFOCUS || (code == EN_CHANGE && !a.loading) {
			a.commit()
		}
	case idTarget, idCodepage, idDefault:
		if code == CBN_SELCHANGE {
			a.readSettings()
			a.refreshList()
			a.loadEditor()
			a.refreshNames()
			a.updateTitle()
		}
	case idTimeout, idColor, idCfgName, idAutoName:
		if code == EN_CHANGE && !a.loading {
			a.readSettings()
			if id != idTimeout && id != idColor {
				a.refreshNames()
			}
		}
	case idAdd:
		a.openBlocks(-1)
	case idBlocks:
		if a.sel >= 1 && a.sel <= a.itemCount() {
			a.openBlocks(a.sel - 1)
		}
	case idCopy:
		a.addItem(true)
	case idDel:
		a.deleteItem()
	case idUp:
		a.moveItem(-1)
	case idDown:
		a.moveItem(1)
	case idPreview, mPreview:
		a.showPreview()
	case idSaveDir, mSaveDir:
		a.saveToDir()
	case idWriteImg, mWriteImg:
		a.writeImage()
	case mOpen:
		if a.askSave() {
			if p := fileDialog(a.hwnd, false, T("Open project"), "", "", projFilter()...); p != "" {
				a.openProjectFile(p)
			}
		}
	case mSave:
		a.saveProject(false)
	case mSaveAs:
		a.saveProject(true)
	case mExit:
		postMsg(a.hwnd, WM_CLOSE, 0, 0)
	case mReadImg:
		a.readFromImage()
	case mReadDir:
		a.readFromDir()
	case mDrivers:
		a.openDrivers()
	case mDrvFolder:
		os.MkdirAll(a.store.Dir, 0755)
		shellOpen(a.store.Dir)
	case mNewImage:
		a.openBootWin()
	case mWizard:
		a.openWizard()
	case mGeomSave:
		a.saveGeom()
	case mGeomFloppy:
		a.geomToFloppy()
	case mGeomFDFloppy:
		a.makeGeomFloppy()
	case mBackupCard:
		a.backupCard()
	case mWriteCard:
		a.writeCard("")
	case mLangExport:
		a.exportLanguage()
	case mLangReload:
		errs := a.loadLanguageFiles()
		a.buildMenu()
		if len(errs) > 0 {
			var t []string
			for _, e := range errs {
				t = append(t, e.Error())
			}
			msgBox(a.hwnd, T("Some language files could not be read:")+"\n\n"+strings.Join(t, "\n"), appTitle(), MB_ICONWARNING)
		} else {
			a.setLanguage(CurrentLanguage())
		}
	case mLangFolder:
		d := filepath.Join(a.data, "lang")
		os.MkdirAll(d, 0755)
		shellOpen(d)
	case mAbout:
		a.about()
	}
}

func mainProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	switch msg {
	case WM_SIZE:
		if a != nil && a.hwnd != 0 && len(a.c) > 0 {
			a.layout()
		}
		return 0
	case WM_GETMINMAXINFO:
		mm := *(**minMaxInfo)(unsafe.Pointer(&lParam)) // lParam is a pointer from Windows
		if a != nil {
			mm.minTrack = point{int32(a.s(900)), int32(a.s(560))}
		}
		return 0
	case WM_COMMAND:
		if a != nil && a.hwnd != 0 {
			a.onCommand(int(loword(wParam)), hiword(wParam))
		}
		return 0
	case WM_CLOSE:
		if a.askSave() {
			pDestroyWindow.Call(hwnd)
		}
		return 0
	case WM_DESTROY:
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// ---- preview window ----

// Child IDs of the small windows start at 10: 1 and 2 are IDOK and IDCANCEL,
// which IsDialogMessage sends for Enter and Esc.
const (
	pvConfig = 10 + iota
	pvAuto
	pvLblC
	pvLblA
	pvNotes
	pvClose
)

func (a *app) showPreview() {
	if !a.ready() {
		return
	}
	out, err := Generate(a.proj)
	if err != nil {
		msgBox(a.hwnd, err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	if a.preview == 0 {
		a.prevC = map[int]uintptr{}
		a.preview = createWindow(0, "EasyDOSDeployPreview", T("Preview"), WS_OVERLAPPEDWINDOW, cwDefault, cwDefault, a.s(1000), a.s(640), 0, 0) // no owner: the main window can come to the front
		mk := func(id int, class string, style, ex uint32, font uintptr) {
			h := createWindow(ex, class, "", WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, a.preview, uintptr(id))
			sendMsg(h, WM_SETFONT, font, 1)
			a.prevC[id] = h
		}
		ro := uint32(WS_TABSTOP | ES_MULTILINE | ES_READONLY | ES_AUTOVSCROLL | ES_AUTOHSCROLL | WS_VSCROLL | WS_HSCROLL)
		mk(pvLblC, "STATIC", 0, 0, a.font)
		mk(pvLblA, "STATIC", 0, 0, a.font)
		mk(pvConfig, "EDIT", ro, WS_EX_CLIENTEDGE, a.mono)
		mk(pvAuto, "EDIT", ro, WS_EX_CLIENTEDGE, a.mono)
		mk(pvNotes, "STATIC", 0, 0, a.font)
		mk(pvClose, "BUTTON", WS_TABSTOP|BS_DEFPUSHBUTTON, 0, a.font)
		setText(a.prevC[pvClose], T("Close"))
		a.layoutPreview()
	}
	disp := func(ls []string) string {
		// show exactly what goes into the file, after codepage conversion
		return strings.Join(DecodeFile(EncodeFile(ls, a.proj.Codepage), a.proj.Codepage), "\r\n")
	}
	setText(a.prevC[pvLblC], out.ConfigName+":")
	setText(a.prevC[pvLblA], out.AutoexecName+":")
	setText(a.prevC[pvConfig], disp(out.Config))
	setText(a.prevC[pvAuto], disp(out.Autoexec))
	var warn []string
	for _, x := range a.proj.Validate() {
		warn = append(warn, x.Msg)
	}
	if len(warn) == 0 {
		warn = []string{T("No warnings. The files are written with DOS (CRLF) line endings.")}
	}
	setText(a.prevC[pvNotes], strings.Join(warn, "  "))
	setText(a.preview, T("Preview")+" – "+a.proj.Target.String())
	pShowWindow.Call(a.preview, SW_SHOW)
	pSetForegroundWindow.Call(a.preview)
}

func (a *app) layoutPreview() {
	w, h := clientSize(a.preview)
	s := a.s
	m := s(10)
	half := (w - 3*m) / 2
	lh := s(18)
	bh := s(28)
	eh := h - 2*m - lh - s(4) - bh - s(10) - s(4)
	move(a.prevC[pvLblC], m, m, half, lh)
	move(a.prevC[pvLblA], 2*m+half, m, half, lh)
	move(a.prevC[pvConfig], m, m+lh+s(4), half, eh)
	move(a.prevC[pvAuto], 2*m+half, m+lh+s(4), half, eh)
	cbw := fitW(s(110), s(24), a.prevC[pvClose])
	move(a.prevC[pvNotes], m, h-m-bh+s(6), w-3*m-cbw, bh)
	move(a.prevC[pvClose], w-m-cbw, h-m-bh, cbw, bh)
}

func previewProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	switch msg {
	case WM_SIZE:
		if a.preview != 0 && len(a.prevC) > 0 {
			a.layoutPreview()
		}
		return 0
	case WM_COMMAND:
		if loword(wParam) == pvClose || (loword(wParam) == IDCANCEL && hiword(wParam) == 0) {
			pShowWindow.Call(hwnd, 0)
		}
		return 0
	case WM_CLOSE:
		pShowWindow.Call(hwnd, 0) // keep it for next time
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}

// ---- drivers window ----

const (
	dvList = 10 + iota
	dvInfo
	dvDownload
	dvBrowse
	dvWeb
	dvFolder
	dvClose
	dvHead
	dvStatus
	dvNew
	dvEdit
	dvDelete
	dvReset
)

const wmDownloadDone = WM_APP + 1

type drvWin struct {
	hwnd   uintptr
	c      map[int]uintptr
	busy   bool
	mu     sync.Mutex
	result []DownloadResult
}

func (a *app) openDrivers() {
	if a.drv != nil {
		pShowWindow.Call(a.drv.hwnd, SW_SHOW)
		pSetForegroundWindow.Call(a.drv.hwnd)
		a.drv.refresh()
		return
	}
	d := &drvWin{c: map[int]uintptr{}}
	a.drv = d
	d.hwnd = createWindow(0, "EasyDOSDeployDrivers", T("Drivers"), WS_OVERLAPPEDWINDOW, cwDefault, cwDefault, a.s(900), a.s(560), 0, 0) // no owner, see Preview
	mk := func(id int, class, text string, style, ex uint32) {
		h := createWindow(ex, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, d.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		d.c[id] = h
	}
	mk(dvHead, "STATIC", T("Select what goes onto the disk (several can be selected). Missing files can be downloaded, or browse for them (a single file or a zip)."), 0, 0)
	mk(dvList, "LISTBOX", "", WS_TABSTOP|LBS_NOTIFY|LBS_MULTIPLESEL|LBS_NOINTEGRALHEIGHT|WS_VSCROLL, WS_EX_CLIENTEDGE)
	mk(dvInfo, "EDIT", "", ES_MULTILINE|ES_READONLY|ES_AUTOVSCROLL|WS_VSCROLL, WS_EX_CLIENTEDGE)
	mk(dvNew, "BUTTON", T("New..."), WS_TABSTOP, 0)
	mk(dvEdit, "BUTTON", T("Edit..."), WS_TABSTOP, 0)
	mk(dvDelete, "BUTTON", T("Remove"), WS_TABSTOP, 0)
	mk(dvReset, "BUTTON", T("Restore built-in list"), WS_TABSTOP, 0)
	mk(dvDownload, "BUTTON", T("Download missing"), WS_TABSTOP|BS_DEFPUSHBUTTON, 0)
	mk(dvBrowse, "BUTTON", T("Browse..."), WS_TABSTOP, 0)
	mk(dvWeb, "BUTTON", T("Web page"), WS_TABSTOP, 0)
	mk(dvFolder, "BUTTON", T("Open folder"), WS_TABSTOP, 0)
	mk(dvClose, "BUTTON", T("Close"), WS_TABSTOP, 0)
	mk(dvStatus, "STATIC", "", 0, 0)
	d.layout()
	d.refresh()
	pShowWindow.Call(d.hwnd, SW_SHOW)
}

func (d *drvWin) layout() {
	a := theApp
	w, h := clientSize(d.hwnd)
	s := a.s
	m := s(10)
	headH := s(36)
	bh := s(28)
	listW := (w - 3*m) * 52 / 100
	top := m + headH
	lh := h - top - 2*bh - 3*m - s(4)
	move(d.c[dvHead], m, m, w-2*m, headH)
	move(d.c[dvList], m, top, listW, lh)
	move(d.c[dvInfo], 2*m+listW, top, w-3*m-listW, lh)
	row := func(y int, ids []int, widths []int) int {
		x := m
		for i, id := range ids {
			bw := fitW(s(widths[i]), s(24), d.c[id])
			move(d.c[id], x, y, bw, bh)
			x += bw + s(6)
		}
		return x
	}
	y1 := top + lh + s(8)
	row(y1, []int{dvNew, dvEdit, dvDelete, dvReset}, []int{90, 90, 90, 170})
	y2 := h - m - bh
	x := row(y2, []int{dvDownload, dvBrowse, dvWeb, dvFolder}, []int{150, 100, 90, 120})
	cw := fitW(s(110), s(24), d.c[dvClose])
	move(d.c[dvStatus], x+s(6), y2+s(6), w-x-cw-s(20)-2*m, bh)
	move(d.c[dvClose], w-m-cw, y2, cw, bh)
}

func (d *drvWin) selected(i int) bool {
	return sendMsg(d.c[dvList], LB_GETSEL, uintptr(i), 0) > 0
}

func (d *drvWin) refresh() { d.refreshFocus(-1) }

// refreshFocus rebuilds the list; focus >= 0 moves the caret there.
func (d *drvWin) refreshFocus(focus int) {
	a := theApp
	lb := d.c[dvList]
	caret := int(sendMsg(lb, LB_GETCARETINDEX, 0, 0))
	if focus >= 0 {
		caret = focus
	}
	top := sendMsg(lb, LB_GETTOPINDEX, 0, 0)
	sendMsg(lb, LB_RESETCONTENT, 0, 0)
	want := map[string]bool{}
	for _, id := range a.proj.Drivers {
		want[id] = true
	}
	for i := range a.store.Catalog {
		dr := &a.store.Catalog[i]
		state := T("present")
		if !a.store.Ready(dr) {
			state = T("MISSING")
		}
		allGen := true
		for _, f := range dr.Files {
			if f.Content == "" {
				allGen = false
			}
		}
		if allGen {
			state = T("built-in")
		}
		sendMsg(lb, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(fmt.Sprintf("%s  —  %s", dr.Name, state)))))
		if want[dr.ID] {
			sendMsg(lb, LB_SETSEL, 1, uintptr(i))
		}
	}
	sendMsg(lb, LB_SETTOPINDEX, top, 0)
	if caret >= 0 && caret < len(a.store.Catalog) {
		sendMsg(lb, LB_SETCARETINDEX, uintptr(caret), 0)
	}
	d.showInfo()
	enable(d.c[dvDownload], !d.busy)
	enable(d.c[dvReset], a.store.Custom)
}

func (d *drvWin) focusedIndex() int {
	a := theApp
	i := int(sendMsg(d.c[dvList], LB_GETCARETINDEX, 0, 0))
	if i < 0 || i >= len(a.store.Catalog) {
		return -1
	}
	return i
}

func (d *drvWin) focused() *Driver {
	if i := d.focusedIndex(); i >= 0 {
		return &theApp.store.Catalog[i]
	}
	return nil
}

func (d *drvWin) showInfo() {
	a := theApp
	dr := d.focused()
	for _, id := range []int{dvEdit, dvDelete, dvBrowse, dvFolder} {
		enable(d.c[id], dr != nil)
	}
	if dr == nil {
		setText(d.c[dvInfo], "")
		enable(d.c[dvWeb], false)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s  (%s)\r\n\r\n%s\r\n\r\n%s\r\n", dr.Name, dr.ID, dr.Desc, T("Files on the disk:"))
	for _, f := range dr.Files {
		st := T("present")
		if f.Content != "" {
			st = T("made by the program")
			if f.KeepExisting {
				st += ", " + T("an existing file is not overwritten")
			}
		} else if _, err := os.Stat(a.store.FilePath(dr.ID, f.Name)); err != nil {
			st = T("missing")
		}
		fmt.Fprintf(&b, "  C:\\%s  (%s)\r\n", f.Dest, st)
	}
	if dr.Info != "" {
		fmt.Fprintf(&b, "\r\n%s %s\r\n", T("Web page:"), dr.Info)
	}
	for i, u := range dr.URLs {
		if i == 0 {
			fmt.Fprintf(&b, "%s\r\n", T("Download from:"))
		}
		fmt.Fprintf(&b, "  %s\r\n", u)
	}
	fmt.Fprintf(&b, "\r\n%s\r\n%s\r\n", T("Local folder:"), a.store.DriverDir(dr.ID))
	setText(d.c[dvInfo], b.String())
	enable(d.c[dvWeb], dr.Info != "")
}

func (d *drvWin) syncSelection() {
	a := theApp
	var ids []string
	for i, dr := range a.store.Catalog {
		if d.selected(i) {
			ids = append(ids, dr.ID)
		}
	}
	// keep IDs not in the catalog (e.g. from an older project)
	for _, id := range a.proj.Drivers {
		if a.store.Find(id) == nil {
			ids = append(ids, id)
		}
	}
	if strings.Join(ids, ",") != strings.Join(a.proj.Drivers, ",") {
		a.proj.Drivers = ids
		a.setDirty()
	}
}

func (d *drvWin) download() {
	a := theApp
	var todo []*Driver
	for i := range a.store.Catalog {
		dr := &a.store.Catalog[i]
		if d.selected(i) && !a.store.Ready(dr) && len(dr.URLs) > 0 {
			todo = append(todo, dr)
		}
	}
	if len(todo) == 0 {
		msgBox(d.hwnd, T("None of the selected entries can be downloaded. Add entries without a download address (such as EC.EXE) with the Browse button."), appTitle(), MB_ICONINFORMATION)
		return
	}
	d.busy = true
	enable(d.c[dvDownload], false)
	setText(d.c[dvStatus], T("Downloading %d driver(s)...", len(todo)))
	store := a.store
	go func() {
		var res []DownloadResult
		for _, dr := range todo {
			res = append(res, store.Download(dr))
		}
		d.mu.Lock()
		d.result = res
		d.mu.Unlock()
		postMsg(d.hwnd, wmDownloadDone, 0, 0)
	}()
}

func (d *drvWin) downloadDone() {
	a := theApp
	d.mu.Lock()
	res := d.result
	d.mu.Unlock()
	d.busy = false
	var ok, bad []string
	for _, r := range res {
		name := r.ID
		if dr := a.store.Find(r.ID); dr != nil {
			name = dr.Name
		}
		if r.OK {
			ok = append(ok, name+": "+r.Message)
		} else {
			bad = append(bad, name+": "+r.Message)
		}
	}
	d.refresh()
	a.updateStatus()
	setText(d.c[dvStatus], T("Done: %d succeeded, %d failed.", len(ok), len(bad)))
	t := ""
	if len(ok) > 0 {
		t += T("Succeeded:") + "\n• " + strings.Join(ok, "\n• ")
	}
	if len(bad) > 0 {
		if t != "" {
			t += "\n\n"
		}
		t += T("Failed:") + "\n• " + strings.Join(bad, "\n• ")
	}
	icon := uint32(MB_ICONINFORMATION)
	if len(bad) > 0 {
		icon = MB_ICONWARNING
	}
	msgBox(d.hwnd, t, appTitle(), icon)
}

func (d *drvWin) browse() {
	a := theApp
	dr := d.focused()
	if dr == nil {
		return
	}
	var names []string
	for _, f := range dr.Files {
		if f.Content == "" {
			names = append(names, f.Name)
		}
	}
	if len(names) == 0 {
		msgBox(d.hwnd, T("The program makes this file itself; there is nothing to browse for."), appTitle(), MB_ICONINFORMATION)
		return
	}
	p := fileDialog(d.hwnd, false, T("File for: %s (%s)", dr.Name, strings.Join(names, ", ")), "", "",
		T("Driver file, zip or disk image"), "*.com;*.exe;*.sys;*.zip;*.cfg;*.img;*.ima;*.vhd;*.hdd;*.dsk;*.vfd;*.flp", T("All files"), "*.*")
	if p == "" {
		return
	}
	got, err := a.store.AddFile(dr, p)
	if err != nil {
		msgBox(d.hwnd, T("Did not work:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	// a browsed driver is surely wanted on the disk
	found := false
	for _, id := range a.proj.Drivers {
		if id == dr.ID {
			found = true
		}
	}
	if !found {
		a.proj.Drivers = append(a.proj.Drivers, dr.ID)
		a.setDirty()
	}
	d.refresh()
	a.updateStatus()
	msg := T("Added: %s", strings.Join(got, ", "))
	if miss := a.store.Missing(dr); len(miss) > 0 {
		var mn []string
		for _, f := range miss {
			mn = append(mn, f.Name)
		}
		msg += "  " + T("Still missing: %s", strings.Join(mn, ", "))
	}
	setText(d.c[dvStatus], msg)
}

func (d *drvWin) remove() {
	a := theApp
	i := d.focusedIndex()
	if i < 0 {
		return
	}
	dr := a.store.Catalog[i]
	if msgBox(d.hwnd, T("Remove %s from the list?\n\nDownloaded files stay in the driver folder.", dr.Name), appTitle(), MB_YESNO|MB_ICONQUESTION) != IDYES {
		return
	}
	a.store.Catalog = append(a.store.Catalog[:i], a.store.Catalog[i+1:]...)
	if err := a.store.SaveCatalog(); err != nil {
		msgBox(d.hwnd, T("Could not save:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
	}
	d.refreshFocus(i - 1)
	a.updateStatus()
}

func (d *drvWin) reset() {
	a := theApp
	if msgBox(d.hwnd, T("Restore the built-in driver list? Your own entries and changes are lost; downloaded files stay."), appTitle(), MB_YESNO|MB_ICONQUESTION) != IDYES {
		return
	}
	if err := a.store.ResetCatalog(); err != nil {
		msgBox(d.hwnd, err.Error(), appTitle(), MB_ICONERROR)
	}
	d.refresh()
	a.updateStatus()
}

func driversProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	d := a.drv
	switch msg {
	case WM_SIZE:
		if d != nil && len(d.c) > 0 {
			d.layout()
		}
		return 0
	case wmDownloadDone:
		if d != nil {
			d.downloadDone()
		}
		return 0
	case WM_COMMAND:
		if d == nil {
			return 0
		}
		switch int(loword(wParam)) {
		case dvList:
			if hiword(wParam) == LBN_SELCHANGE {
				d.syncSelection()
				d.showInfo()
			}
		case dvDownload:
			d.download()
		case dvBrowse:
			d.browse()
		case dvWeb:
			if dr := d.focused(); dr != nil && dr.Info != "" {
				shellOpen(dr.Info)
			}
		case dvFolder:
			if dr := d.focused(); dr != nil {
				os.MkdirAll(a.store.DriverDir(dr.ID), 0755)
				shellOpen(a.store.DriverDir(dr.ID))
			}
		case dvNew:
			a.openEditor(-1)
		case dvEdit:
			if i := d.focusedIndex(); i >= 0 {
				a.openEditor(i)
			}
		case dvDelete:
			d.remove()
		case dvReset:
			d.reset()
		case dvClose:
			pShowWindow.Call(hwnd, 0)
		case IDCANCEL:
			if hiword(wParam) == 0 {
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

// ---- driver entry editor ----

const (
	edID = 10 + iota
	edName
	edDesc
	edURLs
	edInfo
	edFiles
	edOK
	edCancel
	edLblID
	edLblName
	edLblDesc
	edLblURLs
	edLblInfo
	edLblFiles
	edNote
)

type editWin struct {
	hwnd  uintptr
	c     map[int]uintptr
	index int // -1 = new
	orig  Driver
}

func (a *app) openEditor(index int) {
	if a.edit != nil {
		pSetForegroundWindow.Call(a.edit.hwnd)
		return
	}
	e := &editWin{c: map[int]uintptr{}, index: index}
	if index >= 0 {
		e.orig = a.store.Catalog[index]
	}
	a.edit = e
	titleText := T("New driver")
	if index >= 0 {
		titleText = T("Edit driver")
	}
	e.hwnd = createWindow(WS_EX_DLGMODALFRAME, "EasyDOSDeployDrvEdit", titleText, WS_OVERLAPPEDWINDOW, cwDefault, cwDefault, a.s(640), a.s(600), a.drv.hwnd, 0)
	mk := func(id int, class, text string, style, ex uint32) {
		h := createWindow(ex, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, e.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		e.c[id] = h
	}
	line := uint32(WS_TABSTOP | ES_AUTOHSCROLL)
	multi := uint32(WS_TABSTOP | ES_MULTILINE | ES_AUTOVSCROLL | ES_AUTOHSCROLL | ES_WANTRETURN | WS_VSCROLL)
	mk(edLblID, "STATIC", T("ID (lower case, used for the folder name):"), 0, 0)
	mk(edID, "EDIT", e.orig.ID, line, WS_EX_CLIENTEDGE)
	mk(edLblName, "STATIC", T("Name:"), 0, 0)
	mk(edName, "EDIT", e.orig.Name, line, WS_EX_CLIENTEDGE)
	mk(edLblDesc, "STATIC", T("Description:"), 0, 0)
	mk(edDesc, "EDIT", e.orig.Desc, multi, WS_EX_CLIENTEDGE)
	mk(edLblFiles, "STATIC", T("Files, one per line:  NAME.EXT = FOLDER\\NAME.EXT  (place on the DOS disk; without \"=\" it goes to DRIVERS):"), 0, 0)
	mk(edFiles, "EDIT", FormatFiles(e.orig.Files), multi, WS_EX_CLIENTEDGE)
	sendMsg(e.c[edFiles], WM_SETFONT, a.mono, 1)
	mk(edLblURLs, "STATIC", T("Download addresses, one per line (zip or the file itself; may be empty):"), 0, 0)
	mk(edURLs, "EDIT", strings.Join(e.orig.URLs, "\r\n"), multi, WS_EX_CLIENTEDGE)
	mk(edLblInfo, "STATIC", T("Web page (shown when the download fails):"), 0, 0)
	mk(edInfo, "EDIT", e.orig.Info, line, WS_EX_CLIENTEDGE)
	var gen []string
	for _, f := range e.orig.Files {
		if f.Content != "" {
			gen = append(gen, f.Dest)
		}
	}
	note := ""
	if len(gen) > 0 {
		note = T("This entry also makes: %s (generated by the program, not editable here).", strings.Join(gen, ", "))
	}
	mk(edNote, "STATIC", note, 0, 0)
	mk(edOK, "BUTTON", T("OK"), WS_TABSTOP|BS_DEFPUSHBUTTON, 0)
	mk(edCancel, "BUTTON", T("Cancel"), WS_TABSTOP, 0)
	e.layout()
	enable(a.drv.hwnd, false)
	pShowWindow.Call(e.hwnd, SW_SHOW)
	pSetFocus.Call(e.c[edName])
	if index < 0 {
		pSetFocus.Call(e.c[edID])
	}
}

func (e *editWin) layout() {
	a := theApp
	w, h := clientSize(e.hwnd)
	s := a.s
	m := s(10)
	lh := s(18)
	rh := s(23)
	bh := s(28)
	cw := w - 2*m
	y := m
	lab := func(id int) {
		move(e.c[id], m, y, cw, lh)
		y += lh + s(2)
	}
	field := func(id, hh int) {
		move(e.c[id], m, y, cw, hh)
		y += hh + s(8)
	}
	free := h - 2*m - bh - s(10) - 6*(lh+s(2)) - 3*(rh+s(8)) - 3*s(8) - lh - s(6)
	lab(edLblID)
	field(edID, rh)
	lab(edLblName)
	field(edName, rh)
	lab(edLblDesc)
	field(edDesc, free*25/100)
	lab(edLblFiles)
	field(edFiles, free*40/100)
	lab(edLblURLs)
	field(edURLs, free*35/100)
	lab(edLblInfo)
	field(edInfo, rh)
	move(e.c[edNote], m, y, cw, lh)
	bw := fitW(s(100), s(24), e.c[edOK], e.c[edCancel])
	move(e.c[edOK], w-m-2*bw-s(8), h-m-bh, bw, bh)
	move(e.c[edCancel], w-m-bw, h-m-bh, bw, bh)
}

func (e *editWin) close() {
	a := theApp
	if a.drv != nil {
		enable(a.drv.hwnd, true)
		pSetForegroundWindow.Call(a.drv.hwnd)
	}
	pDestroyWindow.Call(e.hwnd)
	a.edit = nil
}

func (e *editWin) save() {
	a := theApp
	d := Driver{
		ID:   strings.ToLower(strings.TrimSpace(getText(e.c[edID]))),
		Name: strings.TrimSpace(getText(e.c[edName])),
		Desc: strings.Join(SplitLines(getText(e.c[edDesc])), " "),
		Info: strings.TrimSpace(getText(e.c[edInfo])),
	}
	for _, u := range SplitLines(getText(e.c[edURLs])) {
		if u = strings.TrimSpace(u); u != "" {
			d.URLs = append(d.URLs, u)
		}
	}
	d.Files, _ = ParseFiles(getText(e.c[edFiles]), e.orig.Files)
	if err := a.store.CheckDriver(&d, e.index); err != nil {
		msgBox(e.hwnd, err.Error(), appTitle(), MB_ICONWARNING)
		return
	}
	pos := e.index
	if e.index >= 0 {
		old := a.store.Catalog[e.index]
		if old.ID != d.ID {
			// keep downloaded files and the project's selection
			os.Rename(a.store.DriverDir(old.ID), a.store.DriverDir(d.ID))
			for i, id := range a.proj.Drivers {
				if id == old.ID {
					a.proj.Drivers[i] = d.ID
					a.setDirty()
				}
			}
		}
		a.store.Catalog[e.index] = d
	} else {
		a.store.Catalog = append(a.store.Catalog, d)
		pos = len(a.store.Catalog) - 1
	}
	if err := a.store.SaveCatalog(); err != nil {
		msgBox(e.hwnd, T("Could not save:")+"\n"+err.Error(), appTitle(), MB_ICONERROR)
		return
	}
	e.close()
	if a.drv != nil {
		a.drv.refreshFocus(pos)
	}
	a.updateStatus()
}

func editProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	e := a.edit
	switch msg {
	case WM_SIZE:
		if e != nil && len(e.c) > 0 {
			e.layout()
		}
		return 0
	case WM_GETMINMAXINFO:
		mm := *(**minMaxInfo)(unsafe.Pointer(&lParam))
		mm.minTrack = point{int32(a.s(520)), int32(a.s(520))}
		return 0
	case WM_COMMAND:
		if e == nil {
			return 0
		}
		switch int(loword(wParam)) {
		case edOK:
			e.save()
		case 1: // Enter in a single-line field
			if hiword(wParam) == 0 {
				e.save()
			}
		case edCancel:
			e.close()
		case IDCANCEL:
			if hiword(wParam) == 0 {
				e.close()
			}
		}
		return 0
	case WM_CLOSE:
		if e != nil {
			e.close()
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}
