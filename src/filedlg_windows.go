//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// pickItems shows the modern Windows open dialog (IFileOpenDialog, the
// one that looks like Explorer) for folders or files, several at once.
// It falls back to the old dialogs where COM fails.

var pCoCreateInstance = ole32.NewProc("CoCreateInstance")

type guid struct {
	d1     uint32
	d2, d3 uint16
	d4     [8]byte
}

var (
	clsidFileOpenDialog = guid{0xDC1C5A9C, 0xE88A, 0x4DDE, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = guid{0xD57C7288, 0xD4AD, 0x4768, [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
)

const (
	fosPickFolders      = 0x20
	fosForceFileSystem  = 0x40
	fosAllowMultiSelect = 0x200
	fosPathMustExist    = 0x800
	sigdnFileSysPath    = 0x80058000
)

// comCall calls method i of a COM object.
func comCall(obj unsafe.Pointer, i int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(i)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

func comRelease(obj unsafe.Pointer) { comCall(obj, 2) }

// shellItemPath gives the file system path of an IShellItem.
func shellItemPath(item unsafe.Pointer) string {
	var p *uint16
	if comCall(item, 5, sigdnFileSysPath, uintptr(unsafe.Pointer(&p))) != 0 || p == nil {
		return ""
	}
	defer pCoTaskMemFree.Call(uintptr(unsafe.Pointer(p)))
	return utf16PtrToString(p)
}

func utf16PtrToString(p *uint16) string {
	var s []uint16
	for ptr := unsafe.Pointer(p); *(*uint16)(ptr) != 0; ptr = unsafe.Add(ptr, 2) {
		s = append(s, *(*uint16)(ptr))
	}
	return syscall.UTF16ToString(s)
}

// pickItems returns the chosen folders (folders true) or files; nil if
// cancelled.
func pickItems(owner uintptr, title string, folders bool) []string {
	var dlg unsafe.Pointer
	const clsctxInproc = 1
	if r, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, clsctxInproc,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)), uintptr(unsafe.Pointer(&dlg))); r != 0 || dlg == nil {
		// old dialogs
		var p string
		if folders {
			p = folderDialog(owner, title)
		} else {
			p = fileDialog(owner, false, title, "", "", T("All files"), "*.*")
		}
		if p == "" {
			return nil
		}
		return []string{p}
	}
	defer comRelease(dlg)
	var opts uint32
	comCall(dlg, 10, uintptr(unsafe.Pointer(&opts))) // GetOptions
	opts |= fosForceFileSystem | fosAllowMultiSelect | fosPathMustExist
	if folders {
		opts |= fosPickFolders
	}
	comCall(dlg, 9, uintptr(opts))                        // SetOptions
	comCall(dlg, 17, uintptr(unsafe.Pointer(u16(title)))) // SetTitle
	if comCall(dlg, 3, owner) != 0 {                      // Show
		return nil // cancelled
	}
	var arr unsafe.Pointer
	if comCall(dlg, 27, uintptr(unsafe.Pointer(&arr))) != 0 || arr == nil { // GetResults
		return nil
	}
	defer comRelease(arr)
	var n uint32
	comCall(arr, 7, uintptr(unsafe.Pointer(&n))) // IShellItemArray::GetCount
	var out []string
	for i := uint32(0); i < n; i++ {
		var item unsafe.Pointer
		if comCall(arr, 8, uintptr(i), uintptr(unsafe.Pointer(&item))) == 0 && item != nil { // GetItemAt
			if p := shellItemPath(item); p != "" {
				out = append(out, p)
			}
			comRelease(item)
		}
	}
	return out
}
