package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Target is the DOS flavour the files are generated for.
type Target int

const (
	TargetMSDOS   Target = 0 // MS-DOS 6.x: CONFIG.SYS [MENU] blocks
	TargetFreeDOS Target = 1 // FreeDOS: FDCONFIG.SYS with MENU lines and "1?" prefixes
)

func (t Target) String() string {
	if t == TargetFreeDOS {
		return "FreeDOS"
	}
	return "MS-DOS 6.x"
}

// Codepage controls how non-ASCII characters are written into the DOS files.
type Codepage int

const (
	CPASCII Codepage = 0 // accents are stripped (safe: the boot menu is shown before any codepage is loaded)
	CP852   Codepage = 1
	CP437   Codepage = 2
)

// Item is one boot menu entry. Config lines go into the item's CONFIG.SYS
// block, Autoexec lines into its AUTOEXEC.BAT branch.
type Item struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Config   []string `json:"config"`
	Autoexec []string `json:"autoexec"`
	// Blocks, when set, is what the lines were built from (see blocks.go).
	Blocks *BlockSet `json:"blocks,omitempty"`
}

// Project is everything the program saves and generates from.
type Project struct {
	Target   Target   `json:"target"`
	Codepage Codepage `json:"codepage"`
	// Timeout in seconds before the default item starts; -1 = wait forever.
	Timeout int `json:"timeout"`
	// Default is the index into Items.
	Default int `json:"default"`
	// MenuColor, e.g. "7,1" (foreground,background); empty = DOS default.
	MenuColor string `json:"menucolor"`
	// Lines that run for every item, before and after the item's own lines.
	Start Item   `json:"start"`
	End   Item   `json:"end"`
	Items []Item `json:"items"`
	// Driver IDs (see drivers.go) to copy into a disk image.
	Drivers []string `json:"drivers"`
	// File names; empty means the default for the target.
	ConfigName   string `json:"configname"`
	AutoexecName string `json:"autoexecname"`
}

const MaxItems = 9

func (p *Project) ConfigFile() string {
	if p.ConfigName != "" {
		return strings.ToUpper(p.ConfigName)
	}
	if p.Target == TargetFreeDOS {
		return "FDCONFIG.SYS"
	}
	return "CONFIG.SYS"
}

func (p *Project) AutoexecFile() string {
	if p.AutoexecName != "" {
		return strings.ToUpper(p.AutoexecName)
	}
	return "AUTOEXEC.BAT"
}

var reservedIDs = map[string]bool{"MENU": true, "COMMON": true, "BMEND": true}

// ValidID reports whether s can be a block name and a batch label:
// 1-8 characters, A-Z 0-9 _ only (labels are only 8 characters significant in DOS).
func ValidID(s string) error {
	if s == "" {
		return fmt.Errorf("%s", T("empty ID"))
	}
	if len(s) > 8 {
		return fmt.Errorf(T("%q is too long (at most 8 characters)"), s)
	}
	for _, c := range s {
		if !(c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return fmt.Errorf(T("%q may only contain A-Z, 0-9 and _"), s)
		}
	}
	if reservedIDs[s] {
		return fmt.Errorf(T("%q is a reserved name"), s)
	}
	return nil
}

// Problem is a validation finding. Fatal ones block generation.
type Problem struct {
	Fatal bool
	Msg   string
}

func (p *Project) Validate() []Problem {
	var out []Problem
	bad := func(f string, a ...any) { out = append(out, Problem{true, T(f, a...)}) }
	warn := func(f string, a ...any) { out = append(out, Problem{false, T(f, a...)}) }
	if len(p.Items) == 0 {
		bad("There are no menu items.")
	}
	if len(p.Items) > MaxItems {
		bad("At most %d menu items are allowed (there are %d).", MaxItems, len(p.Items))
	}
	seen := map[string]bool{}
	for i, it := range p.Items {
		if err := ValidID(it.ID); err != nil {
			bad("ID of menu item %d: %v", i+1, err)
		}
		if seen[it.ID] {
			bad("The ID %s is used twice.", it.ID)
		}
		seen[it.ID] = true
		if strings.TrimSpace(it.Title) == "" {
			warn("Menu item %d has no text; its ID will be shown.", i+1)
		}
		if p.Target == TargetMSDOS && strings.ContainsAny(it.Title, ",") {
			warn("The text of %s contains a comma; MS-DOS accepts it, but check the menu.", it.ID)
		}
		if len(it.Title) > 70 {
			warn("The text of %s is longer than 70 characters; DOS may cut it.", it.ID)
		}
	}
	if p.Default < 0 || p.Default >= len(p.Items) {
		if len(p.Items) > 0 {
			bad("The default menu item does not exist.")
		}
	}
	if p.Timeout > 90 && p.Target == TargetMSDOS {
		bad("MS-DOS allows a timeout of at most 90 seconds.")
	}
	if p.MenuColor != "" {
		parts := strings.Split(p.MenuColor, ",")
		okc := len(parts) <= 2
		for _, s := range parts {
			n := 0
			if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n); err != nil || n < 0 || n > 15 {
				okc = false
			}
		}
		if !okc {
			bad("Menu colour format: foreground[,background], numbers 0-15 (e.g. 15,1).")
		}
	}
	if p.Codepage == CPASCII {
		all := []Item{p.Start, p.End}
		all = append(all, p.Items...)
		for _, it := range all {
			for _, s := range append(append([]string{it.Title}, it.Config...), it.Autoexec...) {
				if hasNonASCII(s) {
					warn("Accented letters are written without accents (ASCII mode).")
					return out
				}
			}
		}
	}
	return out
}

func HasFatal(ps []Problem) bool {
	for _, p := range ps {
		if p.Fatal {
			return true
		}
	}
	return false
}

func (p *Project) Save(path string) error {
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

func LoadProject(path string) (*Project, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p := &Project{}
	if err := json.Unmarshal(b, p); err != nil {
		return nil, fmt.Errorf(T("invalid project file: %v"), err)
	}
	return p, nil
}

// Clone returns a deep copy.
func (p *Project) Clone() *Project {
	b, _ := json.Marshal(p)
	q := &Project{}
	json.Unmarshal(b, q)
	return q
}

// UniqueID returns base, or base with a number, not used by any item.
func (p *Project) UniqueID(base string) string {
	used := map[string]bool{}
	for _, it := range p.Items {
		used[it.ID] = true
	}
	if !used[base] && ValidID(base) == nil {
		return base
	}
	if len(base) > 6 {
		base = base[:6]
	}
	for i := 2; ; i++ {
		s := fmt.Sprintf("%s%d", base, i)
		if !used[s] {
			return s
		}
	}
}
