package main

// Template is a ready-made starting project. Name and Desc are English
// texts; show them with T().
type Template struct {
	Name string
	Desc string
	Make func() *Project
}

func lines(s ...string) []string { return s }

// N marks a text for translation without translating it now.
func N(s string) string { return s }

var msdosStartConfig = lines(
	`DEVICE=C:\DOS\HIMEM.SYS`,
	`DOS=HIGH,UMB`,
	`FILES=30`,
	`BUFFERS=20`,
	`LASTDRIVE=Z`,
)

var msdosStartAutoexec = lines(
	`@ECHO OFF`,
	`PROMPT $P$G`,
	`PATH C:\DOS;C:\DRIVERS`,
	`SET TEMP=C:\DOS`,
)

var freedosStartConfig = lines(
	`!FILES=40`,
	`!BUFFERS=20`,
	`!LASTDRIVE=Z`,
	`DOS=HIGH,UMB`,
	`DEVICE=C:\FREEDOS\BIN\JEMMEX.EXE NOEMS X=TEST I=TEST NOVME NOINVLPG`,
)

var freedosEndConfig = lines(
	`SHELL=C:\FREEDOS\BIN\COMMAND.COM C:\FREEDOS\BIN /E:1024 /P=C:\FDAUTO.BAT`,
)

var freedosStartAutoexec = lines(
	`@ECHO OFF`,
	`SET DOSDIR=C:\FREEDOS`,
	`SET PATH=%DOSDIR%\BIN;C:\DRIVERS`,
	`SET TEMP=%DOSDIR%\TEMP`,
	`SET TMP=%TEMP%`,
	`PROMPT $P$G`,
)

// The packet driver line is for the NE2000 that 86Box/PCem emulate by
// default (I/O 0x300, IRQ 3); edit it for real cards.
var netAutoexec = lines(
	`SET MTCPCFG=C:\MTCP\MTCP.CFG`,
	`LH C:\DRIVERS\NE2000.COM 0x60 3 0x300`,
	`C:\MTCP\DHCP.EXE`,
	`C:\EC\EC.EXE`,
)

var Templates = []Template{
	{
		Name: N("Easy Commander with network + DOS only (MS-DOS 6.x)"),
		Desc: N("Two items: network (NE2000 packet driver, mTCP DHCP, Easy Commander) or plain DOS."),
		Make: func() *Project {
			return &Project{
				Target: TargetMSDOS, Timeout: 5,
				Start:   Item{Config: msdosStartConfig, Autoexec: msdosStartAutoexec},
				Items:   []Item{{ID: "NET", Title: T("Easy Commander (with network)"), Autoexec: netAutoexec}, {ID: "PLAIN", Title: T("DOS only")}},
				Drivers: []string{"ne2000", "mtcp", "mtcpcfg", "ec"},
			}
		},
	},
	{
		Name: N("Easy Commander with network + DOS only (FreeDOS)"),
		Desc: N("The same for FreeDOS (FDCONFIG.SYS, FDAUTO.BAT, JEMMEX)."),
		Make: func() *Project {
			return &Project{
				Target: TargetFreeDOS, Timeout: 5, AutoexecName: "FDAUTO.BAT",
				Start:   Item{Config: freedosStartConfig, Autoexec: freedosStartAutoexec},
				End:     Item{Config: freedosEndConfig},
				Items:   []Item{{ID: "NET", Title: T("Easy Commander (with network)"), Autoexec: netAutoexec}, {ID: "PLAIN", Title: T("DOS only")}},
				Drivers: []string{"ne2000", "mtcp", "mtcpcfg", "ec"},
			}
		},
	},
	{
		Name: N("Games: EMS / no EMS / clean (MS-DOS 6.x)"),
		Desc: N("The classic three items: EMM386 with EMS, EMM386 with UMBs only, or no memory manager. With mouse."),
		Make: func() *Project {
			mouse := lines(`LH C:\DRIVERS\CTMOUSE.EXE`)
			return &Project{
				Target: TargetMSDOS, Timeout: 10, Default: 1,
				Start: Item{Config: lines(`DEVICE=C:\DOS\HIMEM.SYS`, `DOS=HIGH,UMB`, `FILES=30`, `BUFFERS=20`), Autoexec: msdosStartAutoexec},
				Items: []Item{
					{ID: "EMS", Title: T("Games with EMS"), Config: lines(`DEVICE=C:\DOS\EMM386.EXE RAM`), Autoexec: mouse},
					{ID: "NOEMS", Title: T("Games without EMS (more conventional memory)"), Config: lines(`DEVICE=C:\DOS\EMM386.EXE NOEMS`), Autoexec: mouse},
					{ID: "CLEAN", Title: T("Clean boot (no EMM386, no mouse)")},
				},
				Drivers: []string{"ctmouse"},
			}
		},
	},
	{
		Name: N("With / without CD-ROM (MS-DOS 6.x)"),
		Desc: N("UDVD2 CD drive driver and the MSCDEX that comes with DOS."),
		Make: func() *Project {
			return &Project{
				Target: TargetMSDOS, Timeout: 5,
				Start: Item{Config: msdosStartConfig, Autoexec: msdosStartAutoexec},
				Items: []Item{
					{ID: "CDROM", Title: T("With CD-ROM"), Config: lines(`DEVICEHIGH=C:\DRIVERS\UDVD2.SYS /D:CDROM001`), Autoexec: lines(`LH C:\DOS\MSCDEX.EXE /D:CDROM001 /L:D`)},
					{ID: "NOCD", Title: T("Without CD-ROM")},
				},
				Drivers: []string{"udvd2"},
			}
		},
	},
	{
		Name: N("With / without CD-ROM (FreeDOS)"),
		Desc: N("UDVD2 and SHSUCDX from the FreeDOS folders."),
		Make: func() *Project {
			return &Project{
				Target: TargetFreeDOS, Timeout: 5, AutoexecName: "FDAUTO.BAT",
				Start: Item{Config: freedosStartConfig, Autoexec: freedosStartAutoexec},
				End:   Item{Config: freedosEndConfig},
				Items: []Item{
					{ID: "CDROM", Title: T("With CD-ROM"), Config: lines(`DEVICE=C:\FREEDOS\BIN\UDVD2.SYS /D:CDROM001`), Autoexec: lines(`LH C:\FREEDOS\BIN\SHSUCDX.COM /D:CDROM001`)},
					{ID: "NOCD", Title: T("Without CD-ROM")},
				},
			}
		},
	},
	{
		Name: N("Empty (MS-DOS 6.x)"),
		Desc: N("A single empty menu item."),
		Make: func() *Project {
			return &Project{Target: TargetMSDOS, Timeout: 5, Start: Item{Autoexec: lines(`@ECHO OFF`)}, Items: []Item{{ID: "DOS", Title: "DOS"}}}
		},
	},
	{
		Name: N("Empty (FreeDOS)"),
		Desc: N("A single empty menu item."),
		Make: func() *Project {
			return &Project{Target: TargetFreeDOS, Timeout: 5, Start: Item{Autoexec: lines(`@ECHO OFF`)}, Items: []Item{{ID: "DOS", Title: "DOS"}}}
		},
	},
}
