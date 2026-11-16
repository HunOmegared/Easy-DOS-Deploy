package main

import (
	"fmt"
	"strings"
)

// FreeDOS made by the program itself, without a system source: the boot
// sector is built in (fdboot.go), the kernel and the command shell come from
// the official FreeDOS releases on GitHub through the driver catalog.
const (
	fdKernelID  = "fdkernel"
	fdCommandID = "freecom"
)

// fdTools are the programs the FreeDOS building blocks load from
// C:\FREEDOS\BIN (a full FreeDOS installation has them there), with the
// catalog entry each comes from.
var fdTools = []struct{ File, ID string }{
	{"JEMMEX.EXE", "jemm"},
	{"JEMM386.EXE", "jemm"},
	{"HIMEMX.EXE", "himemx"},
	{"CTMOUSE.EXE", "ctmouse"},
	{"SHSUCDX.COM", "shsucdx"},
	{"UDVD2.SYS", "udvd2"},
}

// MakeGeomFloppy makes a 1.44 MB FreeDOS boot floppy image that runs
// GEOM.COM at start; files are those of FreeDOSFiles (only the kernel and
// the command shell in the root are used).
func MakeGeomFloppy(path string, files map[string][]byte) (*Report, error) {
	sys := map[string][]byte{"KERNEL.SYS": files["KERNEL.SYS"], "COMMAND.COM": files["COMMAND.COM"]}
	if sys["KERNEL.SYS"] == nil || sys["COMMAND.COM"] == nil {
		return nil, fmt.Errorf("%s", T("FreeDOS: the kernel (KERNEL.SYS) or the command shell (COMMAND.COM) is missing."))
	}
	sys["GEOM.COM"] = geomCOM
	sys["AUTOEXEC.BAT"] = []byte("@ECHO OFF\r\nPATH A:\\\r\nPROMPT $P$G\r\nCLS\r\nECHO Easy DOS Deploy - geometry floppy (FreeDOS)\r\nECHO.\r\nGEOM\r\n")
	return CreateBootImage(BootOptions{Path: path, Kind: MediaF144, Label: "GEOM", FreeDOS: sys})
}

// fdBootSector is the FreeDOS boot code for a FAT type.
func fdBootSector(fatType int) ([]byte, error) {
	switch fatType {
	case 12:
		return fdBoot12, nil
	case 16:
		return fdBoot16, nil
	}
	return nil, fmt.Errorf("FreeDOS: FAT%d is not supported", fatType)
}

// FreeDOSFiles are the files of a small FreeDOS system for the project's
// menu: the kernel, the command shell (in the root and in C:\FREEDOS\BIN, as
// the generated FDCONFIG.SYS expects), and the tools its lines load from
// C:\FREEDOS\BIN. Missing lists the IDs of the catalog entries still to be
// downloaded.
func FreeDOSFiles(p *Project, s *Store) (files map[string][]byte, missing []string) {
	files = map[string][]byte{}
	need := []struct{ dest, file, id string }{
		{"KERNEL.SYS", "KERNEL.SYS", fdKernelID},
		{"COMMAND.COM", "COMMAND.COM", fdCommandID},
		{`FREEDOS\BIN\COMMAND.COM`, "COMMAND.COM", fdCommandID},
	}
	if p != nil {
		if o, err := Generate(p); err == nil {
			text := strings.ToUpper(strings.Join(o.Config, "\n") + "\n" + strings.Join(o.Autoexec, "\n"))
			for _, t := range fdTools {
				if strings.Contains(text, `\FREEDOS\BIN\`+t.File) {
					need = append(need, struct{ dest, file, id string }{`FREEDOS\BIN\` + t.File, t.File, t.ID})
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, n := range need {
		d := s.Find(n.id)
		var data []byte
		err := fmt.Errorf("unknown")
		if d != nil {
			data, err = s.Data(d, DriverFile{Name: n.file})
		}
		if err != nil {
			if !seen[n.id] {
				seen[n.id] = true
				missing = append(missing, n.id)
			}
			continue
		}
		files[n.dest] = data
	}
	return files, missing
}
