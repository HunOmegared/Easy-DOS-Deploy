package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Own files: folders and files from the Windows PC that go onto the DOS
// disk (games, utilities, Norton Commander ...). Long Windows names become
// 8.3 names, as Windows itself would show them to DOS (NORTON~1).

// CopyItem is a file or folder of the PC and the DOS folder it goes into.
type CopyItem struct {
	Source string `json:"source"` // file or folder on the PC
	Dest   string `json:"dest"`   // DOS folder, e.g. C:\GAMES\DOOM
}

// dosChars are the characters an 8.3 name may have besides letters and digits.
const dosChars = "!#$%&'()-@^_`{}~"

func cleanDOS(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r < 128 && strings.ContainsRune(dosChars, r):
			b.WriteRune(r)
		case r == ' ' || r == '.':
			// dropped, as Windows does
		case r < 128:
			b.WriteByte('_')
		default:
			b.WriteRune(foldAccent(r))
		}
	}
	return b.String()
}

// foldAccent turns an accented letter into its plain one (Á -> A), other
// characters into _.
func foldAccent(r rune) rune {
	for base, set := range map[rune]string{'A': "ÁÀÂÄÃÅ", 'E': "ÉÈÊË", 'I': "ÍÌÎÏ", 'O': "ÓÒÔÖŐÕØ", 'U': "ÚÙÛÜŰ", 'C': "Ç", 'N': "Ñ", 'Y': "Ý", 'S': "ŠŚ", 'Z': "ŽŹŻ"} {
		if strings.ContainsRune(set, r) {
			return base
		}
	}
	return '_'
}

// shortNamer gives the 8.3 names of a folder, unique in that folder.
type shortNamer struct{ used map[string]bool }

func newShortNamer() *shortNamer { return &shortNamer{used: map[string]bool{}} }

// name gives the 8.3 name for a Windows name; changed is true when it is
// not the same name.
func (n *shortNamer) name(long string) (string, bool) {
	base, ext := long, ""
	if i := strings.LastIndex(long, "."); i > 0 {
		base, ext = long[:i], long[i+1:]
	}
	b, e := cleanDOS(base), cleanDOS(ext)
	if len(e) > 3 {
		e = e[:3]
	}
	fits := b != "" && len(b) <= 8 && strings.EqualFold(b, base) && strings.EqualFold(e, ext)
	full := func(b string) string {
		if e == "" {
			return b
		}
		return b + "." + e
	}
	if fits && !n.used[full(b)] {
		n.used[full(b)] = true
		return full(b), false
	}
	if b == "" {
		b = "FILE"
	}
	for i := 1; ; i++ {
		tail := fmt.Sprintf("~%d", i)
		stem := b
		if len(stem) > 8-len(tail) {
			stem = stem[:8-len(tail)]
		}
		if c := full(stem + tail); !n.used[c] {
			n.used[c] = true
			return c, true
		}
	}
}

// DefaultDest proposes C:\NAME for a folder or C:\ for a file.
func DefaultDest(source string) string {
	if st, err := os.Stat(source); err == nil && st.IsDir() {
		n, _ := newShortNamer().name(filepath.Base(source))
		return `C:\` + strings.TrimSuffix(n, ".")
	}
	return `C:\`
}

// splitDest gives the drive index (C: = 0) and the folder parts of a DOS path.
func splitDest(dest string) (int, []string, error) {
	d := strings.ToUpper(strings.TrimSpace(dest))
	if len(d) < 2 || d[1] != ':' || d[0] < 'C' || d[0] > 'Z' {
		return 0, nil, fmt.Errorf(T("%s: the destination must start with a hard disk drive, such as C:\\"), dest)
	}
	var parts []string
	for _, p := range strings.Split(d[2:], `\`) {
		if p = strings.TrimSpace(p); p != "" {
			if _, err := To83(p); err != nil {
				return 0, nil, fmt.Errorf("%s: %v", dest, err)
			}
			parts = append(parts, p)
		}
	}
	return int(d[0] - 'C'), parts, nil
}

// fileCopy is one file to write.
type fileCopy struct {
	from  string
	drive int
	to    string // path on the drive
	size  int64
}

// planCopy lists the files of the items with their 8.3 paths.
func planCopy(items []CopyItem) (files []fileCopy, renamed []string, err error) {
	namers := map[string]*shortNamer{} // per DOS folder
	namer := func(drive int, dir string) *shortNamer {
		k := fmt.Sprintf("%d:%s", drive, dir)
		if namers[k] == nil {
			namers[k] = newShortNamer()
		}
		return namers[k]
	}
	for _, it := range items {
		drive, parts, err := splitDest(it.Dest)
		if err != nil {
			return nil, nil, err
		}
		base := strings.Join(parts, `\`)
		st, err := os.Stat(it.Source)
		if err != nil {
			return nil, nil, err
		}
		if !st.IsDir() {
			n, ch := namer(drive, base).name(filepath.Base(it.Source))
			if ch {
				renamed = append(renamed, filepath.Base(it.Source)+" → "+n)
			}
			files = append(files, fileCopy{it.Source, drive, joinDOS(base, n), st.Size()})
			continue
		}
		// a folder: its contents go into the destination folder
		dirs := map[string]string{it.Source: base} // Windows folder -> DOS folder
		err = filepath.WalkDir(it.Source, func(p string, d fs.DirEntry, err error) error {
			if err != nil || p == it.Source {
				return err
			}
			parent := dirs[filepath.Dir(p)]
			n, ch := namer(drive, parent).name(d.Name())
			if ch {
				renamed = append(renamed, d.Name()+" → "+n)
			}
			if d.IsDir() {
				dirs[p] = joinDOS(parent, n)
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			files = append(files, fileCopy{p, drive, joinDOS(parent, n), info.Size()})
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return files, renamed, nil
}

func joinDOS(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + `\` + name
}

// CopySize is the size of the items, for the summary.
func CopySize(items []CopyItem) (int64, int, error) {
	files, _, err := planCopy(items)
	var n int64
	for _, f := range files {
		n += f.size
	}
	return n, len(files), err
}

// openDrives opens the DOS drives of an image in letter order (C: first).
func openDrives(path string) ([]*Volume, func(), error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	var vols []*Volume
	parts := diskParts(f)
	for _, p := range parts {
		if !isFATPartType(p.Type) {
			continue
		}
		v := &Volume{f: f}
		if err := v.load(int64(p.Start) * 512); err == nil {
			vols = append(vols, v)
		}
	}
	if len(parts) == 0 {
		if v, err := openVolume(f); err == nil {
			vols = append(vols, v)
		}
	}
	if len(vols) == 0 {
		f.Close()
		return nil, nil, fmt.Errorf("%s", T("the image has no FAT drive"))
	}
	return vols, func() { f.Sync(); f.Close() }, nil
}

// CopyOwnFiles writes the items onto the drives of an image.
func CopyOwnFiles(path string, items []CopyItem, prog func(done, total int)) (*Report, error) {
	r := &Report{}
	if len(items) == 0 {
		return r, nil
	}
	files, renamed, err := planCopy(items)
	if err != nil {
		return nil, err
	}
	vols, closeAll, err := openDrives(path)
	if err != nil {
		return nil, err
	}
	defer closeAll()
	// enough space on each drive? (clusters, roughly)
	need := map[int]int64{}
	for _, f := range files {
		if f.drive >= len(vols) {
			return nil, fmt.Errorf(T("%s: the disk has no drive %c:"), f.from, 'C'+f.drive)
		}
		cs := int64(vols[f.drive].clusSize())
		need[f.drive] += (f.size + cs - 1) / cs * cs
	}
	drives := make([]int, 0, len(need))
	for d := range need {
		drives = append(drives, d)
	}
	sort.Ints(drives)
	for _, d := range drives {
		if free := vols[d].FreeBytes(); need[d] > free {
			return nil, fmt.Errorf(T("Not enough space on %c: for your files: %s needed, %s free."), 'C'+d, humanSize(need[d]), humanSize(free))
		}
	}
	var total int64
	for i, f := range files {
		data, err := os.ReadFile(f.from)
		if err != nil {
			return nil, err
		}
		if err := vols[f.drive].WriteFile(f.to, data); err != nil {
			return nil, fmt.Errorf("%c:\\%s: %v", 'C'+f.drive, f.to, err)
		}
		total += f.size
		if prog != nil {
			prog(i+1, len(files))
		}
	}
	r.Done = append(r.Done, T("Your files: %d files, %s.", len(files), humanSize(total)))
	if len(renamed) > 0 {
		show := renamed
		if len(show) > 8 {
			show = append(append([]string{}, show[:8]...), "...")
		}
		r.Done = append(r.Done, T("Long names shortened for DOS (%d): %s", len(renamed), strings.Join(show, ", ")))
	}
	return r, nil
}
