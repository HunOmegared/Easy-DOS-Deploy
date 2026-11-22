package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// biosDisk serves INT 13h for the emulator from a disk image.
type biosDisk struct {
	img       []byte
	g         Geometry
	failReads int
	reads     int
	startDL   *byte // DL when the BIOS jumps to the MBR; nil: 80h
}

func (d *biosDisk) int13(c *cpu8086, out *strings.Builder) error {
	ah := c.get8(4)
	switch ah {
	case 0x00:
		c.cf = false
	case 0x02:
		if c.get8(2) != 0x80 { // a drive that is not there
			c.cf = true
			c.set8(4, 0x01)
			return nil
		}
		d.reads++
		if d.failReads > 0 {
			d.failReads--
			c.cf = true
			c.set8(4, 0x04)
			return nil
		}
		n := int(c.get8(0))
		cyl := uint32(c.get8(5)) | uint32(c.get8(1)&0xC0)<<2
		sec := uint32(c.get8(1) & 0x3F)
		head := uint32(c.get8(6))
		lba := (cyl*d.g.H+head)*d.g.S + sec - 1
		dst := c.lin(c.seg[sES], c.r[rBX])
		copy(c.mem[dst:dst+n*512], d.img[int(lba)*512:int(lba)*512+n*512])
		c.cf = false
		c.set8(4, 0)
	case 0x08:
		maxCyl := d.g.C // what a BIOS reports: last usable cylinder number
		c.set8(5, byte(maxCyl))
		c.set8(1, byte(d.g.S)|byte(maxCyl>>8)<<6)
		c.set8(6, byte(d.g.H-1))
		c.set8(2, 1)
		c.cf = false
	default:
		return nil
	}
	return nil
}

// bootMBR loads sector 0 at 0000:7C00 and runs it until it jumps to the
// loaded boot sector (second arrival at 7C00) or halts.
func bootMBR(t *testing.T, d *biosDisk) (*cpu8086, string) {
	c := newCPU()
	copy(c.mem[0x7C00:], d.img[:512])
	c.r[rDX] = 0x80
	if d.startDL != nil {
		c.r[rDX] = uint16(*d.startDL) // a BIOS that does not pass the drive in DL
	}
	c.seg[sCS] = 0
	c.ip = 0x7C00
	var out strings.Builder
	c.ints = func(c *cpu8086, n byte) error {
		switch n {
		case 0x13:
			return d.int13(c, &out)
		case 0x10:
			out.WriteByte(c.get8(0))
		}
		return nil
	}
	arrivals := 0
	err := c.run(100000, func(c *cpu8086) bool {
		if c.seg[sCS] == 0 && c.ip == 0x7C00 {
			arrivals++
			return arrivals == 2
		}
		return false
	})
	if err != nil {
		t.Fatal(err, out.String())
	}
	return c, out.String()
}

func TestOwnMBRBoots(t *testing.T) {
	g := Geometry{1013, 32, 63} // the Integra card: the boot sector is at C0 H1 S1
	p := filepath.Join(t.TempDir(), "c.img")
	if _, err := CreateBootImage(BootOptions{Path: p, Kind: MediaHDD, Geom: Geometry{40, 32, 63}}); err != nil {
		t.Fatal(err)
	}
	img, _ := os.ReadFile(p)
	d := &biosDisk{img: img, g: g}
	c, out := bootMBR(t, d)
	if c.halted || out != "" {
		t.Fatalf("did not boot: %q", out)
	}
	vbr := img[63*512 : 64*512]
	if string(c.mem[0x7C00:0x7E00]) != string(vbr) {
		t.Fatal("boot sector not loaded")
	}
	if c.get8(2) != 0x80 || c.r[rSI] != 0x07BE {
		t.Fatalf("DL=%02X SI=%04X", c.get8(2), c.r[rSI])
	}

	// some old BIOSes do not pass the boot drive in DL: the drive comes from
	// the partition table (80h), as in the MS-DOS MBR (Integra, 2026-11-15)
	for _, dl := range []byte{0x00, 0x01, 0xFF} {
		dl := dl
		d = &biosDisk{img: img, g: g, startDL: &dl}
		if c, out := bootMBR(t, d); c.halted || out != "" || c.get8(2) != 0x80 {
			t.Fatalf("DL=%02X at start: %q, DL at the boot sector %02X", dl, out, c.get8(2))
		}
	}

	// read errors are retried
	d = &biosDisk{img: img, g: g, failReads: 3}
	if c, out := bootMBR(t, d); c.halted || out != "" || d.reads != 4 {
		t.Fatalf("retry: %q reads=%d", out, d.reads)
	}
	d = &biosDisk{img: img, g: g, failReads: 99}
	if c, out := bootMBR(t, d); !c.halted || !strings.Contains(out, "Disk read error") {
		t.Fatalf("read error: %q", out)
	}
	// no active partition
	noact := append([]byte(nil), img...)
	noact[446] = 0
	if c, out := bootMBR(t, &biosDisk{img: noact, g: g}); !c.halted || !strings.Contains(out, "No active partition") {
		t.Fatalf("no active: %q", out)
	}
	// boot sector without signature
	nosig := append([]byte(nil), img...)
	nosig[63*512+510] = 0
	if c, out := bootMBR(t, &biosDisk{img: nosig, g: g}); !c.halted || !strings.Contains(out, "Missing operating system") {
		t.Fatalf("no signature: %q", out)
	}
	// the second partition entry is the active one
	second := append([]byte(nil), img...)
	copy(second[462:478], second[446:462])
	for i := 446; i < 462; i++ {
		second[i] = 0
	}
	if c, out := bootMBR(t, &biosDisk{img: second, g: g}); c.halted || out != "" || c.r[rSI] != 0x07CE {
		t.Fatalf("second entry: %q SI=%04X", out, c.r[rSI])
	}
}

func runGEOM(t *testing.T, g Geometry, noDisk bool) string {
	c := newCPU()
	for i := range c.seg {
		c.seg[i] = 0x1000
	}
	copy(c.mem[c.lin(0x1000, 0x100):], geomCOM)
	c.ip = 0x100
	c.r[rSP] = 0xFFFE
	var out strings.Builder
	d := &biosDisk{g: g}
	exited := false
	c.ints = func(c *cpu8086, n byte) error {
		switch n {
		case 0x13:
			if noDisk {
				c.cf = true
				return nil
			}
			return d.int13(c, &out)
		case 0x21:
			switch c.get8(4) {
			case 0x09:
				for o := c.r[rDX]; c.rd8(c.seg[sDS], o) != '$'; o++ {
					out.WriteByte(c.rd8(c.seg[sDS], o))
				}
			case 0x02:
				out.WriteByte(c.get8(2))
			case 0x4C:
				exited = true
			}
		}
		return nil
	}
	if err := c.run(100000, func(*cpu8086) bool { return exited }); err != nil {
		t.Fatal(err, out.String())
	}
	return out.String()
}

func TestGEOMCOM(t *testing.T) {
	out := runGEOM(t, Geometry{1013, 32, 63}, false)
	if !strings.Contains(out, "Cylinders: 1013   Heads: 32   Sectors/track: 63") {
		t.Fatal(out)
	}
	out = runGEOM(t, Geometry{1022, 16, 63}, false)
	if !strings.Contains(out, "Cylinders: 1022   Heads: 16   Sectors/track: 63") {
		t.Fatal(out)
	}
	out = runGEOM(t, Geometry{615, 4, 17}, false) // an old 20 MB disk
	if !strings.Contains(out, "Cylinders: 615   Heads: 4   Sectors/track: 17") {
		t.Fatal(out)
	}
	if out := runGEOM(t, Geometry{}, true); !strings.Contains(out, "No hard disk found") {
		t.Fatal(out)
	}
	// the 8 GB Lexar card on the Integra with Auto: the BIOS gives 256 heads
	out = runGEOM(t, Geometry{967, 256, 63}, false)
	if !strings.Contains(out, "Cylinders: 967   Heads: 256   Sectors/track: 63") || !strings.Contains(out, "WARNING: 256 heads") || strings.Contains(out, "Enter these") {
		t.Fatal(out)
	}
	if out = runGEOM(t, Geometry{510, 128, 63}, false); strings.Contains(out, "WARNING") || !strings.Contains(out, "Enter these") {
		t.Fatal(out)
	}
}

func TestDOSGeometry(t *testing.T) {
	card := Geometry{2030, 16, 63} // 1 GB CF card in the Integra's BIOS setup
	if g := DOSGeometry(card, ModeLBA); g != (Geometry{1013, 32, 63}) {
		t.Fatalf("LBA: %v", g) // what MSD and FDISK showed on the real machine
	}
	if g := DOSGeometry(card, ModeNormal); g != (Geometry{1022, 16, 63}) {
		t.Fatalf("NORMAL: %v", g) // what FDISK made on a card partitioned with LBA off
	}
	if g := DOSGeometry(card, ModeLarge); g != (Geometry{1013, 32, 63}) {
		t.Fatalf("LARGE: %v", g)
	}
	if g := DOSGeometry(Geometry{615, 4, 17}, ModeNormal); g != (Geometry{613, 4, 17}) {
		t.Fatalf("small: %v", g)
	}
	// the 8 GB Lexar card on the Integra, measured with GEOM.COM
	lexar := Geometry{15506, 16, 63}
	if g := DOSGeometry(lexar, ModeLBA); g != (Geometry{967, 256, 63}) {
		t.Fatalf("Lexar, Auto: %v", g)
	}
	if g := DOSGeometry(Geometry{4096, 16, 63}, ModeLBA); g != (Geometry{510, 128, 63}) {
		t.Fatalf("Lexar, User 4096: %v", g) // MSD: 510/128/63, 2007 MB
	}
	// later BIOSes: LBA-assist, 255 heads
	if g := DOSGeometry(lexar, ModeAssist); g != (Geometry{970, 255, 63}) {
		t.Fatalf("Lexar, LBA-assist: %v", g)
	}
	if g := DOSGeometry(card, ModeAssist); g != (Geometry{1013, 32, 63}) {
		t.Fatalf("1 GB, LBA-assist: %v", g)
	}
	// 256 heads: a User type with fewer cylinders, the most that DOS can use
	c, g, ok := UserCylinders(lexar, ModeLBA)
	if !ok || c != 8191 || g != (Geometry{1021, 128, 63}) {
		t.Fatal(c, g, ok)
	}
	if _, err := PlanHDD(g); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanHDD(Geometry{967, 256, 63}); err == nil || !strings.Contains(err.Error(), "256") {
		t.Fatal(err)
	}
	if c, _, _ := UserCylinders(Geometry{4096, 16, 63}, ModeLBA); c != 4096 {
		t.Fatal(c) // already fine
	}
}

func TestBlankImage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "blank.img")
	r, err := CreateBootImage(BootOptions{Path: p, Kind: MediaHDD, Geom: Geometry{1013, 32, 63}, Total: 2046240, Blank: true})
	if err != nil || len(r.Warnings) == 0 {
		t.Fatal(err, r)
	}
	st, _ := os.Stat(p)
	if st.Size() != 2046240*512 {
		t.Fatal(st.Size())
	}
}
