package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Translation works gettext-style: the English text is the key. T looks it
// up in the current language and falls back to English. Language files are
// plain UTF-8 text in the "lang" folder, for example lang\de.lng:
//
//	@name = Deutsch
//
//	EN: Save to folder...
//	TR: In Ordner speichern...
//
// "\n" inside a text stands for a line break. Placeholders such as %s and %d
// must stay in the translation in the same order.

type Language struct {
	Code  string
	Name  string
	Texts map[string]string // English -> translated; nil for English
}

var (
	languages = map[string]*Language{
		"en": {Code: "en", Name: "English"},
		"hu": {Code: "hu", Name: "Magyar", Texts: huTexts},
	}
	curLang = languages["en"]
)

// T translates s and, when args are given, formats it like fmt.Sprintf.
func T(s string, args ...any) string {
	if curLang.Texts != nil {
		if t, ok := curLang.Texts[s]; ok && t != "" {
			s = t
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}

func SetLanguage(code string) bool {
	if l, ok := languages[code]; ok {
		curLang = l
		return true
	}
	return false
}

func CurrentLanguage() string { return curLang.Code }

// LanguageList returns the available languages sorted by name.
func LanguageList() []*Language {
	var out []*Language
	for _, l := range languages {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func unescape(s string) string {
	return strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\\`, `\`).Replace(s)
}

func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`).Replace(s)
}

// LoadLanguageFile reads one .lng file. A file for a built-in language
// overrides or completes its texts.
func LoadLanguageFile(path string) (*Language, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	code := strings.ToLower(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	l := &Language{Code: code, Name: code, Texts: map[string]string{}}
	if base, ok := languages[code]; ok && base.Texts != nil {
		for k, v := range base.Texts {
			l.Texts[k] = v
		}
		l.Name = base.Name
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	key := ""
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimRight(sc.Text(), "\r")
		if n == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		t := strings.TrimSpace(line)
		switch {
		case t == "" || strings.HasPrefix(t, "#"):
		case strings.HasPrefix(t, "@name"):
			if i := strings.Index(t, "="); i >= 0 {
				l.Name = strings.TrimSpace(t[i+1:])
			}
		case strings.HasPrefix(line, "EN:"):
			key = unescape(strings.TrimPrefix(strings.TrimPrefix(line, "EN:"), " "))
		case strings.HasPrefix(line, "TR:"):
			if key == "" {
				return nil, fmt.Errorf("%s:%d: TR: without EN:", path, n)
			}
			if v := unescape(strings.TrimPrefix(strings.TrimPrefix(line, "TR:"), " ")); v != "" {
				l.Texts[key] = v
			}
			key = ""
		default:
			return nil, fmt.Errorf("%s:%d: unknown line", path, n)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if code == "en" {
		return nil, fmt.Errorf("en is the built-in source language")
	}
	return l, nil
}

// LoadLanguages loads every *.lng file in dir; errors are returned but do
// not stop the other files.
func LoadLanguages(dir string) []error {
	var errs []error
	files, _ := filepath.Glob(filepath.Join(dir, "*.lng"))
	for _, p := range files {
		l, err := LoadLanguageFile(p)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		languages[l.Code] = l
	}
	return errs
}

// AllKeys lists every translatable text (the Hungarian table is complete;
// a test checks that every T("...") in the code is in it).
func AllKeys() []string {
	var out []string
	for k := range huTexts {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// WriteLanguageTemplate writes a file a translator can fill in. If base is
// a language code, its translations are pre-filled (handy for corrections).
func WriteLanguageTemplate(path, base string) error {
	var b strings.Builder
	b.WriteString("# Easy Retro System Builder translation file (UTF-8).\n")
	b.WriteString("# Name this file <language code>.lng (for example de.lng) and put it into the \"lang\" folder.\n")
	b.WriteString("# Fill in the TR: lines. Keep %s, %d and similar placeholders in the same order. \\n is a line break.\n")
	b.WriteString("# An empty TR: line leaves that text in English.\n\n")
	name := "Your language name"
	src := languages[base]
	if src != nil && src.Texts != nil {
		name = src.Name
	}
	b.WriteString("@name = " + name + "\n\n")
	for _, k := range AllKeys() {
		tr := ""
		if src != nil && src.Texts != nil {
			tr = src.Texts[k]
		}
		b.WriteString("EN: " + escape(k) + "\n")
		b.WriteString("TR: " + escape(tr) + "\n\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}
