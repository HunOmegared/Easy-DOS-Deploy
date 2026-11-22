package main

// BIOS translation modes as offered in BIOS setup programs.
const (
	ModeNormal = 0 // CHS as the drive reports it (max. 1024 cylinders, about 504 MB)
	ModeLBA    = 1 // LBA, heads doubled until the cylinders fit (early BIOSes)
	ModeAssist = 2 // LBA-assisted translation, up to 255 heads (later BIOSes)
	ModeLarge  = 3 // "Large" / ECHS: heads doubled, cylinders halved
)

func TranslationModes() []string {
	return []string{
		T("NORMAL (CHS, LBA off)"),
		T("LBA, heads doubled (e.g. Phoenix 1995)"),
		T("LBA-assisted, up to 255 heads (most BIOSes from 1996)"),
		T("LARGE / ECHS"),
	}
}

// maxFAT16 is the most sectors one MS-DOS 6.x FAT16 partition may have
// (2 GB with 32 KB clusters, a little room left).
const maxFAT16 = 4192000

// DOSGeometry works out the geometry to give this program from the values
// shown in BIOS setup (the drive's own C/H/S) and the translation mode.
//
// The BIOS keeps the last cylinder of its translated geometry, and DOS FDISK
// does not use the last one it reports either: DOS gets two cylinders fewer.
// That is the number MSD and GEOM.COM show. Measured on an Amstrad Integra
// 486 (Phoenix BIOS 1995, LBA on), which doubles the heads and halves the
// cylinders until fewer than 1024 cylinders are left:
//
//	2030/16/63 (1 GB CF card)    -> 1015/32/63  -> DOS 1013/32/63
//	15506/16/63 (8 GB CF card)   -> 969/256/63  -> DOS 967/256/63
//	4096/16/63 (the same, User)  -> 512/128/63  -> DOS 510/128/63
//
// With LBA off the 1 GB card is cut to 1024 cylinders: DOS 1022/16/63.
func DOSGeometry(setup Geometry, mode int) Geometry {
	total := setup.Sectors()
	g := setup
	switch mode {
	case ModeLBA:
		for g.C > 1023 && g.H*2 <= 256 {
			g.C /= 2
			g.H *= 2
		}
	case ModeLarge:
		for g.C > 1024 && g.H*2 <= 255 {
			g.C /= 2
			g.H *= 2
		}
		if g.H == 256 {
			g.H = 255
		}
	case ModeAssist:
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

// UserCylinders is for a BIOS that would give DOS 256 heads, which MS-DOS 6.x
// and Windows 9x cannot use: the most cylinders to type in BIOS setup (drive
// type User, heads and sectors unchanged) so that DOS gets at most 255 heads.
// Example: the 8 GB card above with LBA (heads doubled), 8191 instead of
// 15506 cylinders: DOS 1021/128/63, 4.2 GB (C: and D:).
func UserCylinders(setup Geometry, mode int) (uint32, Geometry, bool) {
	for c := setup.C; c > 0; c-- {
		if g := DOSGeometry(Geometry{c, setup.H, setup.S}, mode); g.H <= 255 {
			return c, g, true
		}
	}
	return 0, Geometry{}, false
}

// UniversalGeom is the geometry of a starter card: plain CHS that every
// BIOS with IDE support can take as a User type (16 heads, 63 sectors).
// The BIOS is set to 1024 cylinders; the image uses fewer, because some
// BIOSes give DOS a cylinder or two less than they are set to.
var UniversalGeom = Geometry{1000, 16, 63}

// biosCylLimit: some BIOSes of the early 1990s do not handle more than 4096
// cylinders in setup (the 2 GB barrier).
const biosCylLimit = 4096
