package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MS-DOS 6.x from its Setup disks: the program does what SETUP does when it
// copies the files (expands the compressed ones into C:\DOS), without
// running SETUP on the target machine. PACKING.LST on disk 1 lists every
// disk and the expanded name of each file.

// SetupDisks is what was read from a set of Setup disks.
type SetupDisks struct {
	Version string // first line of PACKING.LST, e.g. "Microsoft MS-DOS 6.22"
	Boot    string // the image of disk 1 (boot sector and system files)
	Found   []int  // disk numbers found, by their files
	Files   []setupFile
	Missing map[int][]string // files of disks that were not given
	Notes   []string
}

type setupFile struct {
	Name string // expanded name
	Disk int    // 0: not in PACKING.LST
	Data []byte
}

// Choice of a component that SETUP installs for MS-DOS, Windows or both.
const (
	CompNone = iota
	CompDOS
	CompWindows
	CompBoth
)

// SetupOptions are SETUP's choices.
type SetupOptions struct {
	Backup, AntiVirus, Undelete int // CompNone ... CompBoth
	DriveSpace                  bool
}

// DefaultSetupOptions: the MS-DOS versions, as SETUP offers without Windows.
func DefaultSetupOptions() SetupOptions {
	return SetupOptions{Backup: CompDOS, AntiVirus: CompDOS, Undelete: CompDOS}
}

// Files of the optional components (expanded names).
var (
	setupOnly   = []string{"SETUP.EXE", "SETUP.MSG", "DOSSETUP.INI", "PACKING.LST", "AUTOEXEC.BAT", "CONFIG.SYS", "IO.SYS", "MSDOS.SYS"}
	compBackup  = [2][]string{{"MSBACKUP.EXE", "MSBACKUP.HLP", "MSBACKUP.OVL", "MSBCONFG.HLP", "MSBCONFG.OVL", "MSBACKDB.OVL", "MSBACKDR.OVL", "MSBACKFB.OVL", "MSBACKFR.OVL"}, {"MWBACKUP.EXE", "MWBACKUP.HLP", "MWBACKF.DLL", "MWBACKR.DLL", "VFINTD.386"}}
	compAV      = [2][]string{{"MSAV.EXE", "MSAV.HLP", "MSAVHELP.OVL", "MSAVIRUS.LST"}, {"MWAV.EXE", "MWAV.HLP", "MWAVABSI.DLL", "MWAVDLG.DLL", "MWAVDOSL.DLL", "MWAVDRVL.DLL", "MWAVMGR.DLL", "MWAVSCAN.DLL", "MWAVSOS.DLL", "MWAVTSR.EXE"}}
	compUndel   = [2][]string{{"UNDELETE.EXE"}, {"MWUNDEL.EXE", "MWUNDEL.HLP"}}
	compWinTool = []string{"MSTOOLS.DLL", "MWGRAFIC.DLL", "WNTOOLS.GRB"} // used by every Windows tool
	compAVBoth  = []string{"VSAFE.COM", "MSAVIRUS.LST"}
	compDrvSpc  = []string{"DRVSPACE.BIN", "DRVSPACE.EXE", "DRVSPACE.HLP", "DRVSPACE.INF", "DRVSPACE.SYS", "DRVSPACE.TXT", "DBLWIN.HLP", "DBLSPACE.BIN"}
)

// compressed name endings and what EXPAND makes of them, for disks without
// a PACKING.LST
var setupExt = map[string]string{"EX_": "EXE", "CO_": "COM", "SY_": "SYS", "HL_": "HLP", "DL_": "DLL", "OV_": "OVL",
	"38_": "386", "CP_": "CPI", "TX_": "TXT", "IN_": "INF", "PR_": "PRO", "LS_": "LST", "GR_": "GRB", "BI_": "BIN"}

var packLine = regexp.MustCompile(`^\s*(\S+)\s+(\S+)\s*$`)
var packDisk = regexp.MustCompile(`(?i)^\s*Setup Disk #\s*(\d+)`)

// parsePacking reads PACKING.LST: disk number and expanded name of every
// file name on the disks.
func parsePacking(b []byte) (version string, disk map[string]int, expanded map[string]string) {
	disk, expanded = map[string]int{}, map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	cur := 0
	for sc.Scan() {
		l := strings.TrimRight(sc.Text(), "\r")
		if version == "" && strings.TrimSpace(l) != "" {
			version = strings.TrimSpace(l)
		}
		if m := packDisk.FindStringSubmatch(l); m != nil {
			cur, _ = strconv.Atoi(m[1])
			continue
		}
		if cur == 0 {
			continue
		}
		if m := packLine.FindStringSubmatch(l); m != nil && strings.Contains(m[1], ".") && strings.Contains(m[2], ".") {
			n := strings.ToUpper(m[1])
			disk[n], expanded[n] = cur, strings.ToUpper(m[2])
		}
	}
	return
}

// readSetupSource gives the root files of a disk image or a folder.
func readSetupSource(p string) (map[string][]byte, bool, error) {
	files := map[string][]byte{}
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		ents, err := os.ReadDir(p)
		if err != nil {
			return nil, false, err
		}
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			b, err := os.ReadFile(filepath.Join(p, e.Name()))
			if err != nil {
				return nil, false, err
			}
			files[strings.ToUpper(e.Name())] = b
		}
		return files, false, nil
	}
	v, err := OpenImage(p, false)
	if err != nil {
		return nil, false, err
	}
	defer v.Close()
	names, err := v.List("")
	if err != nil {
		return nil, false, err
	}
	for _, n := range names {
		if b, err := v.ReadFile(n); err == nil {
			files[strings.ToUpper(n)] = b
		}
	}
	return files, true, nil
}

// ReadSetupDisks reads and expands MS-DOS Setup disks: disk images or
// folders with the files of the disks, in any order.
func ReadSetupDisks(paths []string) (*SetupDisks, error) {
	s := &SetupDisks{Missing: map[int][]string{}}
	type src struct {
		path  string
		files map[string][]byte
		image bool
	}
	var srcs []src
	var packing []byte
	for _, p := range paths {
		files, image, err := readSetupSource(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", filepath.Base(p), err)
		}
		srcs = append(srcs, src{p, files, image})
		if b := files["PACKING.LST"]; b != nil && packing == nil {
			packing = b
		}
		if files["IO.SYS"] != nil && files["MSDOS.SYS"] != nil && image && s.Boot == "" {
			s.Boot = p
		}
	}
	diskOf, expanded := map[string]int{}, map[string]string{}
	if packing != nil {
		s.Version, diskOf, expanded = parsePacking(packing)
	} else {
		s.Notes = append(s.Notes, T("PACKING.LST was not found: the names of the expanded files are guessed from their endings."))
	}
	seen, found := map[string]bool{}, map[int]bool{}
	var bad []string
	for _, sr := range srcs {
		names := make([]string, 0, len(sr.files))
		for n := range sr.files {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			data := sr.files[n]
			name, ok := expanded[n]
			if !ok {
				name = n
				if i := strings.LastIndexByte(n, '.'); i >= 0 && strings.HasSuffix(n, "_") {
					if e, ok := setupExt[n[i+1:]]; ok {
						name = n[:i+1] + e
					}
				}
			}
			if IsKWAJ(data) {
				out, err := ExpandKWAJ(data)
				if err != nil {
					bad = append(bad, n+" ("+err.Error()+")")
					continue
				}
				data = out
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			s.Files = append(s.Files, setupFile{Name: name, Disk: diskOf[n], Data: data})
			if d := diskOf[n]; d > 0 {
				found[d] = true
			}
		}
	}
	for n, d := range diskOf {
		if !found[d] {
			s.Missing[d] = append(s.Missing[d], expanded[n])
		}
	}
	for d := range found {
		s.Found = append(s.Found, d)
	}
	sort.Ints(s.Found)
	for d := range s.Missing {
		sort.Strings(s.Missing[d])
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf(T("These files cannot be expanded: %s"), strings.Join(bad, ", "))
	}
	if s.Boot == "" {
		return nil, fmt.Errorf("%s", T("Setup disk 1 is missing: it must be a disk image (it has the boot sector, IO.SYS and MSDOS.SYS)."))
	}
	return s, nil
}

// DOSFiles gives the files of C:\DOS for the chosen components, by their
// path on the disk (DOS\HIMEM.SYS ...), and what is missing for them.
func (s *SetupDisks) DOSFiles(o SetupOptions) (map[string][]byte, []string) {
	skip := map[string]bool{}
	for _, n := range setupOnly {
		skip[n] = true
	}
	drop := func(names []string) {
		for _, n := range names {
			skip[n] = true
		}
	}
	keep := func(names []string) {
		for _, n := range names {
			delete(skip, n)
		}
	}
	winTools := false
	for _, c := range []struct {
		choice int
		files  [2][]string
	}{{o.Backup, compBackup}, {o.AntiVirus, compAV}, {o.Undelete, compUndel}} {
		if c.choice&CompDOS == 0 {
			drop(c.files[0])
		}
		if c.choice&CompWindows == 0 {
			drop(c.files[1])
		} else {
			winTools = true
		}
	}
	if o.AntiVirus == CompNone {
		drop(compAVBoth)
	} else {
		keep(compAVBoth)
	}
	if !winTools {
		drop(compWinTool)
	}
	if !o.DriveSpace {
		drop(compDrvSpc)
	}
	out := map[string][]byte{}
	for _, f := range s.Files {
		if !skip[f.Name] && !strings.HasPrefix(f.Name, "DISK.") {
			out[`DOS\`+f.Name] = f.Data
		}
	}
	// the files of missing disks that would have been installed
	var missing []string
	for _, d := range diskNumbers(s.Missing) {
		var names []string
		for _, n := range s.Missing[d] {
			if !skip[n] {
				names = append(names, n)
			}
		}
		if len(names) > 0 {
			missing = append(missing, T("Setup disk %d is missing: %s", d, strings.Join(names, ", ")))
		}
	}
	return out, missing
}

func diskNumbers(m map[int][]string) []int {
	var k []int
	for d := range m {
		k = append(k, d)
	}
	sort.Ints(k)
	return k
}

// SystemFile gives a file of the disks by its expanded name.
func (s *SetupDisks) SystemFile(name string) []byte {
	for _, f := range s.Files {
		if f.Name == name {
			return f.Data
		}
	}
	return nil
}
