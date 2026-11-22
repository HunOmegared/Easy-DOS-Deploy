//go:build windows

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

// Raw access to cards in a card reader (\\.\PhysicalDriveN). Only removable
// / USB / SD disks are offered, never the disk Windows runs from.

const (
	ioctlStorageQueryProperty       = 0x2D1400
	ioctlDiskGetLengthInfo          = 0x7405C
	ioctlDiskGetGeometryEx          = 0x700A0
	ioctlVolumeGetDiskExtents       = 0x560000
	ioctlDiskUpdateProperties       = 0x70140
	fsctlLockVolume                 = 0x90018
	fsctlUnlockVolume               = 0x9001C
	fsctlDismountVolume             = 0x90020
	errAccessDenied                 = syscall.Errno(5)
	errFileNotFound                 = syscall.Errno(2)
	busUSB, busSD, busMMC           = 7, 0x0C, 0x0D
	maxCardSize               int64 = 256 << 30
)

// PhysDisk describes a disk that may be a card.
type PhysDisk struct {
	Number    int
	Model     string
	Size      int64
	Removable bool
	Bus       uint32
	Letters   []string
	System    bool
}

func (d PhysDisk) Label() string {
	l := ""
	if len(d.Letters) > 0 {
		l = "  [" + strings.Join(d.Letters, " ") + "]"
	}
	return fmt.Sprintf("%s %d: %s – %s%s", T("Disk"), d.Number, strings.TrimSpace(d.Model), humanSize(d.Size), l)
}

func (d PhysDisk) Path() string { return fmt.Sprintf(`\\.\PhysicalDrive%d`, d.Number) }

// Card reports whether the disk may be offered for raw writing.
func (d PhysDisk) Card() bool {
	return !d.System && d.Size > 0 && d.Size <= maxCardSize &&
		(d.Removable || d.Bus == busUSB || d.Bus == busSD || d.Bus == busMMC)
}

func openRaw(path string, write bool) (syscall.Handle, error) {
	access := uint32(syscall.GENERIC_READ)
	if write {
		access |= syscall.GENERIC_WRITE
	}
	return syscall.CreateFile(u16(path), access, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
}

func ioctl(h syscall.Handle, code uint32, in []byte, out []byte) (uint32, error) {
	var n uint32
	var ip, op *byte
	if len(in) > 0 {
		ip = &in[0]
	}
	if len(out) > 0 {
		op = &out[0]
	}
	err := syscall.DeviceIoControl(h, code, ip, uint32(len(in)), op, uint32(len(out)), &n, nil)
	return n, err
}

func diskLength(h syscall.Handle) int64 {
	out := make([]byte, 8)
	if _, err := ioctl(h, ioctlDiskGetLengthInfo, nil, out); err != nil {
		return 0
	}
	return int64(binary.LittleEndian.Uint64(out))
}

func cstrAt(b []byte, off uint32) string {
	if off == 0 || int(off) >= len(b) {
		return ""
	}
	end := bytes.IndexByte(b[off:], 0)
	if end < 0 {
		end = len(b) - int(off)
	}
	return strings.TrimSpace(string(b[off : int(off)+end]))
}

// volumeDisk returns the disk number of a drive letter's volume (first extent).
func volumeDisk(letter string) (int, syscall.Handle, error) {
	h, err := syscall.CreateFile(u16(`\\.\`+letter), syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		// read-only open is enough to find the disk
		h, err = syscall.CreateFile(u16(`\\.\`+letter), 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
		if err != nil {
			return -1, 0, err
		}
	}
	out := make([]byte, 256)
	if _, err := ioctl(h, ioctlVolumeGetDiskExtents, nil, out); err != nil {
		syscall.CloseHandle(h)
		return -1, 0, err
	}
	if binary.LittleEndian.Uint32(out) == 0 {
		syscall.CloseHandle(h)
		return -1, 0, errors.New("no extents")
	}
	return int(binary.LittleEndian.Uint32(out[8:])), h, nil
}

func lettersByDisk() map[int][]string {
	out := map[int][]string{}
	for c := 'A'; c <= 'Z'; c++ {
		l := string(c) + ":"
		n, h, err := volumeDisk(l)
		if err != nil {
			continue
		}
		syscall.CloseHandle(h)
		out[n] = append(out[n], l)
	}
	return out
}

// ListDisks returns all physical disks it can see.
func ListDisks() []PhysDisk {
	letters := lettersByDisk()
	sysDisk := -1
	if sd := os.Getenv("SystemDrive"); sd != "" {
		if n, h, err := volumeDisk(sd); err == nil {
			syscall.CloseHandle(h)
			sysDisk = n
		}
	}
	var out []PhysDisk
	for i := 0; i < 32; i++ {
		h, err := syscall.CreateFile(u16(fmt.Sprintf(`\\.\PhysicalDrive%d`, i)), 0,
			syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
		if err != nil {
			continue
		}
		d := PhysDisk{Number: i, Letters: letters[i], System: i == sysDisk}
		// STORAGE_PROPERTY_QUERY: StorageDeviceProperty, PropertyStandardQuery
		q := make([]byte, 12)
		desc := make([]byte, 1024)
		if _, err := ioctl(h, ioctlStorageQueryProperty, q, desc); err == nil {
			d.Removable = desc[10] != 0
			d.Bus = binary.LittleEndian.Uint32(desc[28:])
			d.Model = strings.TrimSpace(cstrAt(desc, binary.LittleEndian.Uint32(desc[12:])) + " " + cstrAt(desc, binary.LittleEndian.Uint32(desc[16:])))
		}
		// GET_DRIVE_GEOMETRY_EX works without administrator rights
		g := make([]byte, 256)
		if _, err := ioctl(h, ioctlDiskGetGeometryEx, nil, g); err == nil {
			d.Size = int64(binary.LittleEndian.Uint64(g[24:])) // 0: no card in the reader
		}
		syscall.CloseHandle(h)
		for _, l := range d.Letters {
			if strings.EqualFold(l, os.Getenv("SystemDrive")) {
				d.System = true
			}
		}
		out = append(out, d)
	}
	return out
}

// Progress is called with the phase (phaseRead, phaseWrite, phaseVerify)
// and the bytes done of that phase; it must be cheap.
type Progress func(phase int, done, total int64)

const (
	phaseRead = iota
	phaseWrite
	phaseVerify
)

func needAdmin(err error) bool { return errors.Is(err, errAccessDenied) }

// ReadDisk copies the whole disk into an image file.
func ReadDisk(d PhysDisk, path string, prog Progress) error {
	h, err := openRaw(d.Path(), false)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	size := diskLength(h)
	if size <= 0 {
		return fmt.Errorf("%s", T("There is no card in the reader (size 0)."))
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	buf := make([]byte, 1<<20)
	var done int64
	for done < size {
		n := int64(len(buf))
		if size-done < n {
			n = size - done
		}
		var got uint32
		if err := syscall.ReadFile(h, buf[:n], &got, nil); err != nil {
			f.Close()
			os.Remove(path)
			return fmt.Errorf(T("read error at %s: %v"), humanSize(done), err)
		}
		if got == 0 {
			break
		}
		if _, err := f.Write(buf[:got]); err != nil {
			f.Close()
			os.Remove(path)
			return err
		}
		done += int64(got)
		prog(phaseRead, done, size)
	}
	return f.Close()
}

// lockVolumes locks and dismounts every mounted volume of the disk so that
// Windows lets us write its sectors. The handles must stay open.
func lockVolumes(disk int) ([]syscall.Handle, error) {
	var hs []syscall.Handle
	for c := 'A'; c <= 'Z'; c++ {
		n, h, err := volumeDisk(string(c) + ":")
		if err != nil {
			continue
		}
		if n != disk {
			syscall.CloseHandle(h)
			continue
		}
		if _, err := ioctl(h, fsctlLockVolume, nil, nil); err != nil {
			syscall.CloseHandle(h)
			unlockVolumes(hs)
			if needAdmin(err) {
				return nil, err
			}
			return nil, fmt.Errorf(T("drive %c: is in use (close Explorer windows and programs using it): %v"), c, err)
		}
		ioctl(h, fsctlDismountVolume, nil, nil)
		hs = append(hs, h)
	}
	return hs, nil
}

func unlockVolumes(hs []syscall.Handle) {
	for _, h := range hs {
		ioctl(h, fsctlUnlockVolume, nil, nil)
		syscall.CloseHandle(h)
	}
}

// WriteResult says what WriteDisk did.
type WriteResult struct {
	Written, Skipped int64
	Verified         bool
}

// WriteDisk writes an image to the disk. quick leaves out areas that are
// free in the image's file system; verify reads everything back.
func WriteDisk(d PhysDisk, image string, quick, verify bool, prog Progress) (*WriteResult, error) {
	if !d.Card() {
		return nil, fmt.Errorf("%s", T("This disk is not a removable card; refusing to write to it."))
	}
	f, err := os.Open(image)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	secs, err := imageSectors(f)
	if err != nil {
		return nil, err
	}
	size := secs * 512
	var vol *diskMap
	if quick {
		vol = openDiskMap(f) // shares f; nil if the layout is unknown
	}
	// open the disk first: this is where missing administrator rights show
	h, err := openRaw(d.Path(), true)
	if err != nil {
		return nil, err
	}
	hs, err := lockVolumes(d.Number)
	if err != nil {
		syscall.CloseHandle(h)
		return nil, err
	}
	defer unlockVolumes(hs)
	defer func() {
		ioctl(h, ioctlDiskUpdateProperties, nil, nil)
		syscall.CloseHandle(h)
	}()
	disk := diskLength(h)
	if disk < size {
		return nil, fmt.Errorf(T("The image (%s) is bigger than the card (%s)."), humanSize(size), humanSize(disk))
	}
	res := &WriteResult{}
	buf := make([]byte, 1<<20)
	back := make([]byte, 1<<20)
	total := size
	if verify {
		total *= 2
	}
	var off int64
	for off < size {
		n := int64(len(buf))
		if size-off < n {
			n = size - off
		}
		if vol != nil && off >= 1<<20 && vol.Skippable(off, n) {
			res.Skipped += n
			off += n
			prog(phaseWrite, off, size)
			continue
		}
		if _, err := f.ReadAt(buf[:n], off); err != nil && err != io.EOF {
			return res, err
		}
		if _, err := syscall.Seek(h, off, 0); err != nil {
			return res, err
		}
		var w uint32
		if err := syscall.WriteFile(h, buf[:n], &w, nil); err != nil || int64(w) != n {
			return res, fmt.Errorf(T("write error at %s: %v"), humanSize(off), err)
		}
		res.Written += n
		off += n
		prog(phaseWrite, off, size)
	}
	if !verify {
		return res, nil
	}
	off = 0
	for off < size {
		n := int64(len(buf))
		if size-off < n {
			n = size - off
		}
		if vol != nil && off >= 1<<20 && vol.Skippable(off, n) {
			off += n
			prog(phaseVerify, off, size)
			continue
		}
		f.ReadAt(buf[:n], off)
		if _, err := syscall.Seek(h, off, 0); err != nil {
			return res, err
		}
		var got uint32
		if err := syscall.ReadFile(h, back[:n], &got, nil); err != nil || int64(got) != n {
			return res, fmt.Errorf(T("read-back error at %s: %v"), humanSize(off), err)
		}
		if !bytes.Equal(buf[:n], back[:n]) {
			return res, fmt.Errorf(T("The card does not hold what was written (at %s). The card or the reader may be faulty."), humanSize(off))
		}
		off += n
		prog(phaseVerify, off, size)
	}
	res.Verified = true
	return res, nil
}

// rawReader reads sectors of an open disk handle (512-byte aligned reads).
type rawReader struct{ h syscall.Handle }

func (r rawReader) ReadAt(b []byte, off int64) (int, error) {
	if _, err := syscall.Seek(r.h, off, 0); err != nil {
		return 0, err
	}
	var n uint32
	if err := syscall.ReadFile(r.h, b, &n, nil); err != nil {
		return int(n), err
	}
	if int(n) < len(b) {
		return int(n), io.ErrUnexpectedEOF
	}
	return int(n), nil
}

// InspectPhysDisk lists the partitions on a disk (needs administrator rights).
func InspectPhysDisk(d PhysDisk) ([]PartInfo, error) {
	h, err := openRaw(d.Path(), false)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(h)
	return InspectDisk(rawReader{h})
}
