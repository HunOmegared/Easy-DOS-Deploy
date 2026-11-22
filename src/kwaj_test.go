package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestKWAJSetupDisks expands every compressed file of a set of MS-DOS 6.x
// Setup disk images. The disks are not part of the repository; set
// EDD_SETUP_DISKS to a folder with the .img files to run it.
func TestKWAJSetupDisks(t *testing.T) {
	dir := os.Getenv("EDD_SETUP_DISKS")
	if dir == "" {
		t.Skip("EDD_SETUP_DISKS not set")
	}
	imgs, _ := filepath.Glob(filepath.Join(dir, "*.img"))
	if len(imgs) == 0 {
		t.Fatal("no images")
	}
	n := 0
	for _, img := range imgs {
		v, err := OpenImage(img, false)
		if err != nil {
			t.Fatal(err)
		}
		names, _ := v.List("")
		for _, name := range names {
			if !strings.HasSuffix(name, "_") {
				continue
			}
			b, err := v.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			if !IsKWAJ(b) {
				t.Errorf("%s: not KWAJ", name)
				continue
			}
			out, err := ExpandKWAJ(b)
			if err != nil {
				t.Errorf("%s %s: %v", filepath.Base(img), name, err)
				continue
			}
			if len(out) > 2 && out[0] == 'M' && out[1] == 'Z' {
				last, pages := int(binary.LittleEndian.Uint16(out[2:])), int(binary.LittleEndian.Uint16(out[4:]))
				if last == 0 {
					last = 512
				}
				if (pages-1)*512+last > len(out) {
					// SORT.EXE of MS-DOS 6.22 is like this on the original disks too
					t.Logf("%s: EXE header says %d bytes, the file has %d", name, (pages-1)*512+last, len(out))
				}
			}
			if ref, err := os.ReadFile("/tmp/x_" + name); err == nil && string(ref) != string(out) {
				t.Errorf("%s differs from the reference", name)
			}
			n++
		}
		v.Close()
	}
	t.Logf("%d files expanded", n)
}
