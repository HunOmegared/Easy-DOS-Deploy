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
	// a partial German file
	de := "@name = Deutsch\n\nEN: &File\nTR: &Datei\n\nEN: Volume: %s\nTR: Laufwerk: %s\n\nEN: OK\nTR: \n"
	os.WriteFile(filepath.Join(dir, "de.lng"), []byte("\ufeff"+de), 0644)
	if errs := LoadLanguages(dir); len(errs) > 0 {
		t.Fatal(errs)
	}
	defer SetLanguage("hu")
	if !SetLanguage("de") || T("&File") != "&Datei" || T("Volume: %s", "C:") != "Laufwerk: C:" || T("OK") != "OK" || T("Close") != "Close" {
		t.Fatal(T("&File"), T("Volume: %s", "C:"))
	}
	found := false
	for _, l := range LanguageList() {
		if l.Name == "Deutsch" {
			found = true
		}
	}
	if !found {
		t.Fatal("de not listed")
	}
	os.WriteFile(filepath.Join(dir, "bad.lng"), []byte("TR: x\n"), 0644)
	if errs := LoadLanguages(dir); len(errs) != 1 {
		t.Fatal("bad file accepted")
	}
}

func TestEnglishOutput(t *testing.T) {
	SetLanguage("en")
	defer SetLanguage("hu")
	p := Templates[0].Make()
	o, err := Generate(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(o.Config, "\n"), "MENUITEM=PLAIN, DOS only") {
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
