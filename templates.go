package main

// Template is a ready-made starting project. Name and Desc are English
// texts; show them with T().
type Template struct {
	ID   string
	Name string
	Desc string
	Make func() *Project
}

func lines(s ...string) []string { return s }

// N marks a text for translation without translating it now.
func N(s string) string { return s }

func withDefault(p *Project, id string) *Project {
	for i, it := range p.Items {
		if it.ID == id {
			p.Default = i
		}
	}
	return p
}

var Templates = []Template{
	{"plain-ms", N("Plain DOS (MS-DOS 6.x)"), N("One menu item: DOS with HIMEM and upper memory, nothing else."),
		func() *Project { return DefaultProject(TargetMSDOS) }},
	{"plain-fd", N("Plain DOS (FreeDOS)"), N("One menu item: DOS with HIMEM and upper memory, nothing else."),
		func() *Project { return DefaultProject(TargetFreeDOS) }},
	{"everyday-ms", N("Everyday: DOS / + mouse and CD-ROM / everything (MS-DOS 6.x)"), N("Three items, from plain DOS to mouse + CD-ROM + sound."),
		func() *Project { return ProjectFromPresets(TargetMSDOS, 5, "DOS", "CDROM", "FULL") }},
	{"everyday-fd", N("Everyday: DOS / + mouse and CD-ROM / everything (FreeDOS)"), N("Three items, from plain DOS to mouse + CD-ROM + sound."),
		func() *Project { return ProjectFromPresets(TargetFreeDOS, 5, "DOS", "CDROM", "FULL") }},
	{"games-ms", N("Games: everything / EMS / no EMS / clean (MS-DOS 6.x)"), N("The classic games menu, mouse and sound in every game item."),
		func() *Project {
			return withDefault(ProjectFromPresets(TargetMSDOS, 10, "FULL", "GAMESEMS", "GAMES", "CLEAN"), "GAMES")
		}},
	{"games-fd", N("Games: everything / EMS / no EMS / clean (FreeDOS)"), N("The classic games menu, mouse and sound in every game item."),
		func() *Project {
			return withDefault(ProjectFromPresets(TargetFreeDOS, 10, "FULL", "GAMESEMS", "GAMES", "CLEAN"), "GAMES")
		}},
	{"net-ms", N("Network with Easy Commander / plain DOS (MS-DOS 6.x)"), N("NE2000 packet driver, mTCP DHCP and Easy Commander, or plain DOS."),
		func() *Project { return ProjectFromPresets(TargetMSDOS, 5, "NET", "DOS") }},
	{"net-fd", N("Network with Easy Commander / plain DOS (FreeDOS)"), N("NE2000 packet driver, mTCP DHCP and Easy Commander, or plain DOS."),
		func() *Project { return ProjectFromPresets(TargetFreeDOS, 5, "NET", "DOS") }},
	{"empty-ms", N("Empty (MS-DOS 6.x)"), N("A single empty menu item, for writing everything by hand."),
		func() *Project {
			return &Project{Target: TargetMSDOS, Timeout: 5, Start: Item{Autoexec: lines(`@ECHO OFF`)}, Items: []Item{{ID: "DOS", Title: "DOS"}}}
		}},
	{"empty-fd", N("Empty (FreeDOS)"), N("A single empty menu item, for writing everything by hand."),
		func() *Project {
			return &Project{Target: TargetFreeDOS, Timeout: 5, Start: Item{Autoexec: lines(`@ECHO OFF`)}, Items: []Item{{ID: "DOS", Title: "DOS"}}}
		}},
}

// TemplateByID returns the template, or the plain MS-DOS one.
func TemplateByID(id string) Template {
	for _, t := range Templates {
		if t.ID == id {
			return t
		}
	}
	return Templates[0]
}
