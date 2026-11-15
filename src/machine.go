package main

import (
	"path"
	"regexp"
	"sort"
	"strings"
)

// Machine holds the drivers found on an existing DOS system, for example a
// backup of the card the target machine boots from: the lines that load its
// mouse, CD-ROM and sound drivers and set up its keyboard. A new menu made
// from building blocks can use them instead of the program's own choices
// (CuteMouse, OAKCDROM, a default BLASTER), so the machine keeps the drivers
// that are known to work on it.
//
// The lines are stored without high loading (LH, DEVICEHIGH and the /L:
// and /S options MEMMAKER writes); the blocks add it again for the memory
// setting of each menu item.
type Machine struct {
	Source   string `json:"source,omitempty"` // where they were found
	Mouse    Group  `json:"mouse"`
	CDROM    Group  `json:"cdrom"`
	Sound    Group  `json:"sound"`
	Keyboard Group  `json:"keyboard"` // COUNTRY, KEYB, code pages: every item
	// Other lists lines that were found but are not used (network, tools),
	// for the user to see.
	Other []string `json:"other,omitempty"`
}

// Group is the CONFIG.SYS and AUTOEXEC.BAT lines of one kind of driver.
type Group struct {
	Config   []string `json:"config,omitempty"`
	Autoexec []string `json:"autoexec,omitempty"`
	// Found tells where the lines came from when the system did not load
	// the driver: "rem" (a commented-out line), "file" (only the driver file
	// was on the system); empty: loaded.
	Found string `json:"found,omitempty"`
}

func (g Group) Empty() bool { return len(g.Config) == 0 && len(g.Autoexec) == 0 }

// Programs is a short description: the program names of the group.
func (g Group) Programs() string {
	var names []string
	seen := map[string]bool{}
	for _, l := range append(append([]string{}, g.Config...), g.Autoexec...) {
		n := lineProgram(l)
		if n == "" || n == "SET" || seen[n] {
			continue
		}
		seen[n] = true
		names = append(names, n)
	}
	if len(names) == 0 && len(g.Autoexec) > 0 {
		// only SET lines, e.g. SET BLASTER=...
		for _, l := range g.Autoexec {
			if k, _, ok := splitKV(strings.TrimSpace(l[3:])); ok && strings.HasPrefix(strings.ToUpper(l), "SET ") && !seen[k] {
				seen[k] = true
				names = append(names, k)
			}
		}
	}
	return strings.Join(names, ", ")
}

// Empty reports whether nothing usable was found.
func (m *Machine) Empty() bool {
	return m == nil || m.Mouse.Empty() && m.CDROM.Empty() && m.Sound.Empty() && m.Keyboard.Empty()
}

// Summary is one line for the user: what was found, by kind.
func (m *Machine) Summary() string {
	if m.Empty() {
		return ""
	}
	var parts []string
	add := func(name string, g Group) {
		if !g.Empty() {
			parts = append(parts, name+" ("+g.Programs()+")")
		}
	}
	add(T("mouse"), m.Mouse)
	add("CD-ROM", m.CDROM)
	add(T("sound"), m.Sound)
	add(T("keyboard"), m.Keyboard)
	return strings.Join(parts, "; ")
}

var (
	reHighOpt   = regexp.MustCompile(`(?i)^/(L:[0-9,;]+|S)$`)
	reCfgKey    = regexp.MustCompile(`(?i)^(DEVICE|DEVICEHIGH|INSTALL|INSTALLHIGH|COUNTRY)\??$`)
	reMenuLine  = regexp.MustCompile(`(?i)^(MENUITEM|MENUDEFAULT|MENUCOLOR|SUBMENU|NUMLOCK|INCLUDE|MENU)\b`)
	reSetSound  = regexp.MustCompile(`(?i)^SET\s+(BLASTER|ULTRASND|ULTRADIR|ULTRA16|SOUND|MIDI|GUS\w*|INTERWAVE|IWDIR|SBPCI|MSSOUND|AWE\w*)\s*=`)
	reSetKeyb   = regexp.MustCompile(`(?i)^SET\s+(KEYB|LANG|NLSPATH)\s*=`)
	reSkipStart = regexp.MustCompile(`(?i)^(REM\b|::|ECHO\b|IF\b|GOTO\b|:|PAUSE\b|CLS\b|PROMPT\b|PATH\b|SET\s+(PATH|PROMPT|TEMP|TMP|DIRCMD|COMSPEC|DOSDIR|CONFIG)\s*=|CHOICE\b|CTTY\b|BREAK\b|VER\b|WIN\b|EXIT\b)`)
)

// Program names, without extension.
var (
	mousePrograms    = set("MOUSE", "CTMOUSE", "GMOUSE", "LMOUSE", "MSMOUSE", "IMOUSE", "MOUSEDRV", "AMOUSE", "KMOUSE", "MMOUSE", "CUTEMOUS")
	cdExtPrograms    = set("MSCDEX", "SHSUCDX", "NWCDEX", "SHCDX33E")
	soundPrograms    = set("DIAGNOSE", "SBCONFIG", "CTCM", "CTCU", "SBINIT", "ULTRINIT", "ULTRAMID", "UNISOUND", "MIXERSET", "AWEUTIL", "SB16SET", "SBSET", "SNDINIT", "SETSOUND", "AUDINIT", "ESSVOL", "ESSINIT", "OPL3SA", "IWINIT", "SBMIX", "CS4232", "CWDINIT", "YMFSB", "SETUPSB", "PNPINIT", "ICU", "DWCFGMG", "SBPCI", "VIBRAINI", "AZTPNP", "ADDINIT", "MSSINIT")
	soundDevices     = set("CTSB16", "CTMMSYS", "CTCM", "SBPNP", "ESSCFG", "OPL3SA", "CS4232", "AWEDRV", "DWCFGMG", "SBIDE", "SBCD")
	keybPrograms     = set("KEYB", "MODE", "NLSFUNC", "CHCP", "GRAFTABL", "DISPLAY")
	memoryPrograms   = set("HIMEM", "EMM386", "QEMM386", "QEMM", "JEMMEX", "JEMM386", "HIMEMX", "UMBPCI", "386MAX", "SETVER")
	ignoredPrograms  = set("COMMAND", "SMARTDRV", "DOSKEY", "SHARE", "ANSI", "RAMDRIVE", "DBLSPACE", "DRVSPACE", "MEM", "MEMMAKER", "WIN", "DOSSHELL", "SCANDISK", "CHKDSK")
	networkPrograms  = set("NE2000", "3C509", "3C5X9PD", "PCNTPK", "RTSPKT", "NET", "DHCP", "IPXODI", "LSL", "NETBIND", "PROTMAN", "EC", "NETX", "VLM")
	soundDirPrefixes = []string{`\SB16\`, `\SBPRO\`, `\SB\`, `\ULTRASND\`, `\GRAVIS\`, `\AWE\`, `\ESS\`, `\AUDIO\`, `\SOUND\`, `\CREATIVE\`, `\YAMAHA\`, `\CRYSTAL\`, `\OPTI\`, `\AZTECH\`}
)

func set(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

// normConfig turns a CONFIG.SYS driver line into "path args" without high
// loading; key is DEVICE, INSTALL or COUNTRY.
func normConfig(l string) (key, line string, ok bool) {
	i := strings.Index(l, "=")
	if i < 0 {
		return "", "", false
	}
	head := strings.Fields(l[:i])
	if len(head) == 0 || !reCfgKey.MatchString(head[0]) {
		return "", "", false
	}
	key = strings.ToUpper(strings.TrimSuffix(head[0], "?"))
	key = strings.TrimSuffix(key, "HIGH")
	f := strings.Fields(l[i+1:])
	for len(f) > 0 && (reHighOpt.MatchString(f[0]) || strings.HasPrefix(strings.ToUpper(f[0]), "SIZE=")) {
		f = f[1:] // DEVICEHIGH=/L:1,12048 /S C:\...
	}
	if len(f) == 0 {
		return "", "", false
	}
	return key, strings.Join(f, " "), true
}

// normAutoexec drops @, LH / LOADHIGH and their /L: /S options.
func normAutoexec(l string) string {
	l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "@"))
	f := strings.Fields(l)
	if len(f) > 0 && (strings.EqualFold(f[0], "LH") || strings.EqualFold(f[0], "LOADHIGH")) {
		f = f[1:]
		for len(f) > 0 && reHighOpt.MatchString(f[0]) {
			f = f[1:]
		}
		return strings.Join(f, " ")
	}
	return l
}

// lineProgram is the program a normalized line runs: its file name in
// capitals without extension ("MSCDEX"), "SET" for SET lines, "CALL" + name
// for CALL.
func lineProgram(l string) string {
	f := strings.Fields(l)
	if len(f) == 0 {
		return ""
	}
	first := strings.ToUpper(f[0])
	if first == "SET" {
		return "SET"
	}
	if first == "CALL" && len(f) > 1 {
		first = strings.ToUpper(f[1])
	}
	first = strings.ReplaceAll(first, "/", `\`)
	base := path.Base(strings.ReplaceAll(first, `\`, "/"))
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	return base
}

func linePath(l string) string {
	f := strings.Fields(l)
	if len(f) == 0 {
		return ""
	}
	if strings.EqualFold(f[0], "CALL") && len(f) > 1 {
		return f[1]
	}
	return f[0]
}

func inSoundDir(l string) bool {
	p := strings.ToUpper(linePath(l))
	for _, d := range soundDirPrefixes {
		if strings.Contains(p, d) {
			return true
		}
	}
	return false
}

// bodyLines are the lines that load something: the menu structure, the
// FreeDOS prefixes and comments are left out; every menu block counts.
func cfgBodyLines(cfg []string) []string {
	var out []string
	for _, l := range cfg {
		l = strings.TrimSpace(stripFDPrefix(l))
		if l == "" || strings.HasPrefix(l, "[") || strings.HasPrefix(strings.ToUpper(l), "REM") || strings.HasPrefix(l, ";") || reMenuLine.MatchString(l) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// FindMachine sorts the lines of a system's CONFIG and AUTOEXEC into kinds.
// read gives the content of a file on that system (for CALLed batch files
// and to check that a driver is there); nil skips both.
func FindMachine(cfg, auto []string, read func(p string) ([]byte, error)) *Machine {
	return findMachine(cfg, auto, read, nil)
}

// Known driver files, for a system that has them but does not load them,
// in the order they are preferred.
var (
	cdDriverFiles = []string{"OAKCDROM.SYS", "VIDE-CDD.SYS", "ATAPI.SYS", "AOATAPI.SYS", "UDVD2.SYS", "XCDROM.SYS", "SBIDE.SYS", "MTMCDAI.SYS", "IDE-CD.SYS", "ATAPICD.SYS", "CDROM.SYS"}
	cdExtFiles    = []string{"MSCDEX.EXE", "SHSUCDX.COM", "SHSUCDX.EXE"}
	mouseFiles    = []string{"CTMOUSE.EXE", "MOUSE.COM", "MOUSE.EXE", "GMOUSE.COM"}
	reDevName     = regexp.MustCompile(`(?i)/D:(\S+)`)
)

// findMachine is FindMachine; files (paths on the volume, as Walk gives
// them) lets it find drivers that are on the system but not loaded.
func findMachine(cfg, auto []string, read func(p string) ([]byte, error), files []string) *Machine {
	m := &Machine{}
	seen := map[string]bool{}
	add := func(g *Group, config bool, l string) {
		k := strings.ToUpper(l)
		if config {
			k = "C:" + k
		}
		if seen[k] {
			return
		}
		seen[k] = true
		if config {
			g.Config = append(g.Config, l)
		} else {
			g.Autoexec = append(g.Autoexec, l)
		}
	}
	other := func(l string) {
		if !seen["O:"+strings.ToUpper(l)] {
			seen["O:"+strings.ToUpper(l)] = true
			m.Other = append(m.Other, l)
		}
	}
	for _, raw := range cfgBodyLines(cfg) {
		key, l, ok := normConfig(raw)
		if !ok {
			continue
		}
		if key == "COUNTRY" {
			add(&m.Keyboard, true, "COUNTRY="+l)
			continue
		}
		prog := lineProgram(l)
		up := strings.ToUpper(l)
		switch {
		case memoryPrograms[prog] || ignoredPrograms[prog]:
		case mousePrograms[prog]:
			add(&m.Mouse, true, l)
		case prog == "DISPLAY":
			add(&m.Keyboard, true, l)
		case soundDevices[prog] || inSoundDir(l):
			add(&m.Sound, true, l)
		case strings.Contains(up, "/D:"):
			add(&m.CDROM, true, l) // a CD-ROM device driver names its device
		default:
			other(raw)
		}
	}
	var scan func(lines []string, depth int)
	scan = func(lines []string, depth int) {
		for _, raw := range lines {
			l := normAutoexec(stripFDPrefix(raw))
			if l == "" || reSkipStart.MatchString(l) {
				continue
			}
			prog := lineProgram(l)
			switch {
			case reSetSound.MatchString(l):
				add(&m.Sound, false, l)
			case reSetKeyb.MatchString(l):
				add(&m.Keyboard, false, l)
			case prog == "SET" || memoryPrograms[prog] || ignoredPrograms[prog]:
			case mousePrograms[prog]:
				add(&m.Mouse, false, l)
			case cdExtPrograms[prog]:
				add(&m.CDROM, false, l)
			case keybPrograms[prog]:
				add(&m.Keyboard, false, l)
			case soundPrograms[prog] || inSoundDir(l):
				add(&m.Sound, false, l)
			case networkPrograms[prog]:
				other(raw)
			case strings.HasPrefix(strings.ToUpper(l), "CALL ") && depth == 0 && read != nil:
				// a batch file of its own: it counts as what it loads
				b, err := read(dosPath(linePath(l)))
				if err != nil {
					other(raw)
					continue
				}
				sub := FindMachine(nil, DecodeFile(b, CP852), nil)
				switch {
				case !sub.Sound.Empty():
					add(&m.Sound, false, l)
				case !sub.Mouse.Empty():
					add(&m.Mouse, false, l)
				case !sub.CDROM.Empty():
					add(&m.CDROM, false, l)
				default:
					other(raw)
				}
			default:
				other(raw)
			}
		}
	}
	scan(auto, 0)
	if read != nil || files != nil {
		m.complete(cfg, auto, read, files) // only where the files can be checked
	}
	// a CD-ROM needs both halves to be any use
	if len(m.CDROM.Config) == 0 || len(m.CDROM.Autoexec) == 0 {
		for _, l := range append(m.CDROM.Config, m.CDROM.Autoexec...) {
			other(l)
		}
		m.CDROM = Group{}
	}
	return m
}

// complete fills in what the system has but does not load: lines commented
// out with REM (when the file they name is there) and, for the CD-ROM and
// the mouse, driver files no line names.
func (m *Machine) complete(cfg, auto []string, read func(p string) ([]byte, error), files []string) {
	have := map[string]bool{}    // upper-case paths on the volume
	where := map[string]string{} // upper-case file name -> path, shortest first
	for _, f := range files {
		up := strings.ToUpper(f)
		have[up] = true
		name := path.Base(strings.ReplaceAll(up, `\`, "/"))
		if old, ok := where[name]; !ok || len(up) < len(old) {
			where[name] = f
		}
	}
	exists := func(l string) bool {
		p := linePath(l)
		if !strings.Contains(p, `\`) {
			return false
		}
		dp := dosPath(p)
		cands := []string{dp}
		if !strings.Contains(path.Base(strings.ReplaceAll(dp, `\`, "/")), ".") {
			cands = []string{dp + ".EXE", dp + ".COM"}
		}
		for _, c := range cands {
			if files != nil && have[strings.ToUpper(c)] {
				return true
			}
			if files == nil && read != nil {
				if _, err := read(c); err == nil {
					return true
				}
			}
		}
		return false
	}
	// the commented-out lines, as a system of their own
	unrem := func(lines []string) []string {
		var out []string
		for _, l := range lines {
			t := strings.TrimSpace(stripFDPrefix(l))
			if len(t) > 4 && strings.EqualFold(t[:4], "REM ") {
				out = append(out, strings.TrimSpace(t[4:]))
			}
		}
		return out
	}
	rem := FindMachine(unrem(cfg), unrem(auto), nil)
	usable := func(g Group) Group {
		var u Group
		for _, l := range g.Config {
			if exists(l) {
				u.Config = append(u.Config, l)
			}
		}
		for _, l := range g.Autoexec {
			if strings.HasPrefix(strings.ToUpper(l), "SET ") || exists(l) {
				u.Autoexec = append(u.Autoexec, l)
			}
		}
		return u
	}
	fileLine := func(names []string) string {
		for _, n := range names {
			if p, ok := where[n]; ok {
				return `C:\` + p
			}
		}
		return ""
	}

	// CD-ROM: the device driver and the extension, with the same name
	cd := m.CDROM
	found := ""
	if len(cd.Config) == 0 {
		if r := usable(Group{Config: remCDConfig(rem, unrem(cfg))}); len(r.Config) > 0 {
			cd.Config, found = r.Config[:1], "rem"
		} else if l := fileLine(cdDriverFiles); l != "" {
			cd.Config, found = []string{l + " /D:MSCD001"}, "file"
		}
	}
	if len(cd.Autoexec) == 0 && len(cd.Config) > 0 {
		var ext []string
		for _, l := range unrem(auto) {
			if cdExtPrograms[lineProgram(normAutoexec(l))] {
				ext = append(ext, normAutoexec(l))
			}
		}
		if r := usable(Group{Autoexec: ext}); len(r.Autoexec) > 0 {
			cd.Autoexec = r.Autoexec[:1]
			if found == "" {
				found = "rem"
			}
		} else if l := fileLine(cdExtFiles); l != "" {
			cd.Autoexec = []string{l + " /D:MSCD001"}
			if found == "" {
				found = "file"
			}
		}
	}
	if len(cd.Config) > 0 && len(cd.Autoexec) > 0 {
		// one device name for both: the loaded line's, or the driver's
		name := ""
		if found == "" || len(m.CDROM.Config) > 0 {
			name = devName(cd.Config[0])
		} else if len(m.CDROM.Autoexec) > 0 {
			name = devName(m.CDROM.Autoexec[0])
		}
		if name == "" {
			name = devName(cd.Config[0])
		}
		if name == "" {
			name = "MSCD001"
		}
		cd.Config = []string{setDevName(cd.Config[0], name)}
		cd.Autoexec = []string{setDevName(cd.Autoexec[0], name)}
		cd.Found = found
		m.CDROM = cd
	}

	if m.Mouse.Empty() {
		if r := usable(rem.Mouse); !r.Empty() {
			r.Found = "rem"
			m.Mouse = r
		} else if l := fileLine(mouseFiles); l != "" {
			m.Mouse = Group{Autoexec: []string{l}, Found: "file"}
		}
	}
	if m.Sound.Empty() {
		// only with a program to run; SET lines alone may be stale
		if r := usable(rem.Sound); len(r.Config) > 0 || hasProgram(r.Autoexec) {
			r.Found = "rem"
			m.Sound = r
		}
	}
}

// remCDConfig are the commented-out CONFIG lines that load a CD driver.
func remCDConfig(rem *Machine, cfg []string) []string {
	var out []string
	for _, raw := range cfg {
		if _, l, ok := normConfig(raw); ok && strings.Contains(strings.ToUpper(l), "/D:") && !mousePrograms[lineProgram(l)] {
			out = append(out, l)
		}
	}
	return out
}

func hasProgram(lines []string) bool {
	for _, l := range lines {
		if !strings.HasPrefix(strings.ToUpper(l), "SET ") {
			return true
		}
	}
	return false
}

func devName(l string) string {
	if m := reDevName.FindStringSubmatch(l); m != nil {
		return strings.ToUpper(m[1])
	}
	return ""
}

func setDevName(l, name string) string {
	if reDevName.MatchString(l) {
		return reDevName.ReplaceAllString(l, "/D:"+name)
	}
	return l + " /D:" + name
}

// dosPath turns C:\DIR\FILE.BAT into a path on the volume (DIR\FILE.BAT).
func dosPath(p string) string {
	p = strings.ReplaceAll(p, "/", `\`)
	if len(p) >= 2 && p[1] == ':' {
		p = p[2:]
	}
	return strings.TrimPrefix(p, `\`)
}

// MissingFiles lists the driver files the machine's lines name that are not
// on the system (exists answers for a path on the volume).
func (m *Machine) MissingFiles(exists func(p string) bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, g := range []Group{m.Mouse, m.CDROM, m.Sound, m.Keyboard} {
		for _, l := range append(append([]string{}, g.Config...), g.Autoexec...) {
			if strings.HasPrefix(strings.ToUpper(l), "SET ") || strings.HasPrefix(strings.ToUpper(l), "COUNTRY=") {
				continue
			}
			p := linePath(l)
			if !strings.Contains(p, `\`) {
				continue // on the PATH; cannot tell
			}
			dp := dosPath(p)
			if !strings.Contains(path.Base(strings.ReplaceAll(dp, `\`, "/")), ".") {
				if exists(dp+".EXE") || exists(dp+".COM") || exists(dp+".BAT") {
					continue
				}
			} else if exists(dp) {
				continue
			}
			if !seen[strings.ToUpper(dp)] {
				seen[strings.ToUpper(dp)] = true
				out = append(out, p)
			}
		}
	}
	sort.Strings(out)
	return out
}

// UseMachine makes the block-made items use the drivers of a machine (nil:
// the program's own again). Items edited by hand are left alone and their
// IDs returned. The keyboard and code page lines go to the lines every item
// runs. Catalog drivers no item needs any more are taken off the list.
func (p *Project) UseMachine(m *Machine) (kept []string) {
	if m != nil && m.Empty() {
		m = nil
	}
	old := p.Machine
	// keyboard lines of the old machine out, of the new one in
	if old != nil {
		p.Start.Config = without(p.Start.Config, old.Keyboard.Config)
		p.Start.Autoexec = without(p.Start.Autoexec, old.Keyboard.Autoexec)
	}
	if m != nil {
		p.Start.Config = append(p.Start.Config, m.Keyboard.Config...)
		p.Start.Autoexec = append(p.Start.Autoexec, m.Keyboard.Autoexec...)
	}
	need := map[string]bool{}
	for i := range p.Items {
		it := &p.Items[i]
		if it.Blocks == nil {
			continue
		}
		if !it.BlocksMatch(p.Target, old) {
			kept = append(kept, it.ID)
			_, _, drv := it.Blocks.BuildFor(p.Target, old)
			for _, d := range drv {
				need[d] = true
			}
			continue
		}
		cfg, auto, drv := it.Blocks.BuildFor(p.Target, m)
		it.Config, it.Autoexec = cfg, auto
		for _, d := range drv {
			need[d] = true
		}
	}
	p.Machine = m
	// the drivers a machine can replace; others (network, EC...) stay
	replaceable := set("ctmouse", "oakcdrom", "udvd2", "unisound")
	var keep []string
	for _, d := range p.Drivers {
		if !replaceable[d] || need[d] {
			keep = append(keep, d)
		}
	}
	p.Drivers = keep
	p.AddDrivers(sortedKeys(need))
	return kept
}

func without(lines, drop []string) []string {
	d := map[string]bool{}
	for _, l := range drop {
		d[strings.ToUpper(l)] = true
	}
	var out []string
	for _, l := range lines {
		if !d[strings.ToUpper(l)] {
			out = append(out, l)
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// MachineFromImage finds the drivers of the DOS system in a disk image (a
// card backup), with the files its lines name that are not on it.
func MachineFromImage(path string) (*Machine, []string, error) {
	v, err := OpenImage(path, false)
	if err != nil {
		return nil, nil, err
	}
	defer v.Close()
	bf, err := readBootFiles(v)
	if err != nil {
		return nil, nil, err
	}
	var files []string
	v.Walk(func(fi FileInfo) error {
		if !fi.Dir {
			files = append(files, fi.Path)
		}
		return nil
	})
	m := findMachine(bf.cfg, bf.auto, v.ReadFile, files)
	m.Source = path
	return m, m.MissingFiles(v.Exists), nil
}
