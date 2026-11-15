package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// fsTarget is a place DOS files can be written: a folder on the PC (for
// example a mounted image) or a FAT volume inside an image file.
type fsTarget interface {
	ReadFile(p string) ([]byte, error)
	WriteFile(p string, data []byte) error
	Exists(p string) bool
}

type folderTarget struct{ dir string }

func (f folderTarget) path(p string) string {
	return filepath.Join(append([]string{f.dir}, splitPath(p)...)...)
}
func (f folderTarget) ReadFile(p string) ([]byte, error) { return os.ReadFile(f.path(p)) }
func (f folderTarget) Exists(p string) bool {
	_, err := os.Stat(f.path(p))
	return err == nil
}
func (f folderTarget) WriteFile(p string, data []byte) error {
	full := f.path(p)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0644)
}

// Report is what happened during a write, for the summary message.
type Report struct {
	Done     []string
	Warnings []string
}

func backupName(name string) string {
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext) + ".BAK"
}

// MissingDrivers lists selected drivers whose files are not in the store.
// DriverNeeds sorts the files still missing for the project's menu (and,
// with freeDOS, for a FreeDOS system made by the program) by how they can be
// got: download has an address, browse has to be chosen by the user (such
// as OAKCDROM.SYS from a Windows 98 boot disk). File names only, each once.
func DriverNeeds(p *Project, store *Store, freeDOS bool) (download, browse []string) {
	seen := map[string]bool{}
	add := func(d *Driver, files []DriverFile) {
		for _, f := range files {
			if seen[f.Name] {
				continue
			}
			seen[f.Name] = true
			if len(d.URLs) > 0 {
				download = append(download, f.Name)
			} else {
				browse = append(browse, f.Name)
			}
		}
	}
	for _, id := range p.Drivers {
		if d := store.Find(id); d != nil {
			add(d, store.Missing(d))
		}
	}
	if freeDOS {
		files, missing := FreeDOSFiles(p, store)
		for _, id := range missing {
			d := store.Find(id)
			if d == nil {
				continue
			}
			var need []DriverFile
			for _, f := range d.Files {
				// only the files the FreeDOS system uses
				if files[f.Name] == nil && files[`FREEDOS\BIN\`+f.Name] == nil && (id != "jemm" || f.Name != "JEMM386.EXE") {
					need = append(need, f)
				}
			}
			add(d, need)
		}
	}
	return download, browse
}

func MissingDrivers(p *Project, store *Store) []string {
	var out []string
	for _, id := range p.Drivers {
		d := store.Find(id)
		if d == nil {
			out = append(out, T("%s (not in the catalog)", id))
			continue
		}
		if miss := store.Missing(d); len(miss) > 0 {
			names := make([]string, len(miss))
			for i, f := range miss {
				names[i] = f.Name
			}
			out = append(out, fmt.Sprintf("%s: %s", d.Name, strings.Join(names, ", ")))
		}
	}
	return out
}

func deploy(t fsTarget, p *Project, store *Store, withDrivers bool) (*Report, error) {
	out, err := Generate(p)
	if err != nil {
		return nil, err
	}
	r := &Report{}
	for _, x := range p.Validate() {
		if !x.Fatal {
			r.Warnings = append(r.Warnings, x.Msg)
		}
	}
	files := out.Files(p.Codepage)
	for _, name := range []string{out.ConfigName, out.AutoexecName} {
		data := files[name]
		if old, err := t.ReadFile(name); err == nil {
			if bytes.Equal(old, data) {
				r.Done = append(r.Done, T("%s (unchanged)", name))
				continue
			}
			if err := t.WriteFile(backupName(name), old); err != nil {
				return r, fmt.Errorf(T("could not save %s: %v"), backupName(name), err)
			}
			r.Done = append(r.Done, T("%s (previous version: %s)", name, backupName(name)))
		} else {
			r.Done = append(r.Done, name)
		}
		if err := t.WriteFile(name, data); err != nil {
			return r, fmt.Errorf(T("could not write %s: %v"), name, err)
		}
		back, err := t.ReadFile(name)
		if err != nil || !bytes.Equal(back, data) {
			return r, fmt.Errorf(T("%s does not match when read back; the image may be damaged"), name)
		}
	}
	if !withDrivers {
		return r, nil
	}
	for _, id := range p.Drivers {
		d := store.Find(id)
		if d == nil {
			r.Warnings = append(r.Warnings, T("Unknown driver: %s", id))
			continue
		}
		for _, f := range d.Files {
			if f.KeepExisting && t.Exists(f.Dest) {
				r.Done = append(r.Done, T("%s (already there, not overwritten)", f.Dest))
				continue
			}
			data, err := store.Data(d, f)
			if err != nil {
				r.Warnings = append(r.Warnings, T("%s is missing (%s) and was not copied. Download it or browse for it in the Drivers window.", f.Name, d.Name))
				continue
			}
			if err := t.WriteFile(f.Dest, data); err != nil {
				return r, fmt.Errorf(T("could not write %s: %v"), f.Dest, err)
			}
			r.Done = append(r.Done, f.Dest)
		}
	}
	return r, nil
}

// WriteToFolder writes the files (and optionally the drivers) into a folder.
func WriteToFolder(dir string, p *Project, store *Store, withDrivers bool) (*Report, error) {
	return deploy(folderTarget{dir}, p, store, withDrivers)
}

// WriteToImage writes into the FAT volume of a disk image file.
func WriteToImage(path string, p *Project, store *Store, withDrivers bool) (*Report, error) {
	v, err := OpenImage(path, true)
	if err != nil {
		return nil, err
	}
	defer v.Close()
	r, err := deploy(v, p, store, withDrivers)
	if r != nil {
		r.Done = append([]string{T("Volume: %s", v.Desc)}, r.Done...)
		if !imageBootable(v) {
			r.Warnings = append(r.Warnings, T("The image has no DOS system files (IO.SYS or KERNEL.SYS), so it will not boot by itself. Put a system on it first (SYS A: / SYS C:)."))
		} else if p.Target == TargetFreeDOS && !v.Exists("KERNEL.SYS") {
			r.Warnings = append(r.Warnings, T("The menu was made for FreeDOS, but the image has an MS-DOS system."))
		} else if p.Target == TargetMSDOS && v.Exists("KERNEL.SYS") && !v.Exists("IO.SYS") {
			r.Warnings = append(r.Warnings, T("The menu was made for MS-DOS, but the image has a FreeDOS system."))
		}
		r.Done = append(r.Done, T("Free space: %s", humanSize(v.FreeBytes())))
	}
	if serr := v.Sync(); err == nil && serr != nil {
		err = serr
	}
	return r, err
}

func imageBootable(v *Volume) bool {
	for _, n := range []string{"IO.SYS", "KERNEL.SYS", "IBMBIO.COM"} {
		if v.Exists(n) {
			return true
		}
	}
	return false
}

var reShellP = regexp.MustCompile(`(?i)/P=([^\s]+)`)

// readSource reads the boot files from a folder or an image and imports them.
func readSource(t fsTarget) (*Project, []string, error) {
	target := TargetMSDOS
	cfgName := "CONFIG.SYS"
	if t.Exists("FDCONFIG.SYS") {
		target, cfgName = TargetFreeDOS, "FDCONFIG.SYS"
	}
	cfgBytes, err := t.ReadFile(cfgName)
	if err != nil {
		return nil, nil, fmt.Errorf("%s", T("CONFIG.SYS / FDCONFIG.SYS not found"))
	}
	cfg := DecodeFile(cfgBytes, CP852)
	if target == TargetMSDOS && looksFreeDOS(cfg) {
		target = TargetFreeDOS
	}
	if target == TargetMSDOS && t.Exists("KERNEL.SYS") && !t.Exists("IO.SYS") && !hasMSMenu(cfg) {
		target = TargetFreeDOS
	}
	autoName := "AUTOEXEC.BAT"
	for _, l := range cfg {
		if k, v, ok := splitKV(strings.TrimSpace(stripFDPrefix(l))); ok && (k == "SHELL" || k == "SHELLHIGH") {
			if m := reShellP.FindStringSubmatch(v); m != nil {
				parts := splitPath(m[1])
				if len(parts) == 1 {
					autoName = strings.ToUpper(parts[0])
				}
			}
		}
	}
	var auto []string
	var notes []string
	if b, err := t.ReadFile(autoName); err == nil {
		auto = DecodeFile(b, CP852)
	} else {
		notes = append(notes, T("%s not found; the AUTOEXEC part is empty.", autoName))
	}
	p, n2 := Import(cfg, auto, target)
	notes = append(notes, n2...)
	if target == TargetFreeDOS && cfgName != "FDCONFIG.SYS" {
		p.ConfigName = cfgName
	}
	if autoName != "AUTOEXEC.BAT" {
		p.AutoexecName = autoName
	}
	p.Codepage = guessCodepage(cfgBytes)
	return p, notes, nil
}

func stripFDPrefix(l string) string {
	if m := reFDPrefix.FindStringSubmatch(l); m != nil {
		return m[2]
	}
	return strings.TrimPrefix(strings.TrimSpace(l), "!")
}

func hasMSMenu(cfg []string) bool {
	for _, l := range cfg {
		if strings.EqualFold(strings.TrimSpace(l), "[MENU]") {
			return true
		}
	}
	return false
}

func looksFreeDOS(cfg []string) bool {
	if hasMSMenu(cfg) {
		return false
	}
	for _, l := range cfg {
		if reFDPrefix.MatchString(l) {
			return true
		}
	}
	return false
}

func guessCodepage(b []byte) Codepage {
	for _, c := range b {
		if c >= 0x80 {
			return CP852
		}
	}
	return CPASCII
}

func ReadFromFolder(dir string) (*Project, []string, error) { return readSource(folderTarget{dir}) }

func ReadFromImage(path string) (*Project, []string, error) {
	v, err := OpenImage(path, false)
	if err != nil {
		return nil, nil, err
	}
	defer v.Close()
	return readSource(v)
}
