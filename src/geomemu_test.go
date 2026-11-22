package main

import (
	"fmt"
	"strings"
	"testing"
)

// A small 8086 emulator for GEOM.COM: the instructions it uses, a BIOS data
// area, INT 13h for several disks, DOS output and an IDE controller with
// devices that answer IDENTIFY DEVICE.

type emu86 struct {
	mem            []byte
	ax, bx, cx, dx uint16
	si, di, bp, sp uint16
	cs, ds, es, ss uint16
	ip             uint16
	cf, zf, sf     bool
	seg            *uint16 // segment override
	in             func(port uint16, word bool) uint16
	out            func(port uint16, v uint16, word bool)
	intr           func(n byte) bool // false: stop
	steps          int
}

func (e *emu86) lin(s, o uint16) int { return (int(s)<<4 + int(o)) & 0xFFFFF }
func (e *emu86) rb(s, o uint16) byte { return e.mem[e.lin(s, o)] }
func (e *emu86) rw(s, o uint16) uint16 {
	return uint16(e.rb(s, o)) | uint16(e.rb(s, o+1))<<8
}
func (e *emu86) wb(s, o uint16, v byte) { e.mem[e.lin(s, o)] = v }
func (e *emu86) ww(s, o uint16, v uint16) {
	e.wb(s, o, byte(v))
	e.wb(s, o+1, byte(v>>8))
}
func (e *emu86) fb() byte {
	v := e.rb(e.cs, e.ip)
	e.ip++
	return v
}
func (e *emu86) fw() uint16 {
	v := e.rw(e.cs, e.ip)
	e.ip += 2
	return v
}
func (e *emu86) push(v uint16) { e.sp -= 2; e.ww(e.ss, e.sp, v) }
func (e *emu86) pop() uint16 {
	v := e.rw(e.ss, e.sp)
	e.sp += 2
	return v
}

func (e *emu86) r16(i byte) *uint16 {
	return [...]*uint16{&e.ax, &e.cx, &e.dx, &e.bx, &e.sp, &e.bp, &e.si, &e.di}[i&7]
}
func (e *emu86) g8(i byte) byte {
	r := *e.r16(i & 3)
	if i&4 != 0 {
		return byte(r >> 8)
	}
	return byte(r)
}
func (e *emu86) s8(i byte, v byte) {
	r := e.r16(i & 3)
	if i&4 != 0 {
		*r = *r&0x00FF | uint16(v)<<8
	} else {
		*r = *r&0xFF00 | uint16(v)
	}
}

// operand of a ModRM byte: a register or a memory address
type opnd struct {
	reg      bool
	idx      byte
	seg, off uint16
}

func (e *emu86) modrm() (byte, opnd) {
	m := e.fb()
	mod, reg, rm := m>>6, m>>3&7, m&7
	if mod == 3 {
		return reg, opnd{reg: true, idx: rm}
	}
	var off uint16
	seg := e.ds
	switch rm {
	case 0:
		off = e.bx + e.si
	case 1:
		off = e.bx + e.di
	case 2:
		off, seg = e.bp+e.si, e.ss
	case 3:
		off, seg = e.bp+e.di, e.ss
	case 4:
		off = e.si
	case 5:
		off = e.di
	case 6:
		if mod == 0 {
			off = e.fw()
		} else {
			off, seg = e.bp, e.ss
		}
	case 7:
		off = e.bx
	}
	switch mod {
	case 1:
		off += uint16(int16(int8(e.fb())))
	case 2:
		off += e.fw()
	}
	if e.seg != nil {
		seg = *e.seg
	}
	return reg, opnd{seg: seg, off: off}
}

func (e *emu86) get(o opnd, w bool) uint16 {
	switch {
	case o.reg && w:
		return *e.r16(o.idx)
	case o.reg:
		return uint16(e.g8(o.idx))
	case w:
		return e.rw(o.seg, o.off)
	}
	return uint16(e.rb(o.seg, o.off))
}

func (e *emu86) set(o opnd, w bool, v uint16) {
	switch {
	case o.reg && w:
		*e.r16(o.idx) = v
	case o.reg:
		e.s8(o.idx, byte(v))
	case w:
		e.ww(o.seg, o.off, v)
	default:
		e.wb(o.seg, o.off, byte(v))
	}
}

func (e *emu86) setZS(v uint16, w bool) {
	if !w {
		v &= 0xFF
		e.sf = v&0x80 != 0
	} else {
		e.sf = v&0x8000 != 0
	}
	e.zf = v == 0
}

// alu does op (add, or, adc, sbb, and, sub, xor, cmp) and returns the result
func (e *emu86) alu(op byte, a, b uint16, w bool) uint16 {
	mask := uint32(0xFF)
	if w {
		mask = 0xFFFF
	}
	A, B := uint32(a)&mask, uint32(b)&mask
	var r uint32
	carry := uint32(0)
	if e.cf {
		carry = 1
	}
	switch op {
	case 0:
		r = A + B
		e.cf = r > mask
	case 2:
		r = A + B + carry
		e.cf = r > mask
	case 1:
		r, e.cf = A|B, false
	case 4:
		r, e.cf = A&B, false
	case 6:
		r, e.cf = A^B, false
	case 3:
		r = A - B - carry
		e.cf = A < B+carry
	case 5, 7:
		r = A - B
		e.cf = A < B
	}
	e.setZS(uint16(r&mask), w)
	return uint16(r & mask)
}

func (e *emu86) jcc(c bool) {
	d := int8(e.fb())
	if c {
		e.ip = uint16(int(e.ip) + int(d))
	}
}

func (e *emu86) step() error {
	e.steps++
	e.seg = nil
	start := e.ip
	op := e.fb()
	for op == 0x26 || op == 0x2E || op == 0x36 || op == 0x3E {
		s := [...]*uint16{&e.es, &e.cs, &e.ss, &e.ds}[(op>>3)&3]
		e.seg = s
		op = e.fb()
	}
	switch {
	case op < 0x40 && op&7 < 6: // ALU forms
		aop, form := op>>3, op&7
		w := form&1 == 1
		switch form {
		case 0, 1:
			reg, o := e.modrm()
			r := e.alu(aop, e.get(o, w), e.get(opnd{reg: true, idx: reg}, w), w)
			if aop != 7 {
				e.set(o, w, r)
			}
		case 2, 3:
			reg, o := e.modrm()
			ro := opnd{reg: true, idx: reg}
			r := e.alu(aop, e.get(ro, w), e.get(o, w), w)
			if aop != 7 {
				e.set(ro, w, r)
			}
		case 4:
			r := e.alu(aop, e.ax&0xFF, uint16(e.fb()), false)
			if aop != 7 {
				e.s8(0, byte(r))
			}
		case 5:
			r := e.alu(aop, e.ax, e.fw(), true)
			if aop != 7 {
				e.ax = r
			}
		}
		return nil
	case op >= 0x40 && op <= 0x4F: // inc/dec r16
		r := e.r16(op & 7)
		c := e.cf
		if op < 0x48 {
			*r = e.alu(0, *r, 1, true)
		} else {
			*r = e.alu(5, *r, 1, true)
		}
		e.cf = c
		return nil
	case op >= 0x50 && op <= 0x57:
		e.push(*e.r16(op & 7))
		return nil
	case op >= 0x58 && op <= 0x5F:
		*e.r16(op & 7) = e.pop()
		return nil
	case op >= 0x70 && op <= 0x7F:
		var c bool
		switch op {
		case 0x72:
			c = e.cf
		case 0x73:
			c = !e.cf
		case 0x74:
			c = e.zf
		case 0x75:
			c = !e.zf
		case 0x76:
			c = e.cf || e.zf
		case 0x77:
			c = !e.cf && !e.zf
		case 0x78:
			c = e.sf
		case 0x79:
			c = !e.sf
		default:
			return fmt.Errorf("jcc %02X at %04X", op, start)
		}
		e.jcc(c)
		return nil
	case op >= 0xB0 && op <= 0xB7:
		e.s8(op&7, e.fb())
		return nil
	case op >= 0xB8 && op <= 0xBF:
		*e.r16(op & 7) = e.fw()
		return nil
	}
	seg := e.ds
	if e.seg != nil {
		seg = *e.seg
	}
	switch op {
	case 0x06:
		e.push(e.es)
	case 0x07:
		e.es = e.pop()
	case 0x1E:
		e.push(e.ds)
	case 0x1F:
		e.ds = e.pop()
	case 0x80, 0x81, 0x83:
		reg, o := e.modrm()
		w := op != 0x80
		var b uint16
		switch op {
		case 0x80:
			b = uint16(e.fb())
		case 0x81:
			b = e.fw()
		case 0x83:
			b = uint16(int16(int8(e.fb())))
		}
		r := e.alu(reg, e.get(o, w), b, w)
		if reg != 7 {
			e.set(o, w, r)
		}
	case 0x84, 0x85:
		reg, o := e.modrm()
		w := op == 0x85
		e.alu(4, e.get(o, w), e.get(opnd{reg: true, idx: reg}, w), w)
	case 0xA8:
		e.alu(4, e.ax&0xFF, uint16(e.fb()), false)
	case 0x86, 0x87:
		reg, o := e.modrm()
		w := op == 0x87
		ro := opnd{reg: true, idx: reg}
		a, b := e.get(o, w), e.get(ro, w)
		e.set(o, w, b)
		e.set(ro, w, a)
	case 0x88, 0x89, 0x8A, 0x8B:
		reg, o := e.modrm()
		w := op&1 == 1
		ro := opnd{reg: true, idx: reg}
		if op < 0x8A {
			e.set(o, w, e.get(ro, w))
		} else {
			e.set(ro, w, e.get(o, w))
		}
	case 0x8E:
		reg, o := e.modrm()
		*[...]*uint16{&e.es, &e.cs, &e.ss, &e.ds}[reg&3] = e.get(o, true)
	case 0xA0:
		e.s8(0, e.rb(seg, e.fw()))
	case 0xA1:
		e.ax = e.rw(seg, e.fw())
	case 0xA2:
		e.wb(seg, e.fw(), byte(e.ax))
	case 0xA3:
		e.ww(seg, e.fw(), e.ax)
	case 0xAB:
		e.ww(e.es, e.di, e.ax)
		e.di += 2
	case 0xAC:
		e.s8(0, e.rb(seg, e.si))
		e.si++
	case 0xAD:
		e.ax = e.rw(seg, e.si)
		e.si += 2
	case 0xC3:
		e.ip = e.pop()
	case 0xC6, 0xC7:
		_, o := e.modrm()
		w := op == 0xC7
		if w {
			e.set(o, w, e.fw())
		} else {
			e.set(o, w, uint16(e.fb()))
		}
	case 0xCD:
		if !e.intr(e.fb()) {
			return errStop
		}
	case 0xD0, 0xD1, 0xD2, 0xD3:
		reg, o := e.modrm()
		w := op&1 == 1
		n := 1
		if op >= 0xD2 {
			n = int(e.cx & 0xFF)
		}
		v := e.get(o, w)
		top := uint16(0x80)
		if w {
			top = 0x8000
		}
		for ; n > 0; n-- {
			switch reg {
			case 4: // shl
				e.cf = v&top != 0
				v <<= 1
			case 5: // shr
				e.cf = v&1 != 0
				v >>= 1
			case 3: // rcr
				c := e.cf
				e.cf = v&1 != 0
				v >>= 1
				if c {
					v |= top
				}
			default:
				return fmt.Errorf("shift /%d at %04X", reg, start)
			}
		}
		if !w {
			v &= 0xFF
		}
		e.set(o, w, v)
		if reg != 3 {
			e.setZS(v, w)
		}
	case 0xE2:
		d := int8(e.fb())
		e.cx--
		if e.cx != 0 {
			e.ip = uint16(int(e.ip) + int(d))
		}
	case 0xE8:
		d := e.fw()
		e.push(e.ip)
		e.ip += d
	case 0xE9:
		d := e.fw()
		e.ip += d
	case 0xEB:
		e.jcc(true)
	case 0xEC:
		e.s8(0, byte(e.in(e.dx, false)))
	case 0xED:
		e.ax = e.in(e.dx, true)
	case 0xEE:
		e.out(e.dx, e.ax&0xFF, false)
	case 0xEF:
		e.out(e.dx, e.ax, true)
	case 0xF6, 0xF7:
		reg, o := e.modrm()
		w := op == 0xF7
		switch reg {
		case 0:
			var b uint16
			if w {
				b = e.fw()
			} else {
				b = uint16(e.fb())
			}
			e.alu(4, e.get(o, w), b, w)
		case 4:
			if !w {
				return fmt.Errorf("mul8 at %04X", start)
			}
			r := uint32(e.ax) * uint32(e.get(o, true))
			e.ax, e.dx = uint16(r), uint16(r>>16)
			e.cf = e.dx != 0
		case 6:
			if !w {
				return fmt.Errorf("div8 at %04X", start)
			}
			d := uint32(e.get(o, true))
			if d == 0 {
				return fmt.Errorf("divide by zero at %04X", start)
			}
			n := uint32(e.dx)<<16 | uint32(e.ax)
			if n/d > 0xFFFF {
				return fmt.Errorf("divide overflow at %04X", start)
			}
			e.ax, e.dx = uint16(n/d), uint16(n%d)
		default:
			return fmt.Errorf("grp3 /%d at %04X", reg, start)
		}
	case 0xF8:
		e.cf = false
	case 0xF9:
		e.cf = true
	case 0xFA, 0xFB, 0xFC, 0x90:
	case 0xFE:
		reg, o := e.modrm()
		c := e.cf
		switch reg {
		case 0:
			e.set(o, false, e.alu(0, e.get(o, false), 1, false))
		case 1:
			e.set(o, false, e.alu(5, e.get(o, false), 1, false))
		default:
			return fmt.Errorf("FE /%d at %04X", reg, start)
		}
		e.cf = c
	default:
		return fmt.Errorf("opcode %02X at %04X", op, start)
	}
	return nil
}

var errStop = fmt.Errorf("stopped")

// ideDev is a device on the simulated IDE controller.
type ideDev struct {
	atapi    bool
	id       [256]uint16
	regs     [8]byte
	feed     int // next IDENTIFY word to read; -1: none
	answered bool
}

func ataIdentify(model string, c, h, s uint16, lba uint32) *ideDev {
	d := &ideDev{feed: -1}
	d.id[1], d.id[3], d.id[6] = c, h, s
	m := fmt.Sprintf("%-40s", model)
	for i := 0; i < 20; i++ {
		d.id[27+i] = uint16(m[2*i])<<8 | uint16(m[2*i+1])
	}
	if lba > 0 {
		d.id[49] = 1 << 9
		d.id[60], d.id[61] = uint16(lba), uint16(lba>>16)
	}
	return d
}

type geomRun struct {
	out  string // what DOS printed
	hl   string // the same with the inverse parts in [ ]
	exit byte
}

// runGEOMEmu runs GEOM.COM with the BIOS disks (what INT 13h reports) and
// the IDE devices [channel][master, slave].
func runGEOMEmu(t *testing.T, disks []Geometry, ide [2][2]*ideDev) geomRun {
	e := &emu86{mem: make([]byte, 1<<20)}
	e.cs, e.ds, e.es, e.ss = 0x1000, 0x1000, 0x1000, 0x1000
	e.sp, e.ip = 0xFFFE, 0x100
	copy(e.mem[e.lin(0x1000, 0x100):], geomCOM)
	e.mem[0x475] = byte(len(disks))
	var out, hl strings.Builder
	var exit byte
	inv := -1 // the character the BIOS just wrote in inverse
	inHL := false
	sel := [2]int{}
	chanOf := func(p uint16) (int, uint16, bool) {
		switch {
		case p >= 0x1F0 && p <= 0x1F7:
			return 0, p - 0x1F0, true
		case p >= 0x170 && p <= 0x177:
			return 1, p - 0x170, true
		case p == 0x3F6:
			return 0, 8, true
		case p == 0x376:
			return 1, 8, true
		}
		return 0, 0, false
	}
	e.in = func(p uint16, w bool) uint16 {
		ch, r, ok := chanOf(p)
		if !ok {
			return 0xFFFF
		}
		if ide[ch][0] == nil && ide[ch][1] == nil {
			return 0xFFFF // nothing on the cable
		}
		d := ide[ch][sel[ch]]
		if d == nil {
			return 0 // the selected device is not there
		}
		switch r {
		case 0:
			if d.feed >= 0 && d.feed < 256 {
				v := d.id[d.feed]
				d.feed++
				return v
			}
			return 0
		case 7, 8:
			switch {
			case !d.answered:
				return 0x50 // ready
			case d.atapi:
				return 0x51 // error: IDENTIFY DEVICE is not for ATAPI
			case d.feed >= 0 && d.feed < 256:
				return 0x58 // data ready
			}
			return 0x50
		}
		return uint16(d.regs[r])
	}
	e.out = func(p uint16, v uint16, w bool) {
		ch, r, ok := chanOf(p)
		if !ok || r == 8 {
			return
		}
		if r == 6 {
			sel[ch] = int(v>>4) & 1
			return
		}
		d := ide[ch][sel[ch]]
		if d == nil {
			return
		}
		if r == 7 && v == 0xEC {
			d.answered = true
			if d.atapi {
				d.regs[4], d.regs[5] = 0x14, 0xEB
			} else {
				d.feed = 0
			}
			return
		}
		d.regs[r] = byte(v)
	}
	e.intr = func(n byte) bool {
		switch n {
		case 0x13:
			i := int(byte(e.dx)) - 0x80
			if byte(e.ax>>8) != 8 || i < 0 || i >= len(disks) {
				e.cf = true
				return true
			}
			g := disks[i]
			e.cx = uint16(byte(g.C))<<8 | uint16(byte(g.S)|byte(g.C>>8)<<6)
			e.dx = uint16(byte(g.H-1))<<8 | uint16(len(disks))
			e.cf = false
		case 0x10:
			switch byte(e.ax >> 8) {
			case 0x0F:
				e.bx = e.bx&0xFF | 0<<8
			case 0x09:
				if byte(e.bx) != 0x70 || e.cx != 1 || byte(e.bx>>8) != 0 {
					t.Fatalf("INT 10h AH=09h BX=%04X CX=%d", e.bx, e.cx)
				}
				inv = int(byte(e.ax))
			}
		case 0x21:
			put := func(c byte) {
				out.WriteByte(c)
				on := inv == int(c)
				if on != inHL {
					if on {
						hl.WriteByte('[')
					} else {
						hl.WriteByte(']')
					}
					inHL = on
				}
				hl.WriteByte(c)
				inv = -1
			}
			switch byte(e.ax >> 8) {
			case 0x44:
				e.dx, e.cf = 0x83, false // the console
			case 0x09:
				for o := e.dx; e.rb(e.ds, o) != '$'; o++ {
					put(e.rb(e.ds, o))
				}
			case 0x02:
				put(byte(e.dx))
			case 0x4C:
				exit = byte(e.ax)
				return false
			}
		}
		return true
	}
	for {
		if e.steps%500 == 0 { // the BIOS timer ticks
			e.ww(0, 0x46C, e.rw(0, 0x46C)+1)
		}
		if err := e.step(); err == errStop {
			break
		} else if err != nil {
			t.Fatal(err, out.String())
		}
		if e.steps > 5000000 {
			t.Fatal("does not stop", out.String())
		}
	}
	return geomRun{strings.ReplaceAll(out.String(), "\r\n", "\n"), strings.ReplaceAll(hl.String(), "\r\n", "\n"), exit}
}

func TestGEOMDisksAndIDE(t *testing.T) {
	// a starter card (User 1024/16/63) as the first disk, the card to
	// measure as the second with Auto/LBA, an old disk on the secondary
	// channel and a CD-ROM
	cd := &ideDev{atapi: true, feed: -1}
	ide := [2][2]*ideDev{
		{ataIdentify("SanDisk SDCFB-512", 993, 16, 63, 1000944), ataIdentify("SanDisk SDCFB-1024", 1985, 16, 63, 2001888)},
		{ataIdentify("ST3144A", 1001, 15, 17, 0), cd},
	}
	r := runGEOMEmu(t, []Geometry{{1022, 16, 63}, {1013, 32, 63}}, ide)
	for _, want := range []string{
		"Disk 1: Cylinders: 1022   Heads: 16   Sectors/track: 63",
		"Disk 2: Cylinders: 1013   Heads: 32   Sectors/track: 63",
		"Enter these three numbers",
		"Primary master:   SanDisk SDCFB-512\n    Own geometry: 993/16/63, 488 MB\n    Without auto-detection, BIOS type User: 993/16/63\n",
		"Primary slave:    SanDisk SDCFB-1024\n    Own geometry: 1985/16/63, 977 MB\n    Without auto-detection, BIOS type User: 1024/16/63\n    (Without LBA: at most 1024 cylinders",
		"Secondary master: ST3144A\n    Own geometry: 1001/15/17, 124 MB\n    Without auto-detection, BIOS type User: 1001/15/17\n",
		"Secondary slave:  CD-ROM or other ATAPI device",
	} {
		if !strings.Contains(r.out, want) {
			t.Fatalf("missing %q in:\n%s", want, r.out)
		}
	}
	if r.exit != 0 {
		t.Fatal(r.exit)
	}
	for _, want := range []string{
		"Disk 2: Cylinders: [1013]   Heads: [32]   Sectors/track: [63]\n",
		"  [Primary master:]   SanDisk",
		"  [Secondary slave:]  CD-ROM",
		"BIOS type User: [1024/16/63]\n",
		"Own geometry: 1985/16/63, 977 MB\n",
	} {
		if !strings.Contains(r.hl, want) {
			t.Fatalf("missing %q in:\n%s", want, r.hl)
		}
	}

	// nothing on the IDE ports, no disk in the BIOS (an XT with XT-IDE)
	r = runGEOMEmu(t, nil, [2][2]*ideDev{})
	if !strings.Contains(r.out, "No hard disk found") || !strings.Contains(r.out, "Primary master:   -") || r.exit != 1 {
		t.Fatal(r.out, r.exit)
	}
	// a big card, over 64 GB of sectors in GB
	big := [2][2]*ideDev{{ataIdentify("BIG CARD", 16383, 16, 63, 250069680)}}
	r = runGEOMEmu(t, []Geometry{{1021, 128, 63}}, big)
	if !strings.Contains(r.out, "Own geometry: 16383/16/63, 119 GB") {
		t.Fatal(r.out)
	}
	// 256 heads: the warning, exit code 2
	r = runGEOMEmu(t, []Geometry{{967, 256, 63}}, big)
	if !strings.Contains(r.out, "WARNING: 256 heads") || strings.Contains(r.out, "Enter these") || r.exit != 2 {
		t.Fatal(r.out)
	}
}
