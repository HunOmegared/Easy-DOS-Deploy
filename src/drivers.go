package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DriverFile is one file a driver package puts on the DOS disk.
type DriverFile struct {
	// Name is the file name looked for inside the downloaded zip (in any
	// folder, case-insensitively) or the name of the downloaded file itself.
	Name string `json:"name"`
	// Dest is the path on the DOS disk, e.g. DRIVERS\NE2000.COM.
	Dest string `json:"dest"`
	// Content, when set, is written as-is (a generated text file); no download.
	Content string `json:"content,omitempty"`
	// KeepExisting: do not overwrite the file if the disk already has it
	// (for configuration files the user may have edited).
	KeepExisting bool `json:"keepexisting,omitempty"`
}

// Driver is a catalog entry.
type Driver struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Desc  string       `json:"desc"`
	URLs  []string     `json:"urls"` // tried in order; zip or plain file
	Info  string       `json:"info"` // web page for manual download
	Files []DriverFile `json:"files"`
}

const fdRepo = "https://www.ibiblio.org/pub/micro/pc-stuff/freedos/files/repositories/1.3/"
const fdMirror = "https://ftp.icm.edu.pl/packages/freedos/repositories/1.3/"

func fdURLs(p string) []string { return []string{fdRepo + p, fdMirror + p} }

// DefaultCatalog lists freely distributable drivers. The download addresses
// are the FreeDOS 1.3 package repository (a frozen release, so the paths do
// not move) plus a mirror. If a download fails the program tells the user
// where to get the file and lets them browse for it.
func DefaultCatalog() []Driver {
	return []Driver{
		{ID: "ne2000", Name: T("NE2000 packet driver (Crynwr)"), Desc: T("For the default network card of 86Box/PCem and real NE2000 cards."),
			URLs: fdURLs("net/crynwr.zip"), Info: "http://crynwr.com/drivers/",
			Files: []DriverFile{{Name: "NE2000.COM", Dest: `DRIVERS\NE2000.COM`}}},
		{ID: "3c509", Name: T("3Com 3C509 packet driver (Crynwr)"), Desc: T("For 3Com EtherLink III cards."),
			URLs: fdURLs("net/crynwr.zip"), Info: "http://crynwr.com/drivers/",
			Files: []DriverFile{{Name: "3C509.COM", Dest: `DRIVERS\3C509.COM`}}},
		{ID: "mtcp", Name: T("mTCP (DHCP, PING, FTP, HTGET)"), Desc: T("TCP/IP programs by Michael Brutman; Easy Commander needs DHCP."),
			URLs: fdURLs("net/mtcp.zip"), Info: "http://www.brutman.com/mTCP/",
			Files: []DriverFile{
				{Name: "DHCP.EXE", Dest: `MTCP\DHCP.EXE`},
				{Name: "PING.EXE", Dest: `MTCP\PING.EXE`},
				{Name: "FTP.EXE", Dest: `MTCP\FTP.EXE`},
				{Name: "HTGET.EXE", Dest: `MTCP\HTGET.EXE`},
			}},
		{ID: "mtcpcfg", Name: T("MTCP.CFG sample"), Desc: T("Basic mTCP setup (packet driver at 0x60, DHCP fills in the rest). An existing file is not overwritten."),
			Files: []DriverFile{{Name: "MTCP.CFG", Dest: `MTCP\MTCP.CFG`, KeepExisting: true,
				Content: "PACKETINT 0x60\r\nHOSTNAME RETROPC\r\n"}}},
		{ID: "ec", Name: T("Easy Commander client (EC.EXE)"), Desc: T("Browse for it from your own release; it has no download address."),
			Files: []DriverFile{{Name: "EC.EXE", Dest: `EC\EC.EXE`}}},
		{ID: "ctmouse", Name: T("CuteMouse mouse driver"), Desc: T("PS/2 and serial mouse; small and free."),
			URLs: fdURLs("base/ctmouse.zip"), Info: "https://cutemouse.sourceforge.net/",
			Files: []DriverFile{{Name: "CTMOUSE.EXE", Dest: `DRIVERS\CTMOUSE.EXE`}}},
		{ID: "jemm", Name: T("JemmEx / Jemm386 memory manager"), Desc: T("Replaces HIMEM+EMM386, also works with MS-DOS (386+)."),
			URLs: append([]string{"https://github.com/Baron-von-Riedesel/Jemm/releases/download/v5.86/JemmB_v586.zip"}, fdURLs("base/jemm.zip")...), Info: "https://github.com/Baron-von-Riedesel/Jemm",
			Files: []DriverFile{{Name: "JEMMEX.EXE", Dest: `DRIVERS\JEMMEX.EXE`}, {Name: "JEMM386.EXE", Dest: `DRIVERS\JEMM386.EXE`}}},
		{ID: "himemx", Name: T("HimemX XMS manager"), Desc: T("Free HIMEM.SYS replacement."),
			URLs: fdURLs("base/himemx.zip"), Info: "https://github.com/Baron-von-Riedesel/HimemX",
			Files: []DriverFile{{Name: "HIMEMX.EXE", Dest: `DRIVERS\HIMEMX.EXE`}}},
		{ID: "oakcdrom", Name: T("OAKCDROM.SYS generic ATAPI CD-ROM driver"), Desc: T("Works with nearly every IDE/ATAPI CD drive, also on old ISA controllers. Part of Windows 98, not free: browse for it on your Windows 98 boot disk (or its image), or in WINDOWS\\COMMAND\\EBD."),
			Files: []DriverFile{{Name: "OAKCDROM.SYS", Dest: `DRIVERS\OAKCDROM.SYS`}}},
		{ID: "unisound", Name: T("UNISOUND ISA PnP sound card driver"), Desc: T("Freeware by JazeFox: sets up ISA PnP sound cards (ESS, Creative, Yamaha, Crystal, Aztech, OPTi...) and their mixer. Download it from the Vogons forum topic and browse for UNISOUND.COM or the zip."),
			Info:  "https://www.vogons.org/viewtopic.php?p=833659",
			Files: []DriverFile{{Name: "UNISOUND.COM", Dest: `DRIVERS\UNISOUND.COM`}}},
		{ID: "udvd2", Name: T("UDVD2 CD/DVD driver"), Desc: T("For IDE/ATAPI CD drives (CONFIG.SYS DEVICE= line)."),
			URLs: fdURLs("drivers/udvd2.zip"), Info: "https://www.ibiblio.org/pub/micro/pc-stuff/freedos/files/repositories/1.3/",
			Files: []DriverFile{{Name: "UDVD2.SYS", Dest: `DRIVERS\UDVD2.SYS`}}},
		{ID: "shsucdx", Name: T("SHSUCDX CD-ROM extension"), Desc: T("Free MSCDEX replacement."),
			URLs: fdURLs("base/shsucdx.zip"), Info: "https://www.ibiblio.org/pub/micro/pc-stuff/freedos/files/repositories/1.3/",
			Files: []DriverFile{{Name: "SHSUCDX.COM", Dest: `DRIVERS\SHSUCDX.COM`}}},
		{ID: fdKernelID, Name: T("FreeDOS kernel (KERNEL.SYS)"), Desc: T("The FreeDOS system: kernel 2043, for 8086 and up, FAT12 and FAT16. Used for FreeDOS disks made without a system source."),
			URLs: []string{"https://github.com/FDOS/kernel/releases/download/ke2043/ke2043_86f16.zip"}, Info: "https://github.com/FDOS/kernel",
			Files: []DriverFile{{Name: "KERNEL.SYS", Dest: "KERNEL.SYS"}}},
		{ID: fdCommandID, Name: T("FreeCOM command shell (COMMAND.COM)"), Desc: T("The FreeDOS command shell, FreeCOM 0.86. Used for FreeDOS disks made without a system source."),
			URLs: []string{"https://github.com/FDOS/freecom/releases/download/com086/command.zip"}, Info: "https://github.com/FDOS/freecom",
			Files: []DriverFile{{Name: "COMMAND.COM", Dest: `FREEDOS\BIN\COMMAND.COM`}}},
	}
}

// Store keeps downloaded/browsed driver files in a folder next to the program.
type Store struct {
	Dir     string // e.g. <exe dir>\drivers
	Catalog []Driver
	// Custom is true when the catalog comes from drivers.json (edited by
	// the user); otherwise it is the built-in one in the current language.
	Custom bool
}

const catalogFile = "drivers.json"

func NewStore(dir string) *Store {
	s := &Store{Dir: dir}
	s.Reload()
	return s
}

func (s *Store) catalogPath() string { return filepath.Join(s.Dir, catalogFile) }

// Reload reads drivers.json, or uses the built-in catalog.
func (s *Store) Reload() error {
	s.Catalog, s.Custom = DefaultCatalog(), false
	b, err := os.ReadFile(s.catalogPath())
	if err != nil {
		return nil
	}
	var c []Driver
	if err := json.Unmarshal(b, &c); err != nil {
		return fmt.Errorf(T("%s is damaged, the built-in list is used: %v"), s.catalogPath(), err)
	}
	s.Catalog, s.Custom = c, true
	// entries added to the built-in list in newer versions
	for _, d := range DefaultCatalog() {
		if s.Find(d.ID) == nil {
			s.Catalog = append(s.Catalog, d)
		}
	}
	return nil
}

// SaveCatalog writes the catalog to drivers.json.
func (s *Store) SaveCatalog() error {
	if err := os.MkdirAll(s.Dir, 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s.Catalog, "", "  ")
	if err := os.WriteFile(s.catalogPath(), b, 0644); err != nil {
		return err
	}
	s.Custom = true
	return nil
}

// ResetCatalog removes drivers.json and returns to the built-in list.
// Downloaded files are kept.
func (s *Store) ResetCatalog() error {
	if err := os.Remove(s.catalogPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return s.Reload()
}

var reDriverID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// CheckDriver validates an entry before it is put into the catalog. index
// is the entry's position when editing, -1 for a new one.
func (s *Store) CheckDriver(d *Driver, index int) error {
	if !reDriverID.MatchString(d.ID) {
		return fmt.Errorf("%s", T("The ID must be 1-32 lower-case letters, digits, - or _."))
	}
	for i, x := range s.Catalog {
		if i != index && x.ID == d.ID {
			return fmt.Errorf(T("The ID %s is already in the list."), d.ID)
		}
	}
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("%s", T("The name is empty."))
	}
	if len(d.Files) == 0 {
		return fmt.Errorf("%s", T("Give at least one file."))
	}
	for _, f := range d.Files {
		if _, err := To83(f.Name); err != nil {
			return fmt.Errorf(T("File name %s: %v"), f.Name, err)
		}
		parts := splitPath(f.Dest)
		if len(parts) == 0 {
			return fmt.Errorf(T("No place on the disk for %s."), f.Name)
		}
		for _, p := range parts {
			if _, err := To83(p); err != nil {
				return fmt.Errorf(T("Place on the disk %s: %v"), f.Dest, err)
			}
		}
	}
	for _, u := range d.URLs {
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			return fmt.Errorf(T("Not a web address: %s"), u)
		}
	}
	return nil
}

// FormatFiles / ParseFiles convert the file list to and from editable
// lines of the form  NAME.EXT = FOLDER\NAME.EXT
func FormatFiles(fs []DriverFile) string {
	var out []string
	for _, f := range fs {
		if f.Content == "" {
			out = append(out, f.Name+" = "+f.Dest)
		}
	}
	return strings.Join(out, "\r\n")
}

func ParseFiles(text string, keep []DriverFile) ([]DriverFile, error) {
	var out []DriverFile
	// generated files cannot be edited as lines; keep them
	for _, f := range keep {
		if f.Content != "" {
			out = append(out, f)
		}
	}
	for _, l := range SplitLines(text) {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		name, dest := l, ""
		if i := strings.Index(l, "="); i >= 0 {
			name, dest = strings.TrimSpace(l[:i]), strings.TrimSpace(l[i+1:])
		}
		name = strings.ToUpper(name)
		if dest == "" {
			dest = `DRIVERS\` + name
		}
		dest = strings.ToUpper(strings.TrimPrefix(strings.TrimPrefix(dest, "C:"), `\`))
		out = append(out, DriverFile{Name: name, Dest: dest})
	}
	return out, nil
}

func (s *Store) Find(id string) *Driver {
	for i := range s.Catalog {
		if s.Catalog[i].ID == id {
			return &s.Catalog[i]
		}
	}
	return nil
}

func (s *Store) DriverDir(id string) string { return filepath.Join(s.Dir, id) }

func (s *Store) FilePath(id, name string) string {
	return filepath.Join(s.DriverDir(id), strings.ToUpper(name))
}

// Missing returns the files of d not yet in the store. A file that is not
// what it should be (an MS-DOS COMMAND.COM in place of FreeCOM) is set aside
// as .BAD and counts as missing.
func (s *Store) Missing(d *Driver) []DriverFile {
	var out []DriverFile
	for _, f := range d.Files {
		if f.Content != "" {
			continue
		}
		p := s.FilePath(d.ID, f.Name)
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			out = append(out, f)
			continue
		}
		if check := fileChecks[d.ID+"/"+strings.ToUpper(f.Name)]; check != nil {
			if b, err := os.ReadFile(p); err != nil || check(b) != nil {
				os.Rename(p, p+".BAD")
				out = append(out, f)
			}
		}
	}
	return out
}

// fileChecks tell the right file from a look-alike, by catalog ID and name.
var fileChecks = map[string]func([]byte) error{
	fdCommandID + "/COMMAND.COM": func(b []byte) error {
		if !bytes.Contains(bytes.ToUpper(b), []byte("FREECOM")) {
			return fmt.Errorf("%s", T("this COMMAND.COM is not FreeCOM (the FreeDOS command shell)"))
		}
		return nil
	},
}

// putFile stores one file of a driver, if it is the right one.
func (s *Store) putFile(d *Driver, name string, data []byte) error {
	if check := fileChecks[d.ID+"/"+strings.ToUpper(name)]; check != nil {
		if err := check(data); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(s.DriverDir(d.ID), 0755); err != nil {
		return err
	}
	return os.WriteFile(s.FilePath(d.ID, name), data, 0644)
}

func (s *Store) Ready(d *Driver) bool { return len(s.Missing(d)) == 0 }

// Data returns the bytes for one file of a driver.
func (s *Store) Data(d *Driver, f DriverFile) ([]byte, error) {
	if f.Content != "" {
		return []byte(f.Content), nil
	}
	return os.ReadFile(s.FilePath(d.ID, f.Name))
}

// AddFile copies a file the user browsed to into the store. It is matched
// to the driver file of the same name; if the driver has only one file the
// name does not have to match. A zip is searched for the needed files.
func (s *Store) AddFile(d *Driver, src string) ([]string, error) {
	b, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(filepath.Ext(src), ".zip") {
		return s.extract(d, b)
	}
	if isImageName(src) {
		return s.extractFromImage(d, src)
	}
	base := strings.ToUpper(filepath.Base(src))
	var target *DriverFile
	var need []DriverFile
	for i, f := range d.Files {
		if f.Content != "" {
			continue
		}
		need = append(need, f)
		if strings.ToUpper(f.Name) == base {
			target = &d.Files[i]
		}
	}
	if target == nil && len(need) == 1 {
		target = &need[0]
	}
	if target == nil {
		names := make([]string, len(need))
		for i, f := range need {
			names[i] = f.Name
		}
		return nil, fmt.Errorf(T("%s does not belong to this driver; expected: %s"), base, strings.Join(names, ", "))
	}
	if err := os.MkdirAll(s.DriverDir(d.ID), 0755); err != nil {
		return nil, err
	}
	if err := s.putFile(d, target.Name, b); err != nil {
		return nil, err
	}
	return []string{target.Name}, nil
}

// zipEntry is a file found in a zip, possibly inside a nested zip.
type zipEntry struct {
	path string
	open func() ([]byte, error)
}

func listZip(zb []byte, prefix string, depth int) ([]zipEntry, error) {
	zr, err := zip.NewReader(bytes.NewReader(zb), int64(len(zb)))
	if err != nil {
		return nil, err
	}
	var out []zipEntry
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		zf := zf
		read := func() ([]byte, error) {
			rc, err := zf.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, 64<<20))
		}
		out = append(out, zipEntry{prefix + zf.Name, read})
		// FreeDOS packages sometimes keep the original archives inside.
		if depth < 2 && strings.EqualFold(path.Ext(zf.Name), ".zip") && zf.UncompressedSize64 < 32<<20 {
			if inner, err := read(); err == nil {
				if sub, err := listZip(inner, prefix+zf.Name+"/", depth+1); err == nil {
					out = append(out, sub...)
				}
			}
		}
	}
	return out, nil
}

func (s *Store) extract(d *Driver, zb []byte) ([]string, error) {
	entries, err := listZip(zb, "", 0)
	if err != nil {
		return nil, fmt.Errorf(T("not a zip file: %v"), err)
	}
	var got []string
	for _, f := range d.Files {
		if f.Content != "" {
			continue
		}
		var best *zipEntry
		for i, e := range entries {
			if !strings.EqualFold(path.Base(e.path), f.Name) {
				continue
			}
			// prefer a binary folder over source or documentation folders, and
			// a direct entry over one inside a nested zip
			if best == nil || (strings.Contains(strings.ToUpper(e.path), "BIN/") && !strings.Contains(strings.ToUpper(best.path), "BIN/")) {
				best = &entries[i]
			}
		}
		if best == nil {
			continue
		}
		data, err := best.open()
		if err != nil {
			return got, err
		}
		if err := os.MkdirAll(s.DriverDir(d.ID), 0755); err != nil {
			return got, err
		}
		if err := s.putFile(d, f.Name, data); err != nil {
			return got, err
		}
		got = append(got, f.Name)
	}
	if len(got) == 0 {
		return nil, fmt.Errorf("%s", T("the zip does not contain the needed files"))
	}
	return got, nil
}

// DownloadResult describes one driver's download attempt.
type DownloadResult struct {
	ID      string
	OK      bool
	Message string
}

var httpClient = &http.Client{Timeout: 90 * time.Second}

// zip cache so that drivers sharing an archive (crynwr) download it once
func (s *Store) fetch(url string) ([]byte, error) {
	cache := filepath.Join(s.Dir, "_letoltes", path.Base(url))
	if b, err := os.ReadFile(cache); err == nil && len(b) > 0 {
		return b, nil
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "EasyDOSDeploy/0.8")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 200<<20))
	if err != nil {
		return nil, err
	}
	if os.MkdirAll(filepath.Dir(cache), 0755) == nil {
		os.WriteFile(cache, b, 0644)
	}
	return b, nil
}

// Download tries each URL of d until all files are in the store.
func (s *Store) Download(d *Driver) DownloadResult {
	r := DownloadResult{ID: d.ID}
	if len(s.Missing(d)) == 0 {
		r.OK, r.Message = true, T("already here")
		return r
	}
	if len(d.URLs) == 0 {
		r.Message = T("no download address; browse for it")
		return r
	}
	var errs []string
	for _, u := range d.URLs {
		b, err := s.fetch(u)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", u, err))
			continue
		}
		if strings.EqualFold(path.Ext(u), ".zip") {
			_, err = s.extract(d, b)
		} else if len(d.Files) == 1 {
			os.MkdirAll(s.DriverDir(d.ID), 0755)
			err = s.putFile(d, d.Files[0].Name, b)
		}
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", u, err))
			continue
		}
		if len(s.Missing(d)) == 0 {
			r.OK, r.Message = true, T("downloaded")
			return r
		}
	}
	miss := s.Missing(d)
	names := make([]string, len(miss))
	for i, f := range miss {
		names[i] = f.Name
	}
	msg := T("failed")
	if len(errs) > 0 {
		msg += " (" + errs[len(errs)-1] + ")"
	}
	where := d.Info
	if where == "" && len(d.URLs) > 0 {
		where = d.URLs[0]
	}
	msg += T(". Missing: %s. Download it yourself from %s and browse for it, or put it here: %s",
		strings.Join(names, ", "), where, s.DriverDir(d.ID))
	r.Message = msg
	return r
}

func isImageName(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".img", ".ima", ".vhd", ".hdd", ".dsk", ".vfd", ".flp":
		return true
	}
	return false
}

// extractFromImage looks for the driver's files on a DOS disk image (for
// example OAKCDROM.SYS on a Windows 98 boot floppy), in any folder.
func (s *Store) extractFromImage(d *Driver, src string) ([]string, error) {
	v, err := OpenImage(src, false)
	if err != nil {
		return nil, err
	}
	defer v.Close()
	found := map[string]string{} // upper-case name -> path on the image
	err = v.Walk(func(fi FileInfo) error {
		if fi.Dir {
			return nil
		}
		name := fi.Path
		if i := strings.LastIndex(name, `\`); i >= 0 {
			name = name[i+1:]
		}
		// prefer the shortest path (root over subfolders)
		if old, ok := found[name]; !ok || len(fi.Path) < len(old) {
			found[name] = fi.Path
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var got []string
	for _, f := range d.Files {
		if f.Content != "" {
			continue
		}
		p, ok := found[strings.ToUpper(f.Name)]
		if !ok {
			continue
		}
		data, err := v.ReadFile(p)
		if err != nil {
			return got, err
		}
		if err := os.MkdirAll(s.DriverDir(d.ID), 0755); err != nil {
			return got, err
		}
		if err := s.putFile(d, f.Name, data); err != nil {
			return got, err
		}
		got = append(got, f.Name)
	}
	if len(got) == 0 {
		return nil, fmt.Errorf("%s", T("the disk image does not contain the needed files"))
	}
	return got, nil
}

// driverName is the name of a catalog entry, or its ID if it is not there.
func (s *Store) driverName(id string) string {
	if d := s.Find(id); d != nil {
		return d.Name
	}
	return id
}
