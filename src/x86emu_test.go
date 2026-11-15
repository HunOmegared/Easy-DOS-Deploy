package main

import (
	"encoding/binary"
	"fmt"
)

// A deliberately small 8086 interpreter: just the instructions used by the
// program's own 16-bit code (MBR and GEOM.COM), so the tests can run them.
// Unknown opcodes stop it with an error rather than guess.

type cpu8086 struct {
	mem    []byte
	r      [8]uint16 // AX CX DX BX SP BP SI DI
	seg    [4]uint16 // ES CS SS DS
	ip     uint16
	zf, cf bool
	halted bool
	ints   func(c *cpu8086, n byte) error
	steps  int
}

const (
	rAX = iota
	rCX
	rDX
	rBX
	rSP
	rBP
	rSI
	rDI
)
const (
	sES = iota
	sCS
	sSS
	sDS
)

func newCPU() *cpu8086 { return &cpu8086{mem: make([]byte, 1<<20)} }

func (c *cpu8086) lin(seg, off uint16) int { return (int(seg)<<4 + int(off)) & 0xFFFFF }
func (c *cpu8086) rd8(seg, off uint16) byte {
	return c.mem[c.lin(seg, off)]
}
func (c *cpu8086) rd16(seg, off uint16) uint16 {
	return uint16(c.rd8(seg, off)) | uint16(c.rd8(seg, off+1))<<8
}
func (c *cpu8086) wr8(seg, off uint16, v byte) { c.mem[c.lin(seg, off)] = v }
func (c *cpu8086) wr16(seg, off uint16, v uint16) {
	c.wr8(seg, off, byte(v))
	c.wr8(seg, off+1, byte(v>>8))
}
func (c *cpu8086) fetch() byte {
	b := c.rd8(c.seg[sCS], c.ip)
	c.ip++
	return b
}
func (c *cpu8086) fetch16() uint16 {
	lo := uint16(c.fetch())
	return lo | uint16(c.fetch())<<8
}

// 8-bit register access: AL CL DL BL AH CH DH BH
func (c *cpu8086) get8(i byte) byte {
	if i < 4 {
		return byte(c.r[i])
	}
	return byte(c.r[i-4] >> 8)
}
func (c *cpu8086) set8(i byte, v byte) {
	if i < 4 {
		c.r[i] = c.r[i]&0xFF00 | uint16(v)
	} else {
		c.r[i-4] = c.r[i-4]&0x00FF | uint16(v)<<8
	}
}
func (c *cpu8086) push(v uint16) {
	c.r[rSP] -= 2
	c.wr16(c.seg[sSS], c.r[rSP], v)
}
func (c *cpu8086) pop() uint16 {
	v := c.rd16(c.seg[sSS], c.r[rSP])
	c.r[rSP] += 2
	return v
}

// modrm returns mod, reg, rm and, for memory operands, the offset (DS).
func (c *cpu8086) modrm() (mod, reg, rm byte, off uint16, err error) {
	b := c.fetch()
	mod, reg, rm = b>>6, (b>>3)&7, b&7
	if mod == 3 {
		return
	}
	switch {
	case mod == 0 && rm == 6:
		off = c.fetch16()
		return
	case rm == 4:
		off = c.r[rSI]
	case rm == 5:
		off = c.r[rDI]
	case rm == 7:
		off = c.r[rBX]
	default:
		err = fmt.Errorf("addressing mode %02X not supported", b)
		return
	}
	switch mod {
	case 1:
		off += uint16(int16(int8(c.fetch())))
	case 2:
		off += c.fetch16()
	}
	return
}

func (c *cpu8086) flags16(v uint16) { c.zf = v == 0 }

func (c *cpu8086) jcc(cond bool) {
	d := int8(c.fetch())
	if cond {
		c.ip = uint16(int(c.ip) + int(d))
	}
}

func (c *cpu8086) step() error {
	c.steps++
	op := c.fetch()
	switch {
	case op >= 0xB8 && op <= 0xBF: // mov r16,imm16
		c.r[op-0xB8] = c.fetch16()
		return nil
	case op >= 0xB0 && op <= 0xB7: // mov r8,imm8
		c.set8(op-0xB0, c.fetch())
		return nil
	case op >= 0x50 && op <= 0x57:
		c.push(c.r[op-0x50])
		return nil
	case op >= 0x58 && op <= 0x5F:
		c.r[op-0x58] = c.pop()
		return nil
	case op >= 0x40 && op <= 0x47:
		c.r[op-0x40]++
		c.flags16(c.r[op-0x40])
		return nil
	case op >= 0x48 && op <= 0x4F:
		c.r[op-0x48]--
		c.flags16(c.r[op-0x48])
		return nil
	}
	switch op {
	case 0xFA, 0xFB, 0xFC, 0x90: // cli sti cld nop
	case 0xF4:
		c.halted = true
	case 0x31, 0x09, 0x30, 0x08, 0x88, 0x89, 0x8B, 0x8A: // r/m forms
		mod, reg, rm, off, err := c.modrm()
		if err != nil {
			return err
		}
		word := op == 0x31 || op == 0x09 || op == 0x8B
		var src uint16
		if mod == 3 {
			if word {
				src = c.r[rm]
			} else {
				src = uint16(c.get8(rm))
			}
		} else if word {
			src = c.rd16(c.seg[sDS], off)
		} else {
			src = uint16(c.rd8(c.seg[sDS], off))
		}
		switch op {
		case 0x31: // xor r/m16,r16
			v := c.r[rm] ^ c.r[reg]
			c.r[rm], c.cf = v, false
			c.flags16(v)
		case 0x09: // or r/m16,r16
			v := c.r[rm] | c.r[reg]
			c.r[rm], c.cf = v, false
			c.flags16(v)
		case 0x30: // xor r/m8,r8
			v := c.get8(rm) ^ c.get8(reg)
			c.set8(rm, v)
			c.zf, c.cf = v == 0, false
		case 0x08: // or r/m8,r8
			v := c.get8(rm) | c.get8(reg)
			c.set8(rm, v)
			c.zf, c.cf = v == 0, false
		case 0x88: // mov r/m8,r8
			if mod != 3 {
				return fmt.Errorf("mov to memory not supported")
			}
			c.set8(rm, c.get8(reg))
		case 0x89: // mov r/m16,r16
			if mod != 3 {
				return fmt.Errorf("mov to memory not supported")
			}
			c.r[rm] = c.r[reg]
		case 0x8A: // mov r8,r/m8
			c.set8(reg, byte(src))
		case 0x8B: // mov r16,r/m16
			c.r[reg] = src
		}
	case 0x8E: // mov sreg,r16
		mod, reg, rm, _, _ := c.modrm()
		if mod != 3 {
			return fmt.Errorf("mov sreg from memory not supported")
		}
		c.seg[reg] = c.r[rm]
	case 0xF3: // rep movsw
		if c.fetch() != 0xA5 {
			return fmt.Errorf("only rep movsw is supported")
		}
		for c.r[rCX] > 0 {
			c.wr16(c.seg[sES], c.r[rDI], c.rd16(c.seg[sDS], c.r[rSI]))
			c.r[rSI] += 2
			c.r[rDI] += 2
			c.r[rCX]--
		}
	case 0xAC: // lodsb
		c.set8(0, c.rd8(c.seg[sDS], c.r[rSI]))
		c.r[rSI]++
	case 0xEA: // jmp far
		off := c.fetch16()
		seg := c.fetch16()
		c.ip, c.seg[sCS] = off, seg
	case 0xEB:
		c.jcc(true)
	case 0x74:
		c.jcc(c.zf)
	case 0x75:
		c.jcc(!c.zf)
	case 0x72:
		c.jcc(c.cf)
	case 0x73:
		c.jcc(!c.cf)
	case 0xE2: // loop
		c.r[rCX]--
		c.jcc(c.r[rCX] != 0)
	case 0xE8: // call rel16
		d := c.fetch16()
		c.push(c.ip)
		c.ip += d
	case 0xC3:
		c.ip = c.pop()
	case 0x80: // grp1 r/m8,imm8
		mod, reg, rm, off, err := c.modrm()
		if err != nil {
			return err
		}
		imm := c.fetch()
		var v byte
		if mod == 3 {
			v = c.get8(rm)
		} else {
			v = c.rd8(c.seg[sDS], off)
		}
		switch reg {
		case 0: // add
			r := v + imm
			c.cf = int(v)+int(imm) > 0xFF
			c.zf = r == 0
			if mod != 3 {
				return fmt.Errorf("add to memory not supported")
			}
			c.set8(rm, r)
		case 7: // cmp
			c.zf, c.cf = v == imm, v < imm
		default:
			return fmt.Errorf("grp1 /%d not supported", reg)
		}
	case 0x81, 0x83: // grp1 r/m16,imm
		mod, reg, rm, off, err := c.modrm()
		if err != nil {
			return err
		}
		var imm uint16
		if op == 0x81 {
			imm = c.fetch16()
		} else {
			imm = uint16(int16(int8(c.fetch())))
		}
		var v uint16
		if mod == 3 {
			v = c.r[rm]
		} else {
			v = c.rd16(c.seg[sDS], off)
		}
		switch reg {
		case 0:
			if mod != 3 {
				return fmt.Errorf("add to memory not supported")
			}
			c.cf = int(v)+int(imm) > 0xFFFF
			c.r[rm] = v + imm
			c.flags16(c.r[rm])
		case 7:
			c.zf, c.cf = v == imm, v < imm
		default:
			return fmt.Errorf("grp1 /%d not supported", reg)
		}
	case 0x25: // and ax,imm16
		c.r[rAX] &= c.fetch16()
		c.flags16(c.r[rAX])
		c.cf = false
	case 0xD2: // shr r/m8,cl
		mod, reg, rm, _, _ := c.modrm()
		if mod != 3 || reg != 5 {
			return fmt.Errorf("only shr r8,cl is supported")
		}
		c.set8(rm, c.get8(rm)>>(c.get8(1)&31))
	case 0xF7: // grp3: div r16
		mod, reg, rm, _, _ := c.modrm()
		if mod != 3 || reg != 6 {
			return fmt.Errorf("only div r16 is supported")
		}
		d := uint32(c.r[rm])
		if d == 0 {
			return fmt.Errorf("divide by zero")
		}
		n := uint32(c.r[rDX])<<16 | uint32(c.r[rAX])
		c.r[rAX], c.r[rDX] = uint16(n/d), uint16(n%d)
	case 0xCD:
		n := c.fetch()
		if c.ints == nil {
			return fmt.Errorf("int %02Xh", n)
		}
		return c.ints(c, n)
	default:
		return fmt.Errorf("opcode %02X at %04X:%04X not supported", op, c.seg[sCS], c.ip-1)
	}
	return nil
}

// run executes until stop() says so, the CPU halts, or the step limit.
func (c *cpu8086) run(limit int, stop func(c *cpu8086) bool) error {
	for i := 0; i < limit; i++ {
		if c.halted || (stop != nil && stop(c)) {
			return nil
		}
		if err := c.step(); err != nil {
			return err
		}
	}
	return fmt.Errorf("step limit reached at %04X:%04X", c.seg[sCS], c.ip)
}

var _ = binary.LittleEndian
