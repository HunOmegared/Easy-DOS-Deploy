//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"
)

// The "Your files" page: folders and files of the PC that go onto the disk.

const (
	flHead = 10 + iota
	flIntro
	flList
	flAddDir
	flAddFile
	flDest
	flRemove
	flSum
)

type filesWin struct {
	hwnd  uintptr
	c     map[int]uintptr
	sizes map[string]int64 // source -> bytes (walked once)
}

func (a *app) createFilesPage() {
	w := &filesWin{c: map[int]uintptr{}, sizes: map[string]int64{}}
	a.files = w
	w.hwnd = a.newPanel("EasyDOSDeployFiles")
	mk := func(id int, class, text string, style, ex uint32) uintptr {
		h := createWindow(ex, class, text, WS_CHILD|WS_VISIBLE|style, 0, 0, 10, 10, w.hwnd, uintptr(id))
		sendMsg(h, WM_SETFONT, a.font, 1)
		w.c[id] = h
		return h
	}
	sendMsg(mk(flHead, "STATIC", T("Your files"), 0, 0), WM_SETFONT, a.head, 1)
	mk(flIntro, "STATIC", T("Folders and files from this PC that go onto the DOS disk: games, utilities, Norton Commander... A folder's contents go into the DOS folder given for it. Long names become 8.3 names, as DOS shows them (NORTON~1)."), 0, 0)
	lv := mk(flList, "SysListView32", "", WS_TABSTOP|LVS_REPORT|LVS_SINGLESEL|LVS_SHOWSELALWAYS|LVS_NOSORTHEADER, WS_EX_CLIENTEDGE)
	sendMsg(lv, LVM_SETEXTENDEDLISTVIEWSTYLE, 0, LVS_EX_FULLROWSELECT|LVS_EX_DOUBLEBUFFER)
	lvInsertColumn(lv, 0, T("On this PC"), a.s(360), false)
	lvInsertColumn(lv, 1, T("On the DOS disk"), a.s(200), false)
	lvInsertColumn(lv, 2, T("Size"), a.s(90), false)
	mk(flAddDir, "BUTTON", T("Add a folder..."), WS_TABSTOP, 0)
	mk(flAddFile, "BUTTON", T("Add a file..."), WS_TABSTOP, 0)
	mk(flDest, "BUTTON", T("Destination..."), WS_TABSTOP, 0)
	mk(flRemove, "BUTTON", T("Remove"), WS_TABSTOP, 0)
	mk(flSum, "STATIC", "", 0, 0)
	w.layout()
	w.refresh()
}

func (w *filesWin) layout() {
	a := theApp
	s := a.s
	cw, ch := clientSize(w.hwnd)
	m := s(14)
	move(w.c[flHead], m, m, cw-2*m, s(28))
	move(w.c[flIntro], m, m+s(32), cw-2*m, s(54))
	top := m + s(32) + s(62)
	bh := s(28)
	lh := ch - top - bh - s(52) - m
	move(w.c[flList], m, top, cw-2*m, lh)
	x := m
	for _, id := range []int{flAddDir, flAddFile, flDest, flRemove} {
		bw := fitW(s(110), s(24), w.c[id])
		move(w.c[id], x, top+lh+s(8), bw, bh)
		x += bw + s(6)
	}
	move(w.c[flSum], m, top+lh+s(8)+bh+s(8), cw-2*m, s(36))
}

func (w *filesWin) size(src string) int64 {
	if n, ok := w.sizes[src]; ok {
		return n
	}
	n, _, _ := CopySize([]CopyItem{{src, `C:\`}})
	w.sizes[src] = n
	return n
}

func (w *filesWin) refresh() {
	a := theApp
	lv := w.c[flList]
	cur := lvFocused(lv)
	sendMsg(lv, LVM_DELETEALLITEMS, 0, 0)
	var total int64
	for i, it := range a.proj.Files {
		n := w.size(it.Source)
		total += n
		lvSetCell(lv, i, 0, it.Source, iImageNone)
		lvSetCell(lv, i, 1, it.Dest, -1)
		lvSetCell(lv, i, 2, humanSize(n), -1)
	}
	if cur >= 0 && cur < len(a.proj.Files) {
		lvSelect(lv, cur)
	}
	if len(a.proj.Files) == 0 {
		setText(w.c[flSum], T("Nothing yet. This step is optional."))
	} else {
		setText(w.c[flSum], T("Together: %s. They are copied after the system, the drivers and the menu.", humanSize(total)))
	}
	a.updateSide()
}

// driveList lists the DOS drives the disk will have (Machine and disk).
func (a *app) driveList() []string {
	out := []string{"C:"}
	if a.boot == nil {
		return out
	}
	o := a.boot.options()
	if o.Kind != MediaHDD {
		return out
	}
	p, err := PlanHDD(o.Geom)
	if err != nil {
		return out
	}
	if o.OnlyC {
		p.OnlyC()
	}
	out = out[:0]
	for i, d := range p.Drives() {
		out = append(out, fmt.Sprintf("%c:  (%s)", 'C'+i, humanSize(int64(d.Size)*512)))
	}
	return out
}

// add puts the chosen folders or files on the list, asking where they go.
func (w *filesWin) add(srcs []string) {
	a := theApp
	var fresh []string
	for _, src := range srcs {
		dup := false
		for _, it := range a.proj.Files {
			if strings.EqualFold(it.Source, src) {
				dup = true
			}
		}
		if !dup {
			fresh = append(fresh, src)
		}
	}
	if len(fresh) == 0 {
		return
	}
	var prompt, folder, hint string
	one := len(fresh) == 1
	dir := false
	if st, err := os.Stat(fresh[0]); err == nil && st.IsDir() {
		dir = true
	}
	switch {
	case one && dir:
		prompt = T("Where should the folder %s go on the DOS disk?", filepath.Base(fresh[0]))
		folder = strings.TrimPrefix(DefaultDest(fresh[0]), `C:\`)
		hint = T("Its contents go into this folder.")
	case one:
		prompt = T("Where should %s go on the DOS disk?", filepath.Base(fresh[0]))
		hint = T("Empty: the root of the drive.")
	default:
		prompt = T("Where should these %d items go on the DOS disk?", len(fresh))
		hint = T("Each folder gets its own folder in it, the files go into it. Empty: the root of the drive.")
	}
	drive, f, ok := destBox(a.hwnd, prompt, a.driveList(), 0, folder, hint)
	if !ok {
		return
	}
	root := fmt.Sprintf(`%c:\`, 'C'+drive)
	for _, src := range fresh {
		dest := root + f
		if !one {
			if st, err := os.Stat(src); err == nil && st.IsDir() {
				n := strings.TrimPrefix(DefaultDest(src), `C:\`)
				dest = strings.TrimSuffix(root+f, `\`) + `\` + n
				if f == "" {
					dest = root + n
				}
			}
		}
		a.proj.Files = append(a.proj.Files, CopyItem{Source: src, Dest: dest})
	}
	a.setDirty()
	w.refresh()
	lvSelect(w.c[flList], len(a.proj.Files)-1)
}

func (w *filesWin) setDest() {
	a := theApp
	i := lvFocused(w.c[flList])
	if i < 0 || i >= len(a.proj.Files) {
		return
	}
	it := &a.proj.Files[i]
	drive, parts, _ := splitDest(it.Dest)
	d, f, ok := destBox(a.hwnd, T("DOS folder for %s:", filepath.Base(it.Source)), a.driveList(), drive, strings.Join(parts, `\`), T("For example: GAMES\\DOOM. Empty: the root of the drive."))
	if !ok {
		return
	}
	it.Dest = fmt.Sprintf(`%c:\%s`, 'C'+d, strings.ToUpper(f))
	a.setDirty()
	w.refresh()
}

func (w *filesWin) remove() {
	a := theApp
	i := lvFocused(w.c[flList])
	if i < 0 || i >= len(a.proj.Files) {
		return
	}
	a.proj.Files = append(a.proj.Files[:i], a.proj.Files[i+1:]...)
	a.setDirty()
	w.refresh()
}

func filesProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	a := theApp
	w := a.files
	switch msg {
	case WM_SIZE:
		if w != nil && len(w.c) > 0 {
			w.layout()
		}
		return 0
	case WM_NOTIFY:
		if w != nil {
			if n := *(**nmListView)(unsafe.Pointer(&lParam)); n.hdr.idFrom == flList && n.hdr.code == NM_DBLCLK {
				w.setDest()
			}
		}
		return 0
	case WM_COMMAND:
		if w == nil || hiword(wParam) != 0 {
			return 0
		}
		switch int(loword(wParam)) {
		case flAddDir:
			if ps := pickItems(a.hwnd, T("Folders for the DOS disk"), true); len(ps) > 0 {
				w.add(ps)
			}
		case flAddFile:
			if ps := pickItems(a.hwnd, T("Files for the DOS disk"), false); len(ps) > 0 {
				w.add(ps)
			}
		case flDest:
			w.setDest()
		case flRemove:
			w.remove()
		}
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r
}
