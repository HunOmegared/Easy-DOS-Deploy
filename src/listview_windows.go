//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// A report-style list view (SysListView32) with check boxes and icons in
// the columns, for the Drivers window.

var (
	comctl32              = syscall.NewLazyDLL("comctl32.dll")
	pInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")
	pImageListCreate      = comctl32.NewProc("ImageList_Create")
	pImageListAdd         = comctl32.NewProc("ImageList_Add")
	pCreateDIBSection     = gdi32.NewProc("CreateDIBSection")
	pDeleteObject         = gdi32.NewProc("DeleteObject")
	pGetSystemMetrics     = user32.NewProc("GetSystemMetrics")
	commonControlsLoaded  bool
)

const (
	WM_NOTIFY = 0x004E

	LVM_SETIMAGELIST             = 0x1003
	LVM_DELETEALLITEMS           = 0x1009
	LVM_GETNEXTITEM              = 0x100C
	LVM_ENSUREVISIBLE            = 0x1013
	LVM_SETCOLUMNWIDTH           = 0x101E
	LVM_GETTOPINDEX              = 0x1027
	LVM_GETCOUNTPERPAGE          = 0x1028
	LVM_SETITEMSTATE             = 0x102B
	LVM_GETITEMSTATE             = 0x102C
	LVM_SETEXTENDEDLISTVIEWSTYLE = 0x1036
	LVM_SUBITEMHITTEST           = 0x1039
	LVM_SETITEMW                 = 0x104C
	LVM_INSERTITEMW              = 0x104D
	LVM_INSERTCOLUMNW            = 0x1061

	LVS_REPORT           = 0x0001
	LVS_SINGLESEL        = 0x0004
	LVS_SHOWSELALWAYS    = 0x0008
	LVS_NOSORTHEADER     = 0x8000
	LVS_EX_SUBITEMIMAGES = 0x0002
	LVS_EX_CHECKBOXES    = 0x0004
	LVS_EX_FULLROWSELECT = 0x0020
	LVS_EX_DOUBLEBUFFER  = 0x00010000

	LVIF_TEXT  = 0x1
	LVIF_IMAGE = 0x2
	LVIF_STATE = 0x8

	LVIS_FOCUSED        = 0x1
	LVIS_SELECTED       = 0x2
	LVIS_STATEIMAGEMASK = 0xF000

	LVNI_FOCUSED  = 0x1
	LVNI_SELECTED = 0x2

	LVCF_FMT      = 0x1
	LVCF_WIDTH    = 0x2
	LVCF_TEXT     = 0x4
	LVCF_SUBITEM  = 0x8
	LVCFMT_CENTER = 0x2

	LVSIL_SMALL = 1

	NM_CLICK        = ^uint32(1)   // -2
	NM_DBLCLK       = ^uint32(2)   // -3
	NM_RETURN       = ^uint32(3)   // -4
	LVN_ITEMCHANGED = ^uint32(100) // -101

	ILC_COLOR32  = 0x20
	SM_CXVSCROLL = 2
)

type initCommonControlsEx struct{ size, icc uint32 }

type lvColumn struct {
	mask      uint32
	fmt       int32
	cx        int32
	text      *uint16
	textMax   int32
	subItem   int32
	image     int32
	order     int32
	cxMin     int32
	cxDefault int32
	cxIdeal   int32
}

type lvItem struct {
	mask      uint32
	item      int32
	subItem   int32
	state     uint32
	stateMask uint32
	text      *uint16
	textMax   int32
	image     int32
	lParam    uintptr
	indent    int32
	groupID   int32
	columns   uint32
	puColumns uintptr
	piColFmt  uintptr
	group     int32
}

type nmhdr struct {
	hwndFrom uintptr
	idFrom   uintptr
	code     uint32
}

// nmListView is NMLISTVIEW and the start of NMITEMACTIVATE.
type nmListView struct {
	hdr      nmhdr
	item     int32
	subItem  int32
	newState uint32
	oldState uint32
	changed  uint32
	pt       point
	lParam   uintptr
}

type lvHitTest struct {
	pt      point
	flags   uint32
	item    int32
	subItem int32
	group   int32
}

func loadCommonControls() {
	if commonControlsLoaded {
		return
	}
	icc := initCommonControlsEx{8, 0x1} // ICC_LISTVIEW_CLASSES
	pInitCommonControlsEx.Call(uintptr(unsafe.Pointer(&icc)))
	commonControlsLoaded = true
}

// makeIconList builds an image list of the program's status icons.
func makeIconList(n int) uintptr {
	il, _, _ := pImageListCreate.Call(uintptr(n), uintptr(n), ILC_COLOR32, iconCount, 0)
	if il == 0 {
		return 0
	}
	for k := 0; k < iconCount; k++ {
		px := drawIcon(k, n)
		// BITMAPINFOHEADER, top-down 32 bpp
		var bih [40]byte
		*(*uint32)(unsafe.Pointer(&bih[0])) = 40
		*(*int32)(unsafe.Pointer(&bih[4])) = int32(n)
		*(*int32)(unsafe.Pointer(&bih[8])) = -int32(n)
		*(*uint16)(unsafe.Pointer(&bih[12])) = 1
		*(*uint16)(unsafe.Pointer(&bih[14])) = 32
		var bits unsafe.Pointer
		hbm, _, _ := pCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bih[0])), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
		if hbm == 0 || bits == nil {
			continue
		}
		copy(unsafe.Slice((*byte)(bits), len(px)), px)
		pImageListAdd.Call(il, hbm, 0)
		pDeleteObject.Call(hbm)
	}
	return il
}

func lvInsertColumn(h uintptr, i int, text string, width int, center bool) {
	c := lvColumn{mask: LVCF_TEXT | LVCF_WIDTH | LVCF_SUBITEM | LVCF_FMT, cx: int32(width), text: u16(text), subItem: int32(i)}
	if center {
		c.fmt = LVCFMT_CENTER
	}
	sendMsg(h, LVM_INSERTCOLUMNW, uintptr(i), uintptr(unsafe.Pointer(&c)))
}

// lvSetCell sets the text and icon (-1: none) of a cell; sub 0 inserts the row.
func lvSetCell(h uintptr, row, sub int, text string, icon int) {
	it := lvItem{mask: LVIF_TEXT, item: int32(row), subItem: int32(sub), text: u16(text)}
	if icon >= 0 {
		it.mask |= LVIF_IMAGE
		it.image = int32(icon)
	}
	if sub == 0 {
		sendMsg(h, LVM_INSERTITEMW, 0, uintptr(unsafe.Pointer(&it)))
	} else {
		sendMsg(h, LVM_SETITEMW, 0, uintptr(unsafe.Pointer(&it)))
	}
}

func lvSetState(h uintptr, row int, state, mask uint32) {
	it := lvItem{state: state, stateMask: mask}
	sendMsg(h, LVM_SETITEMSTATE, uintptr(row), uintptr(unsafe.Pointer(&it)))
}

func lvChecked(h uintptr, row int) bool {
	st := uint32(sendMsg(h, LVM_GETITEMSTATE, uintptr(row), LVIS_STATEIMAGEMASK))
	return st>>12 == 2
}

func lvSetChecked(h uintptr, row int, on bool) {
	v := uint32(1) << 12
	if on {
		v = 2 << 12
	}
	lvSetState(h, row, v, LVIS_STATEIMAGEMASK)
}

func lvFocused(h uintptr) int {
	return int(int32(sendMsg(h, LVM_GETNEXTITEM, ^uintptr(0), LVNI_FOCUSED)))
}

func lvSelect(h uintptr, row int) {
	lvSetState(h, row, LVIS_FOCUSED|LVIS_SELECTED, LVIS_FOCUSED|LVIS_SELECTED)
	sendMsg(h, LVM_ENSUREVISIBLE, uintptr(row), 0)
}

// lvHit tells the row and column under a point of the list's client area.
func lvHit(h uintptr, pt point) (int, int) {
	ht := lvHitTest{pt: pt}
	sendMsg(h, LVM_SUBITEMHITTEST, 0, uintptr(unsafe.Pointer(&ht)))
	return int(ht.item), int(ht.subItem)
}

func scrollBarWidth() int {
	w, _, _ := pGetSystemMetrics.Call(SM_CXVSCROLL)
	return int(w)
}
