package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectDisk(t *testing.T) {
	// a card made by the program: one FAT16 partition with a label
	out := filepath.Join(t.TempDir(), "c.img")
	if _, err := CreateBootImage(BootOptions{Path: out, Kind: MediaHDD, Geom: Geometry{120, 16, 63}, Label: "RETRO"}); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(out)
	ps, err := InspectDisk(f)
	f.Close()
	if err != nil || len(ps) != 1 || ps[0].FS != "FAT16" || ps[0].Label != "RETRO" || ps[0].Modern {
		t.Fatalf("%+v %v", ps, err)
	}
	txt, modern := DescribeParts(ps)
	if modern || !strings.Contains(txt, "FAT16") {
		t.Fatal(txt)
	}

	// a modern USB disk: NTFS partition and a Linux one
	d := make([]byte, 4096*512)
	e := d[446:]
	e[4] = 0x07
	binary.LittleEndian.PutUint32(e[8:], 2048)
	binary.LittleEndian.PutUint32(e[12:], 1024)
	e2 := d[462:]
	e2[4] = 0x83
	binary.LittleEndian.PutUint32(e2[8:], 3072)
	d[510], d[511] = 0x55, 0xAA
	copy(d[2048*512+3:], "NTFS    ")
	d[2048*512+510], d[2048*512+511] = 0x55, 0xAA
	ps, _ = InspectDisk(bytes.NewReader(d))
	if len(ps) != 2 || ps[0].FS != "NTFS" || !ps[0].Modern || ps[1].FS != "Linux" {
		t.Fatalf("%+v", ps)
	}
	if _, modern := DescribeParts(ps); !modern {
		t.Fatal("not modern")
	}

	// a GPT disk and an exFAT stick without partition table
	g := make([]byte, 4096)
	g[446+4] = 0xEE
	g[510], g[511] = 0x55, 0xAA
	if ps, _ := InspectDisk(bytes.NewReader(g)); len(ps) != 1 || !ps[0].Modern {
		t.Fatalf("gpt %+v", ps)
	}
	x := make([]byte, 4096)
	x[0], x[1], x[2] = 0xEB, 0x76, 0x90
	copy(x[3:], "EXFAT   ")
	x[510], x[511] = 0x55, 0xAA
	if ps, _ := InspectDisk(bytes.NewReader(x)); len(ps) != 1 || ps[0].FS != "exFAT" || !ps[0].Modern {
		t.Fatalf("exfat %+v", ps)
	}
	// a floppy image (no MBR)
	fl := filepath.Join(t.TempDir(), "f.img")
	CreateBootImage(BootOptions{Path: fl, Kind: MediaF144})
	f, _ = os.Open(fl)
	ps, _ = InspectDisk(f)
	f.Close()
	if len(ps) != 1 || ps[0].Index != 0 || ps[0].FS != "FAT12" {
		t.Fatalf("floppy %+v", ps)
	}
	// empty disk
	if ps, _ := InspectDisk(bytes.NewReader(make([]byte, 1024))); len(ps) != 0 {
		t.Fatal(ps)
	}
}
