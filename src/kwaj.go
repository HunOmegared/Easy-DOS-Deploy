package main

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// KWAJ is the compressed file format of the MS-DOS 6.x Setup disks (the
// files whose name ends in "_", expanded by EXPAND.EXE). The header is
// "KWAJ" 88 F0 27 D1, the method, the data offset and flags, then optional
// fields chosen by the flags. Method 3 (LZ with Huffman codes) is what the
// MS-DOS 6.x disks use; 0 (stored) and 1 (XOR FF) are trivial.

var kwajMagic = []byte{'K', 'W', 'A', 'J', 0x88, 0xF0, 0x27, 0xD1}

// IsKWAJ tells whether data is a KWAJ compressed file.
func IsKWAJ(b []byte) bool {
	return len(b) >= 14 && string(b[:8]) == string(kwajMagic)
}

var errKWAJ = errors.New("corrupt compressed file")

// ExpandKWAJ expands a KWAJ compressed file.
func ExpandKWAJ(b []byte) ([]byte, error) {
	if !IsKWAJ(b) {
		return nil, errors.New("not a compressed file")
	}
	method := binary.LittleEndian.Uint16(b[8:])
	off := int(binary.LittleEndian.Uint16(b[10:]))
	flags := binary.LittleEndian.Uint16(b[12:])
	size := -1
	if flags&1 != 0 && len(b) >= 18 {
		size = int(binary.LittleEndian.Uint32(b[14:]))
	}
	if off > len(b) || off < 14 {
		return nil, errKWAJ
	}
	data := b[off:]
	var out []byte
	switch method {
	case 0:
		out = append([]byte(nil), data...)
	case 1:
		out = make([]byte, len(data))
		for i, c := range data {
			out[i] = c ^ 0xFF
		}
	case 3:
		var err error
		if out, err = kwajLZH(data, size); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("compression method %d is not supported", method)
	}
	if size >= 0 {
		if len(out) < size {
			return nil, errKWAJ
		}
		out = out[:size]
	}
	return out, nil
}

// kwajBits reads bits most significant first.
type kwajBits struct {
	b    []byte
	p    int
	buf  uint32
	n    uint
	over int // bytes read past the end (zero padding)
}

func (r *kwajBits) need(k uint) {
	for r.n < k {
		var c byte
		if r.p < len(r.b) {
			c = r.b[r.p]
		} else {
			r.over++
		}
		r.p++
		r.buf = r.buf<<8 | uint32(c)
		r.n += 8
	}
}

func (r *kwajBits) bits(k uint) int {
	r.need(k)
	r.n -= k
	return int(r.buf>>r.n) & (1<<k - 1)
}

func (r *kwajBits) peek(k uint) int {
	r.need(k)
	return int(r.buf>>(r.n-k)) & (1<<k - 1)
}

func (r *kwajBits) drop(k uint) { r.n -= k }

func (r *kwajBits) done() bool { return r.over > 2 }

// kwajTree is a canonical Huffman code decoded through a 16 bit table.
type kwajTree struct {
	sym []uint16
	len []uint8
}

const kwajTableBits = 16

func kwajMakeTree(lens []int) (*kwajTree, error) {
	t := &kwajTree{sym: make([]uint16, 1<<kwajTableBits), len: make([]uint8, 1<<kwajTableBits)}
	code := 0
	for l := 1; l <= 16; l++ {
		for s, sl := range lens {
			if sl != l {
				continue
			}
			first := code << (kwajTableBits - l)
			last := first + 1<<(kwajTableBits-l)
			if last > len(t.sym) {
				return nil, errKWAJ
			}
			for i := first; i < last; i++ {
				t.sym[i], t.len[i] = uint16(s), uint8(l)
			}
			code++
		}
		code <<= 1
	}
	return t, nil
}

func (t *kwajTree) read(r *kwajBits) (int, error) {
	i := r.peek(kwajTableBits)
	l := t.len[i]
	if l == 0 {
		return 0, errKWAJ
	}
	r.drop(uint(l))
	return int(t.sym[i]), nil
}

// kwajLens reads the code lengths of one table in one of four encodings.
func kwajLens(r *kwajBits, typ, n int) ([]int, error) {
	l := make([]int, n)
	switch typ {
	case 0:
		c := map[int]int{16: 4, 32: 5, 64: 6, 256: 8}[n]
		for i := range l {
			l[i] = c
		}
	case 1:
		c := r.bits(4)
		l[0] = c
		for i := 1; i < n; i++ {
			if r.bits(1) == 0 {
			} else if r.bits(1) == 0 {
				c++
			} else {
				c = r.bits(4)
			}
			l[i] = c
		}
	case 2:
		c := r.bits(4)
		l[0] = c
		for i := 1; i < n; i++ {
			if s := r.bits(2); s == 3 {
				c = r.bits(4)
			} else {
				c += s - 1
			}
			l[i] = c
		}
	case 3:
		for i := range l {
			l[i] = r.bits(4)
		}
	default:
		return nil, errKWAJ
	}
	for _, v := range l {
		if v < 0 || v > 16 {
			return nil, errKWAJ
		}
	}
	return l, nil
}

func kwajLZH(data []byte, size int) ([]byte, error) {
	r := &kwajBits{b: data}
	var types [6]int
	for i := range types {
		types[i] = r.bits(4)
	}
	// match length after a match, after a literal run, literal run length,
	// offset (high 6 bits), literal
	var trees [5]*kwajTree
	for i, n := range []int{16, 16, 32, 64, 256} {
		l, err := kwajLens(r, types[i], n)
		if err != nil {
			return nil, err
		}
		if trees[i], err = kwajMakeTree(l); err != nil {
			return nil, err
		}
	}
	var win [4096]byte
	for i := range win {
		win[i] = ' '
	}
	out := make([]byte, 0, max(size, 0))
	pos, run := 0, false
	full := func() bool { return size >= 0 && len(out) >= size }
	for !full() && !r.done() {
		t := trees[0]
		if run {
			t = trees[1]
		}
		n, err := t.read(r)
		if err != nil {
			if size < 0 {
				break
			}
			return nil, err
		}
		if n > 0 {
			n += 2
			run = false
			hi, err := trees[3].read(r)
			if err != nil {
				return nil, err
			}
			o := hi<<6 | r.bits(6)
			for ; n > 0; n-- {
				c := win[(pos-o)&4095]
				win[pos] = c
				out = append(out, c)
				pos = (pos + 1) & 4095
			}
		} else {
			n, err = trees[2].read(r)
			if err != nil {
				return nil, err
			}
			n++
			run = n != 32
			for ; n > 0; n-- {
				c, err := trees[4].read(r)
				if err != nil {
					return nil, err
				}
				win[pos] = byte(c)
				out = append(out, byte(c))
				pos = (pos + 1) & 4095
			}
		}
	}
	return out, nil
}
