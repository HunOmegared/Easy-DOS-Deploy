package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Import reads existing CONFIG.SYS / FDCONFIG.SYS and AUTOEXEC.BAT text and
// rebuilds a project from them. Notes explain anything that could not be
// mapped exactly; nothing is silently thrown away.
func Import(configLines, autoexecLines []string, target Target) (*Project, []string) {
	var p *Project
	var notes []string
	if target == TargetFreeDOS {
		p, notes = importFreeDOSConfig(configLines)
	} else {
		p, notes = importMSDOSConfig(configLines)
	}
	p.Target = target
	notes = append(notes, importAutoexec(p, autoexecLines)...)
	if len(p.Items) == 0 {
		notes = append(notes, T("The file had no menu; its content went into the common part. Add menu items."))
	}
	if p.Default >= len(p.Items) {
		p.Default = 0
	}
	return p, notes
}

func isMarker(l string) bool {
	t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "@"))
	return strings.EqualFold(t, genMarker) || strings.EqualFold(t, oldMarker)
}

var reMSBlock = regexp.MustCompile(`^\s*\[([^\]]+)\]\s*$`)

func splitKV(l string) (key, val string, ok bool) {
	i := strings.IndexAny(l, "=")
	if i < 0 {
		return "", "", false
	}
	return strings.ToUpper(strings.TrimSpace(l[:i])), strings.TrimSpace(l[i+1:]), true
}

// normID turns a block name into a valid item ID, keeping the original
// when it is already valid.
func normID(s string, used map[string]bool) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	var b strings.Builder
	for _, c := range s {
		if c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' {
			b.WriteRune(c)
		}
	}
	id := b.String()
	if len(id) > 8 {
		id = id[:8]
	}
	if id == "" || reservedIDs[id] {
		id = "ITEM"
	}
	base := id
	for n := 2; used[id]; n++ {
		suf := strconv.Itoa(n)
		if len(base)+len(suf) > 8 {
			id = base[:8-len(suf)] + suf
		} else {
			id = base + suf
		}
	}
	used[id] = true
	return id
}

func importMSDOSConfig(lines []string) (*Project, []string) {
	p := &Project{Timeout: -1}
	var notes []string
	blocks := map[string][]string{} // upper-case name -> lines
	var order []string
	var commons [][]string // each [COMMON] occurrence in order
	cur := "COMMON"
	curIdx := 0
	commons = append(commons, nil) // lines before any header count as common
	firstItemSeen := false
	commonBeforeItems := 0 // index into commons: last common before first item block
	for _, l := range lines {
		if isMarker(l) {
			continue
		}
		if m := reMSBlock.FindStringSubmatch(l); m != nil {
			name := strings.ToUpper(strings.TrimSpace(m[1]))
			cur = name
			if name == "COMMON" {
				commons = append(commons, nil)
				curIdx = len(commons) - 1
				if !firstItemSeen {
					commonBeforeItems = curIdx
				}
			} else if name != "MENU" {
				if _, ok := blocks[name]; !ok {
					order = append(order, name)
				}
				blocks[name] = blocks[name] // ensure present
				firstItemSeen = true
			}
			continue
		}
		switch cur {
		case "COMMON":
			commons[curIdx] = append(commons[curIdx], l)
		default:
			blocks[cur] = append(blocks[cur], l)
		}
	}
	for i, c := range commons {
		c = trimBlank(c)
		if i <= commonBeforeItems {
			p.Start.Config = append(p.Start.Config, c...)
		} else {
			p.End.Config = append(p.End.Config, c...)
		}
	}
	p.Start.Config = trimBlank(p.Start.Config)
	p.End.Config = trimBlank(p.End.Config)

	// Parse the [MENU] block.
	used := map[string]bool{}
	idOf := map[string]string{} // block name -> item ID
	defName := ""
	for _, l := range blocks["MENU"] {
		t := strings.TrimSpace(l)
		if t == "" || isRem(t) {
			continue
		}
		k, v, ok := splitKV(t)
		if !ok {
			notes = append(notes, T("Unknown line in the [MENU] block: %s", t))
			continue
		}
		switch k {
		case "MENUITEM":
			name, text := v, ""
			if i := strings.Index(v, ","); i >= 0 {
				name, text = v[:i], strings.TrimSpace(v[i+1:])
			}
			name = strings.ToUpper(strings.TrimSpace(name))
			id := normID(name, used)
			if id != name {
				notes = append(notes, T("Block [%s] got a new ID: %s (at most 8 letters/digits).", name, id))
			}
			idOf[name] = id
			body := expandIncludes(name, blocks, map[string]bool{}, &notes)
			p.Items = append(p.Items, Item{ID: id, Title: text, Config: trimBlank(body)})
		case "MENUDEFAULT":
			parts := strings.Split(v, ",")
			defName = strings.ToUpper(strings.TrimSpace(parts[0]))
			if len(parts) > 1 {
				if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
					p.Timeout = n
				}
			}
		case "MENUCOLOR":
			p.MenuColor = strings.ReplaceAll(v, " ", "")
		case "SUBMENU":
			notes = append(notes, T("Submenus (SUBMENU) are not supported, left out: %s", t))
		case "NUMLOCK":
			notes = append(notes, T("The NUMLOCK setting was left out: %s", t))
		default:
			notes = append(notes, T("Unknown line in the [MENU] block: %s", t))
		}
	}
	if defName != "" {
		for i, it := range p.Items {
			if it.ID == idOf[defName] {
				p.Default = i
			}
		}
	}
	for _, name := range order {
		if _, ok := idOf[name]; !ok && name != "MENU" && !includedSomewhere(name, blocks) {
			notes = append(notes, T("No menu item refers to block [%s]; it was left out.", name))
		}
	}
	if len(blocks["MENU"]) == 0 && len(order) > 0 {
		notes = append(notes, T("There is no [MENU] block; the other blocks were left out."))
	}
	return p, notes
}

func includedSomewhere(name string, blocks map[string][]string) bool {
	for _, ls := range blocks {
		for _, l := range ls {
			if k, v, ok := splitKV(strings.TrimSpace(l)); ok && k == "INCLUDE" && strings.EqualFold(v, name) {
				return true
			}
		}
	}
	return false
}

func expandIncludes(name string, blocks map[string][]string, stack map[string]bool, notes *[]string) []string {
	if stack[name] {
		*notes = append(*notes, T("Circular INCLUDE: %s", name))
		return nil
	}
	stack[name] = true
	defer delete(stack, name)
	var out []string
	for _, l := range blocks[name] {
		if k, v, ok := splitKV(strings.TrimSpace(l)); ok && k == "INCLUDE" {
			inc := strings.ToUpper(v)
			if _, exists := blocks[inc]; exists {
				out = append(out, expandIncludes(inc, blocks, stack, notes)...)
				continue
			}
			*notes = append(*notes, T("The block named in INCLUDE does not exist: %s", v))
		}
		out = append(out, l)
	}
	return out
}

func isRem(t string) bool {
	u := strings.ToUpper(t)
	return strings.HasPrefix(u, "REM ") || u == "REM" || strings.HasPrefix(u, ";")
}

func trimBlank(ls []string) []string {
	for len(ls) > 0 && strings.TrimSpace(ls[0]) == "" {
		ls = ls[1:]
	}
	for len(ls) > 0 && strings.TrimSpace(ls[len(ls)-1]) == "" {
		ls = ls[:len(ls)-1]
	}
	return ls
}

var reFDPrefix = regexp.MustCompile(`^\s*([0-9]+)\?(.*)$`)
var reFDMenu = regexp.MustCompile(`(?i)^\s*MENU(\s+(.*))?$`)
var reFDMenuItem = regexp.MustCompile(`^\s*([1-9])\s*[-.):]?\s*(.*)$`)

func importFreeDOSConfig(lines []string) (*Project, []string) {
	p := &Project{Timeout: -1}
	var notes []string
	items := map[int]*Item{}
	maxN := 0
	get := func(n int) *Item {
		if items[n] == nil {
			items[n] = &Item{}
		}
		if n > maxN {
			maxN = n
		}
		return items[n]
	}
	seenItemLine := false
	ids, remLines := fdRemIDs(lines)
	for ln, l := range lines {
		if isMarker(l) || remLines[ln] {
			continue
		}
		t := strings.TrimSpace(l)
		if strings.EqualFold(t, fdEndMarker) {
			seenItemLine = true
			continue
		}
		if m := reFDMenu.FindStringSubmatch(t); m != nil && !strings.Contains(strings.ToUpper(t), "MENUDEFAULT") && !strings.Contains(strings.ToUpper(t), "MENUCOLOR") {
			text := strings.TrimSpace(m[2])
			if text == "" {
				continue
			}
			if mm := reFDMenuItem.FindStringSubmatch(text); mm != nil {
				n, _ := strconv.Atoi(mm[1])
				get(n).Title = strings.TrimSpace(mm[2])
			} else {
				notes = append(notes, T("A MENU line that is not a menu item was left out: %s", t))
			}
			continue
		}
		if k, v, ok := splitKV(strings.TrimPrefix(t, "!")); ok && (k == "MENUDEFAULT" || k == "MENUCOLOR") {
			if k == "MENUCOLOR" {
				p.MenuColor = strings.ReplaceAll(v, " ", "")
				continue
			}
			parts := strings.Split(v, ",")
			if n, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
				p.Default = n - 1
			}
			if len(parts) > 1 {
				if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
					p.Timeout = n
				}
			}
			continue
		}
		if m := reFDPrefix.FindStringSubmatch(l); m != nil {
			seenItemLine = true
			for _, d := range m[1] {
				n := int(d - '0')
				if n == 0 {
					notes = append(notes, T("A line with the 0? prefix (start without menu) was left out: %s", t))
					continue
				}
				it := get(n)
				it.Config = append(it.Config, m[2])
			}
			continue
		}
		if seenItemLine && t != "" {
			p.End.Config = append(p.End.Config, l)
		} else {
			p.Start.Config = append(p.Start.Config, l)
		}
	}
	// FreeDOS: generated files have a "REM ID" line before each block; use it
	// for the ID when present.
	used := map[string]bool{}
	for n := 1; n <= maxN; n++ {
		it := items[n]
		if it == nil {
			it = &Item{}
			notes = append(notes, T("Menu item %d was missing; an empty one was added.", n))
		}
		want := ids[n]
		if want == "" {
			want = fmt.Sprintf("OPT%d", n)
		}
		it.ID = normID(want, used)
		it.Config = trimBlank(it.Config)
		p.Items = append(p.Items, *it)
	}
	p.Start.Config = trimBlank(p.Start.Config)
	p.End.Config = trimBlank(p.End.Config)
	return p, notes
}

// fdRemIDs finds "REM <ID>" lines directly followed by a single-digit
// prefixed line (as this program writes them). It returns digit -> ID and
// the line numbers of those REM lines.
func fdRemIDs(lines []string) (map[int]string, map[int]bool) {
	out := map[int]string{}
	idx := map[int]bool{}
	pending, pendingAt := "", -1
	for n, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(strings.ToUpper(t), "REM ") && !isMarker(t) {
			f := strings.Fields(t)
			if len(f) == 2 && ValidID(strings.ToUpper(f[1])) == nil {
				pending, pendingAt = strings.ToUpper(f[1]), n
				continue
			}
		}
		if m := reFDPrefix.FindStringSubmatch(l); m != nil && pending != "" && len(m[1]) == 1 {
			d := int(m[1][0] - '0')
			if _, dup := out[d]; !dup {
				out[d] = pending
				idx[pendingAt] = true
			}
		}
		pending = ""
	}
	return out, idx
}

var reAutoID = regexp.MustCompile(`^OPT[0-9]$`)

var reIfConfig = regexp.MustCompile(`(?i)^\s*@?IF\s+(NOT\s+)?"?%CONFIG%"?\s*==\s*"?([^"\s]*)"?\s+GOTO\s+:?(\S+)\s*$`)
var reGotoConfig = regexp.MustCompile(`(?i)^\s*@?GOTO\s+:?%CONFIG%\s*$`)
var reLabel = regexp.MustCompile(`^\s*:([^\s:]+)`)
var reGoto = regexp.MustCompile(`(?i)^\s*@?GOTO\s+:?(\S+)\s*$`)

func labelKey(s string) string {
	s = strings.ToUpper(s)
	if len(s) > 8 {
		s = s[:8] // DOS only compares the first 8 characters
	}
	return s
}

func importAutoexec(p *Project, lines []string) []string {
	var notes []string
	// value of %CONFIG% -> item index
	valIdx := map[string]int{}
	for i, it := range p.Items {
		if p.Target == TargetFreeDOS {
			valIdx[strconv.Itoa(i+1)] = i
		} else {
			valIdx[it.ID] = i
		}
	}
	labelItem := map[string]int{} // label -> item index
	dispatch := map[int]bool{}    // line numbers that are dispatch lines
	for n, l := range lines {
		if m := reIfConfig.FindStringSubmatch(l); m != nil && m[1] == "" {
			if i, ok := valIdx[strings.ToUpper(m[2])]; ok {
				labelItem[labelKey(m[3])] = i
				dispatch[n] = true
			}
		} else if reGotoConfig.MatchString(l) {
			for v, i := range valIdx {
				labelItem[labelKey(v)] = i
			}
			dispatch[n] = true
		}
	}
	// FreeDOS items get their IDs from the batch labels when the config
	// file did not name them.
	if p.Target == TargetFreeDOS {
		used := map[string]bool{}
		for _, it := range p.Items {
			used[it.ID] = true
		}
		for k, i := range labelItem {
			if reAutoID.MatchString(p.Items[i].ID) && ValidID(k) == nil && !used[k] {
				used[p.Items[i].ID] = false
				p.Items[i].ID = k
				used[k] = true
			}
		}
	}
	if len(dispatch) == 0 {
		var keep []string
		for _, l := range lines {
			if !isMarker(l) {
				keep = append(keep, l)
			}
		}
		p.Start.Autoexec = trimBlank(keep)
		if len(lines) > 0 && len(p.Items) > 0 {
			notes = append(notes, T("AUTOEXEC.BAT does not branch on %%CONFIG%%, so all of it went into the common part."))
		}
		return notes
	}
	// Find the label the item branches jump to when they are done.
	votes := map[string]int{}
	inItem := false
	var last string
	for _, l := range lines {
		if m := reLabel.FindStringSubmatch(l); m != nil {
			if inItem && last != "" {
				votes[last]++
			}
			_, inItem = labelItem[labelKey(m[1])]
			last = ""
			continue
		}
		if strings.TrimSpace(l) == "" {
			continue
		}
		if m := reGoto.FindStringSubmatch(l); m != nil {
			last = labelKey(m[1])
		} else {
			last = ""
		}
	}
	if inItem && last != "" {
		votes[last]++
	}
	end := ""
	for k, v := range votes {
		if _, isItem := labelItem[k]; isItem {
			continue
		}
		if end == "" || v > votes[end] {
			end = k
		}
	}

	state := -1 // -1 start, >=0 item, -2 end
	var cur *[]string = &p.Start.Autoexec
	finish := func() {
		// drop the trailing "GOTO end" of the section we leave
		ls := trimBlank(*cur)
		if state != -2 && len(ls) > 0 {
			if m := reGoto.FindStringSubmatch(ls[len(ls)-1]); m != nil && labelKey(m[1]) == end {
				ls = ls[:len(ls)-1]
			}
		}
		*cur = trimBlank(ls)
	}
	for n, l := range lines {
		if dispatch[n] || isMarker(l) {
			continue
		}
		if m := reLabel.FindStringSubmatch(l); m != nil && state != -2 {
			k := labelKey(m[1])
			if i, ok := labelItem[k]; ok {
				finish()
				state = i
				cur = &p.Items[i].Autoexec
				if len(*cur) > 0 {
					notes = append(notes, T("Two labels point to the same menu item: %s", m[1]))
				}
				continue
			}
			if k == end && end != "" {
				finish()
				state = -2
				cur = &p.End.Autoexec
				continue
			}
		}
		*cur = append(*cur, l)
	}
	finish()
	// The start section usually ends with a "GOTO end" fallback after the IF lines.
	if ls := p.Start.Autoexec; len(ls) > 0 {
		if m := reGoto.FindStringSubmatch(ls[len(ls)-1]); m != nil && labelKey(m[1]) == end {
			p.Start.Autoexec = trimBlank(ls[:len(ls)-1])
		}
	}
	for i, it := range p.Items {
		found := false
		for _, j := range labelItem {
			if j == i {
				found = true
			}
		}
		if !found {
			notes = append(notes, T("Menu item %s has no branch in AUTOEXEC.BAT.", it.ID))
		}
	}
	return notes
}
