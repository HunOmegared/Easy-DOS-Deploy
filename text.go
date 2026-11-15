package main

import (
	"strings"
	"unicode"
)

func hasNonASCII(s string) bool {
	for _, r := range s {
		if r > 0x7E || (r < 0x20 && r != '\t') {
			return true
		}
	}
	return false
}

// asciiFold maps the accented letters that occur in Hungarian (and a few
// other common ones) to plain ASCII.
var asciiFold = map[rune]string{
	'á': "a", 'é': "e", 'í': "i", 'ó': "o", 'ö': "o", 'ő': "o", 'ú': "u", 'ü': "u", 'ű': "u",
	'Á': "A", 'É': "E", 'Í': "I", 'Ó': "O", 'Ö': "O", 'Ő': "O", 'Ú': "U", 'Ü': "U", 'Ű': "U",
	'ä': "a", 'Ä': "A", 'ß': "ss", 'ç': "c", 'Ç': "C", 'ñ': "n", 'Ñ': "N",
	'č': "c", 'Č': "C", 'š': "s", 'Š': "S", 'ž': "z", 'Ž': "Z", 'ř': "r", 'Ř': "R",
	'–': "-", '—': "-", '„': "\"", '”': "\"", '“': "\"", '’': "'", '‘': "'", '…': "...",
	' ': " ",
}

func cpTable(cp Codepage) *[128]rune {
	switch cp {
	case CP852:
		return &cp852High
	case CP437:
		return &cp437High
	}
	return nil
}

// EncodeLine converts one line of text to DOS bytes in the given codepage.
func EncodeLine(s string, cp Codepage) []byte {
	tab := cpTable(cp)
	var rev map[rune]byte
	if tab != nil {
		rev = make(map[rune]byte, 128)
		for i, r := range tab {
			rev[r] = byte(128 + i)
		}
	}
	out := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r == '\t' || (r >= 0x20 && r < 0x7F):
			out = append(out, byte(r))
		case rev != nil && rev[r] != 0:
			out = append(out, rev[r])
		case asciiFold[r] != "":
			out = append(out, asciiFold[r]...)
		case unicode.IsControl(r):
			// drop
		default:
			out = append(out, '?')
		}
	}
	return out
}

// EncodeFile joins lines with CRLF (DOS), with a final CRLF.
func EncodeFile(lines []string, cp Codepage) []byte {
	var out []byte
	for _, l := range lines {
		out = append(out, EncodeLine(l, cp)...)
		out = append(out, '\r', '\n')
	}
	return out
}

// DecodeFile turns DOS bytes into lines. Any line ending is accepted; a
// trailing Ctrl-Z (old DOS end of file) is removed.
func DecodeFile(b []byte, cp Codepage) []string {
	if i := indexByte(b, 0x1A); i >= 0 {
		b = b[:i]
	}
	tab := cpTable(cp)
	if tab == nil {
		tab = &cp437High
	}
	var sb strings.Builder
	for _, c := range b {
		if c < 0x80 {
			sb.WriteByte(c)
		} else {
			sb.WriteRune(tab[c-128])
		}
	}
	s := strings.ReplaceAll(sb.String(), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// SplitLines splits text from an edit box into lines, dropping trailing empties.
func SplitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.TrimRight(s, "\n \t")
	if strings.TrimSpace(s) == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return lines
}

// JoinLines is the inverse of SplitLines for a Windows edit box.
func JoinLines(lines []string) string {
	return strings.Join(lines, "\r\n")
}
