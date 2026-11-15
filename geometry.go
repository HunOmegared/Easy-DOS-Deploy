package main

// BIOS translation modes as offered in BIOS setup programs.
const (
	ModeNormal = 0 // CHS as the drive reports it (max. 1024 cylinders, about 504 MB)
	ModeLBA    = 1 // LBA assisted translation
	ModeLarge  = 2 // "Large" / ECHS: heads doubled, cylinders halved
)

func TranslationModes() []string {
	return []string{T("NORMAL (CHS, LBA off)"), T("LBA (LBA mode on)"), T("LARGE / ECHS")}
}

// DOSGeometry works out the geometry to give this program from the values
// shown in BIOS setup (the drive's own C/H/S) and the translation mode.
//
// The BIOS gives DOS one cylinder less than its translated geometry (the
// last one is kept for diagnostics), and DOS FDISK does not use the last
// cylinder the BIOS reports either; that is the number MSD shows. Example:
// a 1 GB CF card shown as 2030/16/63 with LBA on becomes 1015/32/63 in the
// BIOS, and DOS partitions 1013/32/63. The same card with LBA off: 1024
// cylinders at most, so 1022/16/63.
func DOSGeometry(setup Geometry, mode int) Geometry {
	total := setup.Sectors()
	g := setup
	switch mode {
	case ModeLarge:
		for g.C > 1024 && g.H*2 <= 255 {
			g.C /= 2
			g.H *= 2
		}
		if g.H == 256 {
			g.H = 255
		}
	case ModeLBA:
		g.S = 63
		mib := total / 2048
		switch {
		case mib <= 504:
			g.H = 16
		case mib <= 1008:
			g.H = 32
		case mib <= 2016:
			g.H = 64
		case mib <= 4032:
			g.H = 128
		default:
			g.H = 255
		}
		g.C = uint32(total / int64(g.H*g.S))
	}
	if g.C > 1024 {
		g.C = 1024
	}
	if g.C > 2 {
		g.C -= 2
	}
	return g
}
