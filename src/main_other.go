//go:build !windows

package main

import (
	"fmt"
	"os"
)

// On other systems there is no window; a small command line helps testing:
//
//	bootmenu gen <project.json|template-number> <folder>
//	bootmenu img <project.json|template-number> <image>
//	bootmenu read <folder|image>
func main() {
	if len(os.Args) < 3 {
		fmt.Println("használat: bootmenu gen|img <projekt.json|sablonszám> <cél>  |  bootmenu read <mappa|kép>")
		os.Exit(2)
	}
	store := NewStore("drivers")
	if os.Args[1] == "read" {
		var p *Project
		var notes []string
		var err error
		if st, e := os.Stat(os.Args[2]); e == nil && st.IsDir() {
			p, notes, err = ReadFromFolder(os.Args[2])
		} else {
			p, notes, err = ReadFromImage(os.Args[2])
		}
		if err != nil {
			fmt.Println("hiba:", err)
			os.Exit(1)
		}
		for _, n := range notes {
			fmt.Println("megjegyzés:", n)
		}
		p.Save("/dev/stdout")
		return
	}
	var p *Project
	var n int
	if _, err := fmt.Sscanf(os.Args[2], "%d", &n); err == nil && n >= 1 && n <= len(Templates) {
		p = Templates[n-1].Make()
	} else if p, err = LoadProject(os.Args[2]); err != nil {
		fmt.Println("hiba:", err)
		os.Exit(1)
	}
	var r *Report
	var err error
	if os.Args[1] == "img" {
		r, err = WriteToImage(os.Args[3], p, store, true)
	} else {
		r, err = WriteToFolder(os.Args[3], p, store, true)
	}
	if r != nil {
		for _, d := range r.Done {
			fmt.Println("kész:", d)
		}
		for _, w := range r.Warnings {
			fmt.Println("figyelem:", w)
		}
	}
	if err != nil {
		fmt.Println("hiba:", err)
		os.Exit(1)
	}
}
