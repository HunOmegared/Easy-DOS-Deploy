// Command winres writes rsrc_windows_amd64.syso: the Windows resources of
// EasyDOSDeploy.exe (icon, version information, manifest) as a COFF object that
// the Go linker includes automatically. No outside tools are needed.
//
//	go run ./tools/winres
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sort"
	"unicode/utf16"
)

const (
	version     = "0.8.6"
	versionMS   = 0<<16 | 8 // major.minor
	versionLS   = 6<<16 | 0 // patch.build
	productName = "Easy DOS Deploy"
	company     = "Daniel Szentmiklosi"
	description = "Easy DOS Deploy - DOS boot menus, bootable disk images and CF cards"
	copyright   = "Copyright (C) 2026 Daniel Szentmiklosi. GNU GPL v3."
	exeName     = "EasyDOSDeploy.exe"
)

const manifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity type="win32" name="Szentmiklosi.EasyDOSDeploy" version="0.8.6.0" processorArchitecture="amd64"/>
  <description>Easy DOS Deploy</description>
  <dependency>
    <dependentAssembly>
      <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
    </dependentAssembly>
  </dependency>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
    </application>
  </compatibility>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true</dpiAware>
    </windowsSettings>
  </application>
</assembly>
`

// ---- icon: a 3.5" floppy disk drawn from shapes, at several sizes ----

type rgba struct{ r, g, b, a byte }

func iconPixels(n int) []rgba {
	px := make([]rgba, n*n)
	f := func(v float64) int { return int(v*float64(n) + 0.5) }
	fill := func(x0, y0, x1, y1 float64, c rgba) {
		for y := f(y0); y < f(y1); y++ {
			for x := f(x0); x < f(x1); x++ {
				if x >= 0 && y >= 0 && x < n && y < n {
					px[y*n+x] = c
				}
			}
		}
	}
	body := rgba{0x23, 0x4E, 0x9C, 0xFF}
	edge := rgba{0x14, 0x2E, 0x60, 0xFF}
	metal := rgba{0xC8, 0xCC, 0xD2, 0xFF}
	slot := rgba{0x50, 0x56, 0x60, 0xFF}
	label := rgba{0xF4, 0xF4, 0xEE, 0xFF}
	line := rgba{0x9A, 0xB4, 0xD8, 0xFF}
	fill(0.06, 0.06, 0.94, 0.94, edge)  // outline
	fill(0.10, 0.10, 0.90, 0.90, body)  // body
	fill(0.26, 0.06, 0.72, 0.38, metal) // shutter
	fill(0.52, 0.12, 0.63, 0.32, slot)  // shutter window
	fill(0.20, 0.52, 0.80, 0.90, label) // label
	if n >= 32 {
		fill(0.26, 0.62, 0.74, 0.65, line)
		fill(0.26, 0.72, 0.74, 0.75, line)
		fill(0.26, 0.82, 0.60, 0.85, line)
	}
	// cut corner (top right), as on a real disk
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if float64(x-y) > 0.80*float64(n) {
				px[y*n+x] = rgba{}
			}
		}
	}
	return px
}

// iconDIB is an RT_ICON resource: BITMAPINFOHEADER, 32-bit BGRA bottom-up, AND mask.
func iconDIB(n int) []byte {
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	w(uint32(40))
	w(int32(n))
	w(int32(2 * n))
	w(uint16(1))
	w(uint16(32))
	w(uint32(0))
	w(uint32(n * n * 4))
	w(int32(0))
	w(int32(0))
	w(uint32(0))
	w(uint32(0))
	px := iconPixels(n)
	for y := n - 1; y >= 0; y-- {
		for x := 0; x < n; x++ {
			p := px[y*n+x]
			b.Write([]byte{p.b, p.g, p.r, p.a})
		}
	}
	row := ((n + 31) / 32) * 4
	b.Write(make([]byte, row*n)) // AND mask: all 0, alpha does the work
	return b.Bytes()
}

// ---- version information ----

func u16z(s string) []byte {
	var b bytes.Buffer
	for _, c := range utf16.Encode([]rune(s)) {
		binary.Write(&b, binary.LittleEndian, c)
	}
	b.Write([]byte{0, 0})
	return b.Bytes()
}

func pad4(b *bytes.Buffer) {
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
}

// vnode builds one VS_VERSIONINFO-style block.
func vnode(key string, value []byte, valueLen uint16, typ uint16, children ...[]byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{0, 0})
	binary.Write(&b, binary.LittleEndian, valueLen)
	binary.Write(&b, binary.LittleEndian, typ)
	b.Write(u16z(key))
	pad4(&b)
	b.Write(value)
	for _, c := range children {
		pad4(&b)
		b.Write(c)
	}
	out := b.Bytes()
	binary.LittleEndian.PutUint16(out, uint16(len(out)))
	return out
}

func versionInfo() []byte {
	var fixed bytes.Buffer
	for _, v := range []uint32{0xFEEF04BD, 0x00010000, versionMS, versionLS, versionMS, versionLS,
		0x3F, 0, 0x00040004, 1, 0, 0, 0} {
		binary.Write(&fixed, binary.LittleEndian, v)
	}
	str := func(k, v string) []byte {
		val := u16z(v)
		return vnode(k, val, uint16(len(val)/2), 1)
	}
	table := vnode("040904B0", nil, 0, 1,
		str("CompanyName", company),
		str("FileDescription", description),
		str("FileVersion", version),
		str("InternalName", "EasyDOSDeploy"),
		str("LegalCopyright", copyright),
		str("OriginalFilename", exeName),
		str("ProductName", productName),
		str("ProductVersion", version),
		str("Comments", "https://github.com/HunOmegared/EasyDOSDeploy"),
	)
	sfi := vnode("StringFileInfo", nil, 0, 1, table)
	tr := []byte{0x09, 0x04, 0xB0, 0x04}
	vfi := vnode("VarFileInfo", nil, 0, 1, vnode("Translation", tr, 4, 0))
	return vnode("VS_VERSION_INFO", fixed.Bytes(), uint16(fixed.Len()), 0, sfi, vfi)
}

// ---- resource tree and COFF object ----

type res struct {
	typ, id uint32
	data    []byte
}

const lang = 0x0409

func main() {
	sizes := []int{16, 24, 32, 48, 64}
	var rs []res
	var group bytes.Buffer
	binary.Write(&group, binary.LittleEndian, []uint16{0, 1, uint16(len(sizes))})
	for i, n := range sizes {
		dib := iconDIB(n)
		rs = append(rs, res{3, uint32(i + 1), dib}) // RT_ICON
		group.Write([]byte{byte(n), byte(n), 0, 0})
		binary.Write(&group, binary.LittleEndian, uint16(1))
		binary.Write(&group, binary.LittleEndian, uint16(32))
		binary.Write(&group, binary.LittleEndian, uint32(len(dib)))
		binary.Write(&group, binary.LittleEndian, uint16(i+1))
	}
	rs = append(rs, res{14, 1, group.Bytes()})    // RT_GROUP_ICON
	rs = append(rs, res{16, 1, versionInfo()})    // RT_VERSION
	rs = append(rs, res{24, 1, []byte(manifest)}) // RT_MANIFEST
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].typ != rs[j].typ {
			return rs[i].typ < rs[j].typ
		}
		return rs[i].id < rs[j].id
	})

	// Layout: root directory, one directory per type, one per (type,id)
	// (the language level), then data entries, then the data.
	types := []uint32{}
	byType := map[uint32][]res{}
	for _, r := range rs {
		if _, ok := byType[r.typ]; !ok {
			types = append(types, r.typ)
		}
		byType[r.typ] = append(byType[r.typ], r)
	}
	dirSize := func(n int) int { return 16 + 8*n }
	off := dirSize(len(types))
	typeDir := map[uint32]int{}
	for _, t := range types {
		typeDir[t] = off
		off += dirSize(len(byType[t]))
	}
	langDir := make([]int, len(rs))
	for i := range rs {
		langDir[i] = off
		off += dirSize(1)
	}
	dataEntry := make([]int, len(rs))
	for i := range rs {
		dataEntry[i] = off
		off += 16
	}
	dataOff := make([]int, len(rs))
	for i, r := range rs {
		off = (off + 7) &^ 7
		dataOff[i] = off
		off += len(r.data)
	}
	sec := make([]byte, (off+3)&^3)
	le := binary.LittleEndian
	writeDir := func(at, nIDs int) {
		le.PutUint16(sec[at+14:], uint16(nIDs))
	}
	writeDir(0, len(types))
	for i, t := range types {
		e := 16 + 8*i
		le.PutUint32(sec[e:], t)
		le.PutUint32(sec[e+4:], uint32(typeDir[t])|0x80000000)
	}
	idx := 0
	var relocs []uint32
	for _, t := range types {
		writeDir(typeDir[t], len(byType[t]))
		for j := range byType[t] {
			r := rs[idx]
			e := typeDir[t] + 16 + 8*j
			le.PutUint32(sec[e:], r.id)
			le.PutUint32(sec[e+4:], uint32(langDir[idx])|0x80000000)
			writeDir(langDir[idx], 1)
			le.PutUint32(sec[langDir[idx]+16:], lang)
			le.PutUint32(sec[langDir[idx]+20:], uint32(dataEntry[idx]))
			de := dataEntry[idx]
			le.PutUint32(sec[de:], uint32(dataOff[idx])) // RVA, fixed up by the linker
			le.PutUint32(sec[de+4:], uint32(len(r.data)))
			relocs = append(relocs, uint32(de))
			copy(sec[dataOff[idx]:], r.data)
			idx++
		}
	}

	// COFF: file header, one section header, raw data, relocations, symbols.
	var out bytes.Buffer
	w := func(v any) { binary.Write(&out, le, v) }
	const fileHdr, secHdr = 20, 40
	rawPtr := fileHdr + secHdr
	relPtr := rawPtr + len(sec)
	symPtr := relPtr + 10*len(relocs)
	w(uint16(0x8664)) // AMD64
	w(uint16(1))      // sections
	w(uint32(0))      // time stamp
	w(uint32(symPtr))
	w(uint32(1)) // symbols
	w(uint16(0)) // optional header
	w(uint16(0x0004))
	name := [8]byte{'.', 'r', 's', 'r', 'c'}
	w(name)
	w(uint32(0)) // virtual size
	w(uint32(0)) // virtual address
	w(uint32(len(sec)))
	w(uint32(rawPtr))
	w(uint32(relPtr))
	w(uint32(0)) // line numbers
	w(uint16(len(relocs)))
	w(uint16(0))
	w(uint32(0x40000040)) // initialized data, readable
	out.Write(sec)
	for _, r := range relocs {
		w(r)
		w(uint32(0))      // symbol 0: the section
		w(uint16(0x0003)) // IMAGE_REL_AMD64_ADDR32NB
	}
	w(name)      // symbol name
	w(uint32(0)) // value
	w(int16(1))  // section number
	w(uint16(0)) // type
	w(uint8(3))  // static
	w(uint8(0))  // aux symbols
	w(uint32(4)) // empty string table
	if err := os.WriteFile("rsrc_windows_amd64.syso", out.Bytes(), 0644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("rsrc_windows_amd64.syso: %d bytes, %d resources\n", out.Len(), len(rs))
}
