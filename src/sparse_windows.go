//go:build windows

package main

import (
	"os"
	"unsafe"
)

var pDeviceIoControlSparse = kernel32.NewProc("DeviceIoControl")

// makeSparse marks a new image file sparse, so that NTFS does not fill the
// unwritten parts with zeros: writing a logical drive near the end of a
// big image would otherwise first zero gigabytes of the file.
func makeSparse(f *os.File) {
	const fsctlSetSparse = 0x000900C4
	var n uint32
	pDeviceIoControlSparse.Call(f.Fd(), fsctlSetSparse, 0, 0, 0, 0, uintptr(unsafe.Pointer(&n)), 0)
}
