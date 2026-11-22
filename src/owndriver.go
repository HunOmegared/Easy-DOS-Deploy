package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Own drivers: a driver file (or a zip of one) the user adds, with the line
// that loads it, into chosen menu items. The entry goes into the user's
// catalog, so later projects have it in the list.

// OwnLine is a line of an own driver in a block-made menu item; the item
// is rebuilt with it.
type OwnLine struct {
	Driver string `json:"driver"`
	Config bool   `json:"config,omitempty"` // CONFIG.SYS, else AUTOEXEC.BAT
	Line   string `json:"line"`
}

// Kinds of own drivers, for the list text.
var OwnGroups = []string{"mouse", "cdrom", "sound", "keyboard", "other"}

func OwnGroupNames() []string {
	return []string{T("Mouse"), "CD-ROM", T("Sound"), T("Keyboard"), T("Other")}
}

// OwnDriver describes the driver to add.
type OwnDriver struct {
	Name   string
	Group  string // one of OwnGroups
	Source string // the file or zip
	Main   string // the file the line loads (from a zip: its name)
	Config bool
	Line   string
}

// isLoadable: the files a load line can name.
func isLoadable(name string) bool {
	switch strings.ToUpper(path.Ext(name)) {
	case ".SYS", ".COM", ".EXE":
		return true
	}
	return false
}

// ZipDriverFiles lists the file names in a zip (without folders), the
// loadable ones first.
func ZipDriverFiles(p string) ([]string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	es, err := listZip(b, "", 2) // no nested zips
	if err != nil {
		return nil, fmt.Errorf(T("not a zip file: %v"), err)
	}
	seen := map[string]bool{}
	var names []string
	for _, e := range es {
		n := strings.ToUpper(path.Base(strings.ReplaceAll(e.path, `\`, "/")))
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		names = append(names, n)
	}
	sort.SliceStable(names, func(i, j int) bool { return isLoadable(names[i]) && !isLoadable(names[j]) })
	return names, nil
}

// ownDest is where the files go: C:\DRIVERS\FILE, or for a zip its own
// folder C:\DRIVERS\NAME\ (drivers with several files).
func ownDest(o OwnDriver) string {
	if strings.EqualFold(filepath.Ext(o.Source), ".zip") {
		return `DRIVERS\` + dosStem(o.Main) + `\`
	}
	return `DRIVERS\`
}

func dosStem(name string) string {
	s := strings.ToUpper(strings.TrimSuffix(path.Base(name), path.Ext(name)))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if len(out) > 8 {
		out = out[:8]
	}
	if out == "" {
		out = "DRIVER"
	}
	return out
}

// SuggestOwnLine proposes the line that loads the main file.
func SuggestOwnLine(o OwnDriver) (line string, config bool) {
	p := `C:\` + ownDest(o) + strings.ToUpper(path.Base(o.Main))
	if strings.EqualFold(path.Ext(o.Main), ".SYS") {
		return "DEVICE=" + p, true
	}
	return p, false
}

// AddOwnDriver puts the driver into the catalog and its files into the
// driver folder.
func (s *Store) AddOwnDriver(o OwnDriver) (*Driver, error) {
	if strings.TrimSpace(o.Name) == "" {
		return nil, fmt.Errorf("%s", T("Give the driver a name."))
	}
	files := []string{strings.ToUpper(path.Base(o.Main))}
	if strings.EqualFold(filepath.Ext(o.Source), ".zip") {
		var err error
		if files, err = ZipDriverFiles(o.Source); err != nil {
			return nil, err
		}
	}
	base := "own-" + strings.ToLower(dosStem(o.Main))
	id := base
	for n := 2; s.Find(id) != nil; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	group := "other"
	for i, g := range OwnGroups {
		if g == o.Group {
			group = OwnGroupNames()[i]
		}
	}
	d := Driver{ID: id, Name: strings.TrimSpace(o.Name), Desc: T("Your own driver (%s), added from %s.", group, filepath.Base(o.Source))}
	dest := ownDest(o)
	for _, f := range files {
		d.Files = append(d.Files, DriverFile{Name: f, Dest: dest + f})
	}
	s.Catalog = append(s.Catalog, d)
	dp := &s.Catalog[len(s.Catalog)-1]
	if _, err := s.AddFile(dp, o.Source); err != nil {
		s.Catalog = s.Catalog[:len(s.Catalog)-1]
		return nil, err
	}
	if err := s.SaveCatalog(); err != nil {
		return nil, err
	}
	return dp, nil
}

// AddOwnLine adds the line of a driver to menu items. A block-made item
// keeps it among its blocks (so rebuilding keeps it), other items get the
// line itself.
func (p *Project) AddOwnLine(id string, items []int, config bool, line string) {
	for _, i := range items {
		if i < 0 || i >= len(p.Items) {
			continue
		}
		it := &p.Items[i]
		if it.Blocks != nil && it.BlocksMatch(p.Target, p.Machine) {
			it.Blocks.Own = append(it.Blocks.Own, OwnLine{Driver: id, Config: config, Line: line})
			it.Config, it.Autoexec, _ = it.Blocks.BuildFor(p.Target, p.Machine)
			continue
		}
		if config {
			it.Config = append(it.Config, line)
		} else {
			it.Autoexec = append(it.Autoexec, line)
		}
	}
	p.AddDrivers([]string{id})
}
