//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	comdlg32 = syscall.NewLazyDLL("comdlg32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	pRegisterClassExW     = user32.NewProc("RegisterClassExW")
	pCreateWindowExW      = user32.NewProc("CreateWindowExW")
	pDefWindowProcW       = user32.NewProc("DefWindowProcW")
	pGetMessageW          = user32.NewProc("GetMessageW")
	pTranslateMessage     = user32.NewProc("TranslateMessage")
	pDispatchMessageW     = user32.NewProc("DispatchMessageW")
	pIsDialogMessageW     = user32.NewProc("IsDialogMessageW")
	pPostQuitMessage      = user32.NewProc("PostQuitMessage")
	pSendMessageW         = user32.NewProc("SendMessageW")
	pPostMessageW         = user32.NewProc("PostMessageW")
	pMoveWindow           = user32.NewProc("MoveWindow")
	pShowWindow           = user32.NewProc("ShowWindow")
	pUpdateWindow         = user32.NewProc("UpdateWindow")
	pSetWindowTextW       = user32.NewProc("SetWindowTextW")
	pGetWindowTextW       = user32.NewProc("GetWindowTextW")
	pGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	pEnableWindow         = user32.NewProc("EnableWindow")
	pMessageBoxW          = user32.NewProc("MessageBoxW")
	pLoadCursorW          = user32.NewProc("LoadCursorW")
	pLoadIconW            = user32.NewProc("LoadIconW")
	pCreateMenu           = user32.NewProc("CreateMenu")
	pCreatePopupMenu      = user32.NewProc("CreatePopupMenu")
	pAppendMenuW          = user32.NewProc("AppendMenuW")
	pSetMenu              = user32.NewProc("SetMenu")
	pDestroyWindow        = user32.NewProc("DestroyWindow")
	pGetClientRect        = user32.NewProc("GetClientRect")
	pSetProcessDPIAware   = user32.NewProc("SetProcessDPIAware")
	pGetDC                = user32.NewProc("GetDC")
	pReleaseDC            = user32.NewProc("ReleaseDC")
	pSetFocus             = user32.NewProc("SetFocus")
	pGetKeyState          = user32.NewProc("GetKeyState")
	pSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	pIsWindow             = user32.NewProc("IsWindow")
	pDestroyMenu          = user32.NewProc("DestroyMenu")
	pGetMenu              = user32.NewProc("GetMenu")
	pDrawMenuBar          = user32.NewProc("DrawMenuBar")

	pCreateFontW    = gdi32.NewProc("CreateFontW")
	pGetDeviceCaps  = gdi32.NewProc("GetDeviceCaps")
	pGetStockObject = gdi32.NewProc("GetStockObject")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	pGetOpenFileNameW = comdlg32.NewProc("GetOpenFileNameW")
	pGetSaveFileNameW = comdlg32.NewProc("GetSaveFileNameW")

	pSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	pSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	pShellExecuteW        = shell32.NewProc("ShellExecuteW")

	pCoInitializeEx = ole32.NewProc("CoInitializeEx")
	pCoTaskMemFree  = ole32.NewProc("CoTaskMemFree")
)

const (
	WM_CREATE         = 0x0001
	WM_DESTROY        = 0x0002
	WM_SIZE           = 0x0005
	WM_CLOSE          = 0x0010
	WM_GETMINMAXINFO  = 0x0024
	WM_SETFONT        = 0x0030
	WM_KEYDOWN        = 0x0100
	WM_COMMAND        = 0x0111
	WM_CTLCOLORSTATIC = 0x0138
	WM_APP            = 0x8000

	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_CHILD            = 0x40000000
	WS_VISIBLE          = 0x10000000
	WS_TABSTOP          = 0x00010000
	WS_VSCROLL          = 0x00200000
	WS_HSCROLL          = 0x00100000
	WS_EX_CLIENTEDGE    = 0x00000200
	WS_EX_DLGMODALFRAME = 0x00000001

	ES_MULTILINE   = 0x0004
	ES_UPPERCASE   = 0x0008
	ES_AUTOVSCROLL = 0x0040
	ES_AUTOHSCROLL = 0x0080
	ES_READONLY    = 0x0800
	ES_WANTRETURN  = 0x1000

	LBS_NOTIFY           = 0x0001
	LBS_MULTIPLESEL      = 0x0008
	LBS_NOINTEGRALHEIGHT = 0x0100

	CBS_DROPDOWNLIST = 0x0003

	BS_PUSHBUTTON    = 0x0000
	BS_DEFPUSHBUTTON = 0x0001
	BS_AUTOCHECKBOX  = 0x0003
	BM_GETCHECK      = 0x00F0
	BM_SETCHECK      = 0x00F1
	CBS_DROPDOWN     = 0x0002
	CB_GETCOUNT      = 0x0146
	CBN_EDITCHANGE   = 5

	LB_ADDSTRING     = 0x0180
	LB_RESETCONTENT  = 0x0184
	LB_SETSEL        = 0x0185
	LB_SETCURSEL     = 0x0186
	LB_GETSEL        = 0x0187
	LB_GETCURSEL     = 0x0188
	LB_GETCOUNT      = 0x018B
	LB_SETTOPINDEX   = 0x0197
	LB_GETTOPINDEX   = 0x018E
	LB_SETCARETINDEX = 0x019E
	LB_GETCARETINDEX = 0x019F
	LBN_SELCHANGE    = 1

	CB_ADDSTRING    = 0x0143
	CB_GETCURSEL    = 0x0147
	CB_RESETCONTENT = 0x014B
	CB_SETCURSEL    = 0x014E
	CBN_SELCHANGE   = 1

	EN_CHANGE    = 0x0300
	EN_KILLFOCUS = 0x0200
	EM_SETLIMIT  = 0x00C5
	EM_SETCUE    = 0x1501
	BN_CLICKED   = 0

	MF_STRING    = 0x0000
	MF_POPUP     = 0x0010
	MF_SEPARATOR = 0x0800
	MF_CHECKED   = 0x0008

	MB_OK              = 0x00000000
	MB_YESNO           = 0x00000004
	MB_YESNOCANCEL     = 0x00000003
	MB_ICONERROR       = 0x00000010
	MB_ICONQUESTION    = 0x00000020
	MB_ICONWARNING     = 0x00000030
	MB_ICONINFORMATION = 0x00000040
	IDYES              = 6
	IDNO               = 7
	IDCANCEL           = 2

	SW_SHOW         = 5
	COLOR_BTNFACE   = 15
	IDC_ARROW       = 32512
	IDI_APPLICATION = 32512
	LOGPIXELSY      = 90
	VK_CONTROL      = 0x11

	OFN_OVERWRITEPROMPT = 0x00000002
	OFN_HIDEREADONLY    = 0x00000004
	OFN_NOCHANGEDIR     = 0x00000008
	OFN_PATHMUSTEXIST   = 0x00000800
	OFN_FILEMUSTEXIST   = 0x00001000
	OFN_EXPLORER        = 0x00080000

	BIF_RETURNONLYFSDIRS = 0x0001
	BIF_NEWDIALOGSTYLE   = 0x0040
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type point struct{ x, y int32 }

type msgT struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
	private uint32
}

type rect struct{ left, top, right, bottom int32 }

type minMaxInfo struct {
	reserved, maxSize, maxPos, minTrack, maxTrack point
}

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	Flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        uintptr
	dwReserved        uint32
	FlagsEx           uint32
}

type browseInfo struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      *uint16
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

type actCtx struct {
	cbSize                 uint32
	dwFlags                uint32
	lpSource               *uint16
	wProcessorArchitecture uint16
	wLangId                uint16
	lpAssemblyDirectory    *uint16
	lpResourceName         *uint16
	lpApplicationName      *uint16
	hModule                uintptr
}

func u16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("?")
	}
	return p
}

func loword(x uintptr) uint32 { return uint32(x) & 0xFFFF }
func hiword(x uintptr) uint32 { return (uint32(x) >> 16) & 0xFFFF }

func sendMsg(h uintptr, m uint32, w, l uintptr) uintptr {
	r, _, _ := pSendMessageW.Call(h, uintptr(m), w, l)
	return r
}

func postMsg(h uintptr, m uint32, w, l uintptr) {
	pPostMessageW.Call(h, uintptr(m), w, l)
}

func setText(h uintptr, s string) { pSetWindowTextW.Call(h, uintptr(unsafe.Pointer(u16(s)))) }

func getText(h uintptr) string {
	n, _, _ := pGetWindowTextLengthW.Call(h)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	return syscall.UTF16ToString(buf)
}

func enable(h uintptr, on bool) {
	v := uintptr(0)
	if on {
		v = 1
	}
	pEnableWindow.Call(h, v)
}

func move(h uintptr, x, y, w, hh int) {
	if w < 0 {
		w = 0
	}
	if hh < 0 {
		hh = 0
	}
	pMoveWindow.Call(h, uintptr(x), uintptr(y), uintptr(w), uintptr(hh), 1)
}

var (
	pSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	pUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	pCallNextHookEx      = user32.NewProc("CallNextHookEx")
	pSetDlgItemTextW     = user32.NewProc("SetDlgItemTextW")
	pGetDlgItem          = user32.NewProc("GetDlgItem")
	pGetCurrentThreadId  = kernel32.NewProc("GetCurrentThreadId")

	msgHook   uintptr
	msgFlags  uint32
	msgHookCB = syscall.NewCallback(msgHookProc)
)

// msgHookProc renames the buttons of our message boxes into the program's
// language when the box is about to appear (Windows would use its own).
func msgHookProc(code int32, wParam, lParam uintptr) uintptr {
	const hcbtActivate = 5
	if code == hcbtActivate && msgHook != 0 {
		h := wParam
		set := func(id int, text string) {
			if c, _, _ := pGetDlgItem.Call(h, uintptr(id)); c != 0 {
				pSetDlgItemTextW.Call(h, uintptr(id), uintptr(unsafe.Pointer(u16(text))))
			}
		}
		set(IDYES, T("&Yes"))
		set(IDNO, T("&No"))
		if msgFlags&0x0F == MB_OK {
			set(1, T("OK"))
			set(IDCANCEL, T("OK"))
		} else {
			set(1, T("OK"))
			set(IDCANCEL, T("Cancel"))
		}
		hk := msgHook
		msgHook = 0
		pUnhookWindowsHookEx.Call(hk)
		return 0
	}
	r, _, _ := pCallNextHookEx.Call(0, uintptr(code), wParam, lParam)
	return r
}

func msgBox(owner uintptr, text, caption string, flags uint32) int {
	tid, _, _ := pGetCurrentThreadId.Call()
	msgFlags = flags
	msgHook, _, _ = pSetWindowsHookExW.Call(5, msgHookCB, 0, tid) // WH_CBT
	r, _, _ := pMessageBoxW.Call(owner, uintptr(unsafe.Pointer(u16(text))), uintptr(unsafe.Pointer(u16(caption))), uintptr(flags))
	if msgHook != 0 {
		pUnhookWindowsHookEx.Call(msgHook)
		msgHook = 0
	}
	return int(r)
}

func clientSize(h uintptr) (int, int) {
	var r rect
	pGetClientRect.Call(h, uintptr(unsafe.Pointer(&r)))
	return int(r.right - r.left), int(r.bottom - r.top)
}

func ctrlDown() bool {
	r, _, _ := pGetKeyState.Call(VK_CONTROL)
	return int16(r) < 0
}

func hInstance() uintptr {
	h, _, _ := pGetModuleHandleW.Call(0)
	return h
}

func registerClass(name string, proc uintptr) {
	cur, _, _ := pLoadCursorW.Call(0, IDC_ARROW)
	// icon group 1 from rsrc_windows_amd64.syso; the Windows default if missing
	icon, _, _ := pLoadIconW.Call(hInstance(), 1)
	if icon == 0 {
		icon, _, _ = pLoadIconW.Call(0, IDI_APPLICATION)
	}
	wc := wndClassEx{
		lpfnWndProc:   proc,
		hInstance:     hInstance(),
		hIcon:         icon,
		hIconSm:       icon,
		hCursor:       cur,
		hbrBackground: COLOR_BTNFACE + 1,
		lpszClassName: u16(name),
	}
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
}

func createWindow(exStyle uint32, class, title string, style uint32, x, y, w, h int, parent, id uintptr) uintptr {
	r, _, _ := pCreateWindowExW.Call(uintptr(exStyle), uintptr(unsafe.Pointer(u16(class))), uintptr(unsafe.Pointer(u16(title))),
		uintptr(style), uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, id, hInstance(), 0)
	return r
}

func makeFont(face string, pt, dpi int) uintptr {
	h := -(pt*dpi + 36) / 72
	f, _, _ := pCreateFontW.Call(uintptr(int32(h)), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(u16(face))))
	if f == 0 {
		f, _, _ = pGetStockObject.Call(17) // DEFAULT_GUI_FONT
	}
	return f
}

func screenDPI() int {
	dc, _, _ := pGetDC.Call(0)
	d, _, _ := pGetDeviceCaps.Call(dc, LOGPIXELSY)
	pReleaseDC.Call(0, dc)
	if d == 0 {
		return 96
	}
	return int(d)
}

// filter: pairs of "description", "*.ext;*.ext"
func makeFilter(pairs ...string) *uint16 {
	var b []uint16
	for _, s := range pairs {
		b = append(b, syscall.StringToUTF16(s)...) // includes the 0
	}
	b = append(b, 0)
	return &b[0]
}

func fileDialog(owner uintptr, save bool, title, defExt, initial string, filter ...string) string {
	buf := make([]uint16, 4096)
	copy(buf, syscall.StringToUTF16(initial))
	ofn := openFileName{
		hwndOwner:   owner,
		lpstrFilter: makeFilter(filter...),
		lpstrFile:   &buf[0],
		nMaxFile:    uint32(len(buf)),
		lpstrTitle:  u16(title),
		Flags:       OFN_EXPLORER | OFN_HIDEREADONLY | OFN_NOCHANGEDIR | OFN_PATHMUSTEXIST,
	}
	if defExt != "" {
		ofn.lpstrDefExt = u16(defExt)
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	var r uintptr
	if save {
		ofn.Flags |= OFN_OVERWRITEPROMPT
		r, _, _ = pGetSaveFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	} else {
		ofn.Flags |= OFN_FILEMUSTEXIST
		r, _, _ = pGetOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn)))
	}
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func folderDialog(owner uintptr, title string) string {
	name := make([]uint16, 260)
	bi := browseInfo{hwndOwner: owner, pszDisplayName: &name[0], lpszTitle: u16(title), ulFlags: BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE}
	pidl, _, _ := pSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return ""
	}
	defer pCoTaskMemFree.Call(pidl)
	path := make([]uint16, 1024)
	if r, _, _ := pSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&path[0]))); r == 0 {
		return ""
	}
	return syscall.UTF16ToString(path)
}

// runAsAdmin starts the program again with administrator rights.
func runAsAdmin(args string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	r, _, _ := pShellExecuteW.Call(0, uintptr(unsafe.Pointer(u16("runas"))), uintptr(unsafe.Pointer(u16(exe))),
		uintptr(unsafe.Pointer(u16(args))), 0, SW_SHOW)
	return r > 32
}

func shellOpen(target string) {
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(u16("open"))), uintptr(unsafe.Pointer(u16(target))), 0, 0, SW_SHOW)
}
