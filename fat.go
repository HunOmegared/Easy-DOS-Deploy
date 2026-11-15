package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Volume is a FAT12/16/32 file system inside a disk image file: a raw
// floppy image (.img/.ima), a raw hard disk image with an MBR (86Box .img,
// .hdd), or a fixed-size .vhd. Only 8.3 names are written.
type Volume struct {
	f        *os.File
	base     int64 // byte offset of the volume inside the file
	bps      uint32
	spc      uint32
	rsvd     uint32
	nfats    uint32
	rootEnts uint32
	fatSecs  uint32
	totSecs  uint32
	FatType  int
	rootClus uint32 // FAT32
	rootSec  uint32 // FAT12/16 fixed root directory
	rootSecs uint32
	dataSec  uint32
	nclus    uint32 // valid cluster numbers: 2 .. nclus+1
	fsinfo   uint32
	fat      []byte
	Desc     string
}

var ErrNotFound = errors.New("not found")

func le16(b []byte) uint32 { return uint32(binary.LittleEndian.Uint16(b)) }
func le32(b []byte) uint32 { return binary.LittleEndian.Uint32(b) }

func looksLikeBPB(s []byte) bool {
	if len(s) < 512 {
		return false
	}
	if s[0] != 0xEB && s[0] != 0xE9 {
		return false
	}
	bps := le16(s[11:])
	spc := uint32(s[13])
	if bps != 512 && bps != 1024 && bps != 2048 && bps != 4096 {
		return false
	}
	if spc == 0 || spc&(spc-1) != 0 {
		return false
	}
	return le16(s[14:]) >= 1 && s[16] >= 1 && s[16] <= 2
}

func isFATPartType(t byte) bool {
	switch t {
	case 0x01, 0x04, 0x06, 0x0B, 0x0C, 0x0E, 0x14, 0x16, 0x1B, 0x1C, 0x1E:
		return true
	}
	return false
}

// OpenImage opens the first usable FAT volume in the image.
func OpenImage(path string, writable bool) (*Volume, error) {
	flag := os.O_RDONLY
	if writable {
		flag = os.O_RDWR
	}
	f, err := os.OpenFile(path, flag, 0)
	if err != nil {
		return nil, err
	}
	v, err := openVolume(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return v, nil
}

func openVolume(f *os.File) (*Volume, error) {
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	head := make([]byte, 512)
	if _, err := f.ReadAt(head, 0); err != nil {
		return nil, fmt.Errorf("%s", T("the file is too short to be a disk image"))
	}
	if string(head[:8]) == "conectix" {
		return nil, fmt.Errorf("%s", T("this is a dynamic (growing) VHD; only fixed-size VHDs are supported. Convert it to fixed size or use a raw .img image"))
	}
	if size >= 1024 {
		foot := make([]byte, 512)
		if _, err := f.ReadAt(foot, size-512); err == nil && string(foot[:8]) == "conectix" {
			if binary.BigEndian.Uint32(foot[60:]) != 2 {
				return nil, fmt.Errorf("%s", T("this is not a fixed-size VHD; only fixed-size VHDs are supported"))
			}
		}
	}
	if looksLikeBPB(head) && !hasMBRWithFAT(head) {
		v := &Volume{f: f}
		if err := v.load(0); err != nil {
			return nil, err
		}
		v.Desc = T("FAT%d, %s (no partition table)", v.FatType, humanSize(int64(v.totSecs)*int64(v.bps)))
		return v, nil
	}
	if head[510] != 0x55 || head[511] != 0xAA {
		return nil, fmt.Errorf("%s", T("unknown image format (no boot sector and no partition table)"))
	}
	type part struct {
		n      int
		active bool
		lba    uint32
	}
	var parts []part
	for i := 0; i < 4; i++ {
		e := head[446+16*i:]
		if isFATPartType(e[4]) && le32(e[8:]) > 0 {
			parts = append(parts, part{i + 1, e[0] == 0x80, le32(e[8:])})
		}
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("%s", T("the partition table has no FAT partition (is the image formatted?)"))
	}
	// prefer the active (boot) partition
	for i, p := range parts {
		if p.active && i != 0 {
			parts[0], parts[i] = parts[i], parts[0]
			break
		}
	}
	var lastErr error
	for _, p := range parts {
		v := &Volume{f: f}
		if err := v.load(int64(p.lba) * 512); err != nil {
			lastErr = err
			continue
		}
		v.Desc = T("partition %d, FAT%d, %s", p.n, v.FatType, humanSize(int64(v.totSecs)*int64(v.bps)))
		return v, nil
	}
	return nil, fmt.Errorf(T("the FAT partition cannot be read: %v"), lastErr)
}

func hasMBRWithFAT(head []byte) bool {
	if head[510] != 0x55 || head[511] != 0xAA {
		return false
	}
	// A real boot sector's BPB fields overlap the partition table area only
	// with boot code; check that the table looks sane and points somewhere.
	n := 0
	for i := 0; i < 4; i++ {
		e := head[446+16*i:]
		if e[0] != 0 && e[0] != 0x80 {
			return false
		}
		if isFATPartType(e[4]) && le32(e[8:]) > 0 && le32(e[12:]) > 0 {
			n++
		}
	}
	return n > 0
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%d KB", n>>10)
	}
}

func (v *Volume) load(base int64) error {
	b := make([]byte, 512)
	if _, err := v.f.ReadAt(b, base); err != nil {
		return err
	}
	if !looksLikeBPB(b) {
		return fmt.Errorf("%s", T("no valid FAT boot sector"))
	}
	v.base = base
	v.bps = le16(b[11:])
	v.spc = uint32(b[13])
	v.rsvd = le16(b[14:])
	v.nfats = uint32(b[16])
	v.rootEnts = le16(b[17:])
	v.totSecs = le16(b[19:])
	if v.totSecs == 0 {
		v.totSecs = le32(b[32:])
	}
	v.fatSecs = le16(b[22:])
	if v.fatSecs == 0 {
		v.fatSecs = le32(b[36:])
	}
	if v.fatSecs == 0 || v.totSecs == 0 {
		return fmt.Errorf("%s", T("invalid boot sector (BPB)"))
	}
	v.rootSecs = (v.rootEnts*32 + v.bps - 1) / v.bps
	v.rootSec = v.rsvd + v.nfats*v.fatSecs
	v.dataSec = v.rootSec + v.rootSecs
	if v.dataSec >= v.totSecs {
		return fmt.Errorf("%s", T("invalid boot sector (BPB)"))
	}
	v.nclus = (v.totSecs - v.dataSec) / v.spc
	switch {
	case v.nclus < 4085:
		v.FatType = 12
	case v.nclus < 65525:
		v.FatType = 16
	default:
		v.FatType = 32
		v.rootClus = le32(b[44:])
		v.fsinfo = le16(b[48:])
	}
	if v.FatType != 32 && v.rootEnts == 0 {
		return fmt.Errorf("%s", T("invalid boot sector (BPB)"))
	}
	maxFat := (v.nclus + 2) * uint32(v.FatType) / 8
	if v.fatSecs*v.bps < maxFat {
		v.nclus = v.fatSecs*v.bps*8/uint32(v.FatType) - 2
	}
	v.fat = make([]byte, v.fatSecs*v.bps)
	if _, err := v.f.ReadAt(v.fat, v.secOff(v.rsvd)); err != nil {
		return fmt.Errorf(T("the FAT cannot be read: %v"), err)
	}
	return nil
}

func (v *Volume) Close() error { return v.f.Close() }

func (v *Volume) secOff(sec uint32) int64 { return v.base + int64(sec)*int64(v.bps) }
func (v *Volume) clusSize() uint32        { return v.spc * v.bps }
func (v *Volume) clusOff(c uint32) int64  { return v.secOff(v.dataSec + (c-2)*v.spc) }

func (v *Volume) get(n uint32) uint32 {
	switch v.FatType {
	case 12:
		off := n + n/2
		x := le16(v.fat[off:])
		if n&1 != 0 {
			return x >> 4
		}
		return x & 0xFFF
	case 16:
		return le16(v.fat[2*n:])
	}
	return le32(v.fat[4*n:]) & 0x0FFFFFFF
}

func (v *Volume) set(n, val uint32) {
	switch v.FatType {
	case 12:
		off := n + n/2
		if n&1 != 0 {
			v.fat[off] = v.fat[off]&0x0F | byte(val<<4)
			v.fat[off+1] = byte(val >> 4)
		} else {
			v.fat[off] = byte(val)
			v.fat[off+1] = v.fat[off+1]&0xF0 | byte(val>>8)&0x0F
		}
	case 16:
		binary.LittleEndian.PutUint16(v.fat[2*n:], uint16(val))
	default:
		old := le32(v.fat[4*n:])
		binary.LittleEndian.PutUint32(v.fat[4*n:], old&0xF0000000|val&0x0FFFFFFF)
	}
}

func (v *Volume) eoc() uint32 {
	switch v.FatType {
	case 12:
		return 0xFFF
	case 16:
		return 0xFFFF
	}
	return 0x0FFFFFFF
}

func (v *Volume) isEOC(x uint32) bool {
	switch v.FatType {
	case 12:
		return x >= 0xFF8
	case 16:
		return x >= 0xFFF8
	}
	return x >= 0x0FFFFFF8
}

func (v *Volume) validClus(c uint32) bool { return c >= 2 && c < v.nclus+2 }

// chain returns the cluster list starting at c.
func (v *Volume) chain(c uint32) ([]uint32, error) {
	var out []uint32
	for v.validClus(c) {
		out = append(out, c)
		if uint32(len(out)) > v.nclus {
			return nil, fmt.Errorf("%s", T("damaged FAT (circular chain)"))
		}
		c = v.get(c)
	}
	if c != 0 && !v.isEOC(c) && len(out) > 0 {
		return nil, fmt.Errorf("%s", T("damaged FAT chain"))
	}
	return out, nil
}

type fullErr struct{}

func (fullErr) Error() string { return T("the image does not have enough free space") }

var errFull error = fullErr{}

// alloc reserves n free clusters, linked, and returns them.
func (v *Volume) alloc(n uint32) ([]uint32, error) {
	if n == 0 {
		return nil, nil
	}
	var got []uint32
	for c := uint32(2); c < v.nclus+2 && uint32(len(got)) < n; c++ {
		if v.get(c) == 0 {
			got = append(got, c)
		}
	}
	if uint32(len(got)) < n {
		return nil, errFull
	}
	for i, c := range got {
		if i+1 < len(got) {
			v.set(c, got[i+1])
		} else {
			v.set(c, v.eoc())
		}
	}
	return got, nil
}

func (v *Volume) free(cs []uint32) {
	for _, c := range cs {
		v.set(c, 0)
	}
}

func (v *Volume) FreeBytes() int64 {
	var n int64
	for c := uint32(2); c < v.nclus+2; c++ {
		if v.get(c) == 0 {
			n++
		}
	}
	return n * int64(v.clusSize())
}

func (v *Volume) flushFAT() error {
	for i := uint32(0); i < v.nfats; i++ {
		if _, err := v.f.WriteAt(v.fat, v.secOff(v.rsvd+i*v.fatSecs)); err != nil {
			return err
		}
	}
	if v.FatType == 32 && v.fsinfo != 0 && v.fsinfo != 0xFFFF {
		// Mark the cached free-cluster count as unknown; DOS recounts it.
		b := make([]byte, 512)
		if _, err := v.f.ReadAt(b, v.secOff(v.fsinfo)); err == nil && le32(b) == 0x41615252 {
			binary.LittleEndian.PutUint32(b[488:], 0xFFFFFFFF)
			binary.LittleEndian.PutUint32(b[492:], 0xFFFFFFFF)
			v.f.WriteAt(b, v.secOff(v.fsinfo))
		}
	}
	return nil
}

func (v *Volume) reloadFAT() {
	v.f.ReadAt(v.fat, v.secOff(v.rsvd))
}

// ---- directories ----

// dir identifies a directory: clus==0 means the fixed FAT12/16 root.
type dir struct{ clus uint32 }

func (v *Volume) root() dir {
	if v.FatType == 32 {
		return dir{v.rootClus}
	}
	return dir{0}
}

// entryOffsets lists the byte offset of every 32-byte slot of d.
func (v *Volume) slots(d dir) ([]int64, error) {
	var out []int64
	if d.clus == 0 {
		start := v.secOff(v.rootSec)
		for i := uint32(0); i < v.rootEnts; i++ {
			out = append(out, start+int64(i)*32)
		}
		return out, nil
	}
	cs, err := v.chain(d.clus)
	if err != nil {
		return nil, err
	}
	per := v.clusSize() / 32
	for _, c := range cs {
		o := v.clusOff(c)
		for i := uint32(0); i < per; i++ {
			out = append(out, o+int64(i)*32)
		}
	}
	return out, nil
}

type dirEntry struct {
	off  int64
	raw  [32]byte
	Name string
}

func (e *dirEntry) attr() byte    { return e.raw[11] }
func (e *dirEntry) isDir() bool   { return e.raw[11]&0x10 != 0 }
func (e *dirEntry) size() uint32  { return le32(e.raw[28:]) }
func (e *dirEntry) first() uint32 { return le16(e.raw[26:]) | le16(e.raw[20:])<<16 }

func name83String(n []byte) string {
	base := strings.TrimRight(string(n[:8]), " ")
	if base != "" && base[0] == 0x05 {
		base = "\xE5" + base[1:]
	}
	ext := strings.TrimRight(string(n[8:11]), " ")
	if ext == "" {
		return base
	}
	return base + "." + ext
}

// readDir returns the live entries and the offset of the first usable free slot.
func (v *Volume) readDir(d dir) ([]dirEntry, []int64, error) {
	slots, err := v.slots(d)
	if err != nil {
		return nil, nil, err
	}
	var ents []dirEntry
	var free []int64
	buf := make([]byte, 32)
	ended := false
	for _, o := range slots {
		if ended {
			free = append(free, o)
			continue
		}
		if _, err := v.f.ReadAt(buf, o); err != nil {
			return nil, nil, err
		}
		switch {
		case buf[0] == 0x00:
			ended = true
			free = append(free, o)
		case buf[0] == 0xE5:
			free = append(free, o)
		case buf[11] == 0x0F: // long name part
		case buf[11]&0x08 != 0: // volume label
		default:
			var e dirEntry
			e.off = o
			copy(e.raw[:], buf)
			e.Name = name83String(buf[:11])
			ents = append(ents, e)
		}
	}
	return ents, free, nil
}

// To83 converts a file name to the 11-byte directory form.
func To83(s string) ([11]byte, error) {
	var out [11]byte
	for i := range out {
		out[i] = ' '
	}
	u := strings.ToUpper(strings.TrimSpace(s))
	if u == "" || u == "." || u == ".." {
		return out, fmt.Errorf(T("invalid name: %q"), s)
	}
	base, ext := u, ""
	if i := strings.LastIndex(u, "."); i >= 0 {
		base, ext = u[:i], u[i+1:]
	}
	if len(base) == 0 || len(base) > 8 || len(ext) > 3 {
		return out, fmt.Errorf(T("%q is not an 8.3 name"), s)
	}
	for _, c := range base + ext {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'()-@^_`{}~", c)) {
			return out, fmt.Errorf(T("%q contains a character that is not allowed"), s)
		}
	}
	copy(out[:], base)
	copy(out[8:], ext)
	return out, nil
}

func splitPath(p string) []string {
	p = strings.ReplaceAll(p, "/", `\`)
	if len(p) >= 2 && p[1] == ':' {
		p = p[2:]
	}
	var out []string
	for _, s := range strings.Split(p, `\`) {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (v *Volume) find(d dir, name string) (*dirEntry, error) {
	n, err := To83(name)
	if err != nil {
		return nil, err
	}
	want := name83String(n[:])
	ents, _, err := v.readDir(d)
	if err != nil {
		return nil, err
	}
	for i := range ents {
		if ents[i].Name == want {
			return &ents[i], nil
		}
	}
	return nil, ErrNotFound
}

func dosTime(t time.Time) (uint16, uint16) {
	y := t.Year()
	if y < 1980 {
		y = 1980
	}
	d := uint16((y-1980)<<9 | int(t.Month())<<5 | t.Day())
	tm := uint16(t.Hour()<<11 | t.Minute()<<5 | t.Second()/2)
	return d, tm
}

func makeEntry(name [11]byte, attr byte, first, size uint32) [32]byte {
	var e [32]byte
	copy(e[:11], name[:])
	e[11] = attr
	d, t := dosTime(time.Now())
	binary.LittleEndian.PutUint16(e[14:], t)
	binary.LittleEndian.PutUint16(e[16:], d)
	binary.LittleEndian.PutUint16(e[18:], d)
	binary.LittleEndian.PutUint16(e[20:], uint16(first>>16))
	binary.LittleEndian.PutUint16(e[22:], t)
	binary.LittleEndian.PutUint16(e[24:], d)
	binary.LittleEndian.PutUint16(e[26:], uint16(first))
	binary.LittleEndian.PutUint32(e[28:], size)
	return e
}

// addEntry writes a new entry into d, growing a cluster-chain directory if
// needed. FAT must be flushed by the caller afterwards.
func (v *Volume) addEntry(d dir, e [32]byte) error {
	_, free, err := v.readDir(d)
	if err != nil {
		return err
	}
	if len(free) == 0 {
		if d.clus == 0 {
			return fmt.Errorf(T("the root directory is full (at most %d entries)"), v.rootEnts)
		}
		cs, err := v.chain(d.clus)
		if err != nil {
			return err
		}
		nc, err := v.alloc(1)
		if err != nil {
			return err
		}
		if err := v.zeroCluster(nc[0]); err != nil {
			return err
		}
		v.set(cs[len(cs)-1], nc[0])
		free = []int64{v.clusOff(nc[0])}
	}
	o := free[0]
	// If we take the end-of-directory slot, make sure the next slot still
	// marks the end (it might contain old garbage).
	slots, _ := v.slots(d)
	for i, s := range slots {
		if s == o && i+1 < len(slots) {
			b := make([]byte, 1)
			v.f.ReadAt(b, o)
			if b[0] == 0x00 {
				v.f.WriteAt([]byte{0}, slots[i+1])
			}
		}
	}
	_, err = v.f.WriteAt(e[:], o)
	return err
}

func (v *Volume) zeroCluster(c uint32) error {
	_, err := v.f.WriteAt(make([]byte, v.clusSize()), v.clusOff(c))
	return err
}

// mkdirAll walks/creates the directories in parts.
func (v *Volume) mkdirAll(parts []string, create bool) (dir, error) {
	d := v.root()
	for _, p := range parts {
		e, err := v.find(d, p)
		if err == nil {
			if !e.isDir() {
				return d, fmt.Errorf(T("%s is a file, not a folder"), p)
			}
			d = dir{e.first()}
			if d.clus == 0 {
				d = v.root()
			}
			continue
		}
		if err != ErrNotFound || !create {
			return d, err
		}
		name, err := To83(p)
		if err != nil {
			return d, err
		}
		nc, err := v.alloc(1)
		if err != nil {
			return d, err
		}
		c := nc[0]
		if err := v.zeroCluster(c); err != nil {
			return d, err
		}
		var dot, dotdot [11]byte
		copy(dot[:], ".          ")
		copy(dotdot[:], "..         ")
		parent := d.clus
		if v.FatType == 32 && parent == v.rootClus {
			parent = 0
		}
		e1 := makeEntry(dot, 0x10, c, 0)
		e2 := makeEntry(dotdot, 0x10, parent, 0)
		v.f.WriteAt(e1[:], v.clusOff(c))
		v.f.WriteAt(e2[:], v.clusOff(c)+32)
		if err := v.addEntry(d, makeEntry(name, 0x10, c, 0)); err != nil {
			v.reloadFAT()
			return d, err
		}
		if err := v.flushFAT(); err != nil {
			return d, err
		}
		d = dir{c}
	}
	return d, nil
}

// ReadFile returns the contents of a file on the volume.
func (v *Volume) ReadFile(p string) ([]byte, error) {
	parts := splitPath(p)
	if len(parts) == 0 {
		return nil, ErrNotFound
	}
	d, err := v.mkdirAll(parts[:len(parts)-1], false)
	if err != nil {
		return nil, err
	}
	e, err := v.find(d, parts[len(parts)-1])
	if err != nil {
		return nil, err
	}
	if e.isDir() {
		return nil, fmt.Errorf(T("%s is a folder"), p)
	}
	cs, err := v.chain(e.first())
	if err != nil {
		return nil, err
	}
	size := int64(e.size())
	if int64(len(cs))*int64(v.clusSize()) < size {
		return nil, fmt.Errorf(T("%s: the FAT chain is shorter than the file"), p)
	}
	out := make([]byte, 0, size)
	buf := make([]byte, v.clusSize())
	for _, c := range cs {
		if int64(len(out)) >= size {
			break
		}
		if _, err := v.f.ReadAt(buf, v.clusOff(c)); err != nil && err != io.EOF {
			return nil, err
		}
		n := size - int64(len(out))
		if n > int64(len(buf)) {
			n = int64(len(buf))
		}
		out = append(out, buf[:n]...)
	}
	return out, nil
}

func (v *Volume) Exists(p string) bool {
	parts := splitPath(p)
	if len(parts) == 0 {
		return false
	}
	d, err := v.mkdirAll(parts[:len(parts)-1], false)
	if err != nil {
		return false
	}
	_, err = v.find(d, parts[len(parts)-1])
	return err == nil
}

// WriteFile creates or replaces a file, creating folders as needed.
func (v *Volume) WriteFile(p string, data []byte) error {
	parts := splitPath(p)
	if len(parts) == 0 {
		return fmt.Errorf("%s", T("empty path"))
	}
	name, err := To83(parts[len(parts)-1])
	if err != nil {
		return err
	}
	for _, s := range parts[:len(parts)-1] {
		if _, err := To83(s); err != nil {
			return err
		}
	}
	d, err := v.mkdirAll(parts[:len(parts)-1], true)
	if err != nil {
		return err
	}
	old, err := v.find(d, parts[len(parts)-1])
	if err != nil && err != ErrNotFound {
		return err
	}
	if old != nil && old.isDir() {
		return fmt.Errorf(T("%s is the name of a folder"), p)
	}
	cs := v.clusSize()
	need := (uint32(len(data)) + cs - 1) / cs
	var oldChain []uint32
	if old != nil {
		if oldChain, err = v.chain(old.first()); err != nil {
			return err
		}
	}
	// Allocate before freeing so the old file stays intact if anything fails.
	clus, err := v.alloc(need)
	if err == errFull && old != nil {
		v.free(oldChain)
		oldChain = nil
		clus, err = v.alloc(need)
	}
	if err != nil {
		v.reloadFAT()
		return err
	}
	buf := make([]byte, cs)
	for i, c := range clus {
		for j := range buf {
			buf[j] = 0
		}
		copy(buf, data[uint32(i)*cs:])
		if _, err := v.f.WriteAt(buf, v.clusOff(c)); err != nil {
			v.reloadFAT()
			return err
		}
	}
	first := uint32(0)
	if len(clus) > 0 {
		first = clus[0]
	}
	v.free(oldChain)
	if old != nil {
		e := makeEntry(name, old.attr()|0x20, first, uint32(len(data)))
		copy(e[13:20], old.raw[13:20]) // keep the creation date
		if err := v.flushFAT(); err != nil {
			return err
		}
		_, err = v.f.WriteAt(e[:], old.off)
		return err
	}
	if err := v.addEntry(d, makeEntry(name, 0x20, first, uint32(len(data)))); err != nil {
		v.reloadFAT()
		return err
	}
	return v.flushFAT()
}

// List returns the names in a folder (for checks such as "is it bootable").
func (v *Volume) List(p string) ([]string, error) {
	d, err := v.mkdirAll(splitPath(p), false)
	if err != nil {
		return nil, err
	}
	ents, _, err := v.readDir(d)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		out = append(out, e.Name)
	}
	return out, nil
}

func (v *Volume) Sync() error { return v.f.Sync() }

// SetAttr changes the attribute byte of a file or folder.
func (v *Volume) SetAttr(p string, attr byte) error {
	parts := splitPath(p)
	if len(parts) == 0 {
		return ErrNotFound
	}
	d, err := v.mkdirAll(parts[:len(parts)-1], false)
	if err != nil {
		return err
	}
	e, err := v.find(d, parts[len(parts)-1])
	if err != nil {
		return err
	}
	_, err = v.f.WriteAt([]byte{attr}, e.off+11)
	return err
}

// FileInfo describes one entry found by Walk.
type FileInfo struct {
	Path string // backslash separated, no drive
	Dir  bool
	Attr byte
	Size uint32
}

// Walk visits every file and folder, parents before children.
func (v *Volume) Walk(fn func(FileInfo) error) error {
	var walk func(d dir, prefix string, depth int) error
	walk = func(d dir, prefix string, depth int) error {
		if depth > 32 {
			return fmt.Errorf("%s", T("damaged FAT (circular chain)"))
		}
		ents, _, err := v.readDir(d)
		if err != nil {
			return err
		}
		for _, e := range ents {
			if e.Name == "." || e.Name == ".." {
				continue
			}
			p := prefix + e.Name
			if err := fn(FileInfo{Path: p, Dir: e.isDir(), Attr: e.attr(), Size: e.size()}); err != nil {
				return err
			}
			if e.isDir() && e.first() != 0 {
				if err := walk(dir{e.first()}, p+`\`, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(v.root(), "", 0)
}

// Skippable reports whether bytes [off, off+n) of the image file carry no
// information for this file system: they lie entirely in free clusters of
// the data area, or past the end of the volume. A raw copy may leave such
// areas out.
func (v *Volume) Skippable(off, n int64) bool {
	volEnd := v.base + int64(v.totSecs)*int64(v.bps)
	if off >= volEnd {
		return true
	}
	dataStart := v.secOff(v.dataSec)
	if off < dataStart {
		return false
	}
	cs := int64(v.clusSize())
	end := off + n
	if end > volEnd {
		end = volEnd
	}
	first := uint32((off-dataStart)/cs) + 2
	last := uint32((end-1-dataStart)/cs) + 2
	for c := first; c <= last; c++ {
		if c >= v.nclus+2 {
			break // slack after the last cluster
		}
		if v.get(c) != 0 {
			return false
		}
	}
	return true
}
