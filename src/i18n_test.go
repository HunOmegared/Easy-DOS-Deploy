package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// the older tests check Hungarian texts
	SetLanguage("hu")
	os.Exit(m.Run())
}

var reKey = regexp.MustCompile("\\b[TN]\\(\\s*(\"(?:[^\"\\\\]|\\\\.)*\"|`[^`]*`)")

func sourceKeys(t *testing.T) []string {
	files, _ := filepath.Glob("*.go")
	var keys []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "i18n_hu.go" || f == "i18n.go" {
			continue
		}
		b, _ := os.ReadFile(f)
		for _, m := range reKey.FindAllStringSubmatch(string(b), -1) {
			lit := m[1]
			var s string
			if lit[0] == '`' {
				s = lit[1 : len(lit)-1]
			} else {
				var err error
				if s, err = strconv.Unquote(lit); err != nil {
					t.Fatalf("%s: %v", lit, err)
				}
			}
			keys = append(keys, s)
		}
	}
	return keys
}

var rePlace = regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z%]`)

func TestEveryTextTranslated(t *testing.T) {
	keys := sourceKeys(t)
	if len(keys) < 200 {
		t.Fatalf("only %d keys found", len(keys))
	}
	used := map[string]bool{}
	for _, k := range keys {
		used[k] = true
		tr, ok := huTexts[k]
		if !ok {
			t.Errorf("not translated: %q", k)
			continue
		}
		a := strings.Join(rePlace.FindAllString(k, -1), " ")
		b := strings.Join(rePlace.FindAllString(tr, -1), " ")
		if a != b {
			t.Errorf("placeholders differ in %q: %q vs %q", k, a, b)
		}
	}
	for k := range huTexts {
		if !used[k] {
			t.Errorf("unused translation: %q", k)
		}
	}
}

func TestLanguageFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hu.lng")
	if err := WriteLanguageTemplate(p, "hu"); err != nil {
		t.Fatal(err)
	}
	l, err := LoadLanguageFile(p)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range huTexts {
		if l.Texts[k] != v {
			t.Fatalf("%q: %q vs %q", k, l.Texts[k], v)
		}
	}
	// a partial file of a new language
	nl := "@name = Nederlands\n\nEN: &File\nTR: &Bestand\n\nEN: Volume: %s\nTR: Volume: %s!\n\nEN: OK\nTR: \n"
	os.WriteFile(filepath.Join(dir, "nl.lng"), []byte("\ufeff"+nl), 0644)
	if errs := LoadLanguages(dir); len(errs) > 0 {
		t.Fatal(errs)
	}
	defer SetLanguage("hu")
	if !SetLanguage("nl") || T("&File") != "&Bestand" || T("Volume: %s", "C:") != "Volume: C:!" || T("OK") != "OK" || T("Close") != "Close" {
		t.Fatal(T("&File"), T("Volume: %s", "C:"))
	}
	// a file for a built-in language overrides some of its texts only
	os.WriteFile(filepath.Join(dir, "de.lng"), []byte("EN: OK\nTR: Gut\n"), 0644)
	if errs := LoadLanguages(dir); len(errs) > 0 {
		t.Fatal(errs)
	}
	if !SetLanguage("de") || T("OK") != "Gut" || T("&File") != "&Datei" || T("Close") != "Schließen" {
		t.Fatal(T("OK"), T("&File"), T("Close"))
	}
	found := false
	for _, l := range LanguageList() {
		if l.Name == "Nederlands" {
			found = true
		}
	}
	if !found {
		t.Fatal("nl not listed")
	}
	os.WriteFile(filepath.Join(dir, "bad.lng"), []byte("TR: x\n"), 0644)
	if errs := LoadLanguages(dir); len(errs) != 1 {
		t.Fatal("bad file accepted")
	}
}

func TestEnglishOutput(t *testing.T) {
	SetLanguage("en")
	defer SetLanguage("hu")
	p := TemplateByID("net-ms").Make()
	o, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(o.Config, "\n"), "MENUITEM=NET, DOS + network + Easy Commander") {
		t.Fatal(o.Config)
	}
	p.Items = nil
	if ps := p.Validate(); ps[0].Msg != "There are no menu items." {
		t.Fatal(ps)
	}
}

func TestCatalogEditing(t *testing.T) {
	s := NewStore(t.TempDir())
	if s.Custom {
		t.Fatal("custom at start")
	}
	files, _ := ParseFiles("rtspkt.com = drivers\\rtspkt.com\r\n  pcntpk.com\r\n", nil)
	d := Driver{ID: "realtek", Name: "Realtek 8139", Files: files, URLs: []string{"https://example.org/rtspkt.zip"}}
	if err := s.CheckDriver(&d, -1); err != nil {
		t.Fatal(err)
	}
	if files[0].Name != "RTSPKT.COM" || files[0].Dest != `DRIVERS\RTSPKT.COM` || files[1].Dest != `DRIVERS\PCNTPK.COM` {
		t.Fatal(files)
	}
	s.Catalog = append(s.Catalog, d)
	if err := s.SaveCatalog(); err != nil {
		t.Fatal(err)
	}
	s2 := NewStore(s.Dir)
	if !s2.Custom || s2.Find("realtek") == nil {
		t.Fatal("not saved")
	}
	// generated files survive editing
	m := s2.Find("mtcpcfg")
	got, _ := ParseFiles(FormatFiles(m.Files), m.Files)
	if len(got) != 1 || got[0].Content == "" {
		t.Fatal(got)
	}
	bad := []Driver{
		{ID: "Bad ID", Name: "x", Files: files},
		{ID: "ne2000", Name: "dup", Files: files},
		{ID: "ok", Name: "", Files: files},
		{ID: "ok", Name: "x"},
		{ID: "ok", Name: "x", Files: []DriverFile{{Name: "TOOLONGNAME.COM", Dest: "X.COM"}}},
		{ID: "ok", Name: "x", Files: []DriverFile{{Name: "A.COM", Dest: `BAD DIR\A.COM`}}},
		{ID: "ok", Name: "x", Files: files, URLs: []string{"ftp://x"}},
	}
	for i, b := range bad {
		if s2.CheckDriver(&b, -1) == nil {
			t.Fatalf("bad %d accepted", i)
		}
	}
	if err := s2.ResetCatalog(); err != nil || s2.Custom || s2.Find("realtek") != nil {
		t.Fatal("reset")
	}
	os.WriteFile(filepath.Join(s.Dir, catalogFile), []byte("{broken"), 0644)
	if err := s2.Reload(); err == nil || s2.Find("ne2000") == nil {
		t.Fatal("broken json")
	}
}

// The built-in language files have every text, with the same placeholders,
// and no text that the program does not use.
func TestBuiltinLanguages(t *testing.T) {
	for _, code := range []string{"de", "fr", "es", "it"} {
		l := builtin[code]
		if l == nil || l.Name == code {
			t.Fatalf("%s is not built in", code)
		}
		for k := range huTexts {
			tr, ok := l.Texts[k]
			if !ok || tr == "" {
				t.Errorf("%s: not translated: %q", code, k)
				continue
			}
			a := strings.Join(rePlace.FindAllString(k, -1), " ")
			b := strings.Join(rePlace.FindAllString(tr, -1), " ")
			if a != b {
				t.Errorf("%s: placeholders differ in %q: %q vs %q", code, k, a, b)
			}
			if strings.Contains(k, "&") != strings.Contains(tr, "&") {
				t.Errorf("%s: menu key (&) missing or extra in %q: %q", code, k, tr)
			}
		}
		for k := range l.Texts {
			if _, ok := huTexts[k]; !ok {
				t.Errorf("%s: unused translation: %q", code, k)
			}
		}
	}
}

// Menu items of one menu need different keys (the letter after &).
func TestMenuKeys(t *testing.T) {
	menus := [][]string{
		{"&File", "&Import", "&Output", "Dis&k image", "&Drivers", "&Language", "&Help"},
		{"Start &wizard...", "&New from template", "&Open project...", "&Save project", "Save project &as...", "E&xit"},
		{"Existing menu from &disk image...", "Existing menu from &folder..."},
		{"&Preview", "Save to &folder...", "Write to disk &image..."},
		{"&New bootable disk image...", "&Back up a card to an image file...", "&Write an image to a card...",
			"Save &GEOM.COM (shows the geometry on the target machine)...", "Put GEOM.COM onto a boot &floppy image...",
			"&Make a FreeDOS floppy with GEOM.COM..."},
		{"&Manage drivers...", "Open driver &folder"},
		{"&Make a translation file...", "&Reload language files", "Open language &folder"},
		{"&Yes", "&No"},
	}
	defer SetLanguage("hu")
	for _, l := range LanguageList() {
		SetLanguage(l.Code)
		for _, m := range menus {
			seen := map[rune]string{}
			for _, k := range m {
				s := []rune(strings.ToLower(T(k)))
				for i, r := range s {
					if r == '&' && i+1 < len(s) {
						if o, ok := seen[s[i+1]]; ok {
							t.Errorf("%s: %q and %q use the same key", l.Code, o, T(k))
						}
						seen[s[i+1]] = T(k)
						break
					}
				}
			}
		}
	}
}
