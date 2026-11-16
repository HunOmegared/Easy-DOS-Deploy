package main

import (
	"fmt"
	"strings"
)

// Marker written into generated files so they can be recognised on import.
const genMarker = "REM -- Easy DOS Deploy --"

// oldMarkers are what earlier versions wrote (under earlier names of the
// program); still recognised on import.
var oldMarkers = []string{"REM -- Easy DDT --", "REM -- Easy Retro System Builder --", "REM -- DOS Boot Menu Builder --"}

const endLabel = "BMEND"

// fdEndMarker separates the "common, before" and "common, after" lines in
// a FreeDOS file (they are otherwise indistinguishable).
const fdEndMarker = "REM -- kozos sorok vege --"

// Output is the result of a generation: file name -> lines.
type Output struct {
	ConfigName, AutoexecName string
	Config, Autoexec         []string
}

func (o *Output) Files(cp Codepage) map[string][]byte {
	return map[string][]byte{
		o.ConfigName:   EncodeFile(o.Config, cp),
		o.AutoexecName: EncodeFile(o.Autoexec, cp),
	}
}

func title(it Item) string {
	t := strings.TrimSpace(it.Title)
	if t == "" {
		return it.ID
	}
	return t
}

// Generate builds the two files. Call Validate first; Generate refuses fatal problems.
func Generate(p *Project) (*Output, error) {
	if ps := p.Validate(); HasFatal(ps) {
		for _, x := range ps {
			if x.Fatal {
				return nil, fmt.Errorf("%s", x.Msg)
			}
		}
	}
	o := &Output{ConfigName: p.ConfigFile(), AutoexecName: p.AutoexecFile()}
	if p.Target == TargetFreeDOS {
		o.Config = genFreeDOSConfig(p)
	} else {
		o.Config = genMSDOSConfig(p)
	}
	o.Autoexec = genAutoexec(p)
	return o, nil
}

func genMSDOSConfig(p *Project) []string {
	var c []string
	c = append(c, genMarker, "[MENU]")
	for _, it := range p.Items {
		c = append(c, fmt.Sprintf("MENUITEM=%s, %s", it.ID, title(it)))
	}
	def := p.Items[p.Default].ID
	if p.Timeout >= 0 {
		c = append(c, fmt.Sprintf("MENUDEFAULT=%s, %d", def, p.Timeout))
	} else {
		c = append(c, "MENUDEFAULT="+def)
	}
	if p.MenuColor != "" {
		c = append(c, "MENUCOLOR="+strings.ReplaceAll(p.MenuColor, " ", ""))
	}
	c = append(c, "", "[COMMON]")
	c = append(c, p.Start.Config...)
	for _, it := range p.Items {
		c = append(c, "", "["+it.ID+"]")
		c = append(c, it.Config...)
	}
	// A trailing [COMMON] block is what Microsoft recommends, so that
	// installers that append lines put them where every item sees them.
	c = append(c, "", "[COMMON]")
	c = append(c, p.End.Config...)
	return c
}

func genFreeDOSConfig(p *Project) []string {
	var c []string
	c = append(c, genMarker)
	if p.MenuColor != "" {
		c = append(c, "MENUCOLOR="+strings.ReplaceAll(p.MenuColor, " ", ""))
	}
	if p.Timeout >= 0 {
		c = append(c, fmt.Sprintf("MENUDEFAULT=%d,%d", p.Default+1, p.Timeout))
	} else {
		c = append(c, fmt.Sprintf("MENUDEFAULT=%d", p.Default+1))
	}
	c = append(c, "MENU")
	for i, it := range p.Items {
		c = append(c, fmt.Sprintf("MENU %d - %s", i+1, title(it)))
	}
	c = append(c, "MENU", "")
	c = append(c, p.Start.Config...)
	for i, it := range p.Items {
		if len(it.Config) == 0 {
			continue
		}
		c = append(c, "", "REM "+it.ID)
		for _, l := range it.Config {
			if strings.TrimSpace(l) == "" {
				continue // an empty "1?" line would be meaningless
			}
			c = append(c, fmt.Sprintf("%d?%s", i+1, l))
		}
	}
	if len(p.End.Config) > 0 {
		c = append(c, "", fdEndMarker)
		c = append(c, p.End.Config...)
	}
	return c
}

// configValue is what %CONFIG% contains for item i.
func configValue(p *Project, i int) string {
	if p.Target == TargetFreeDOS {
		return fmt.Sprint(i + 1)
	}
	return p.Items[i].ID
}

func genAutoexec(p *Project) []string {
	var a []string
	start := p.Start.Autoexec
	// Keep "@ECHO OFF" as the very first line so the marker is not echoed.
	if len(start) > 0 && strings.EqualFold(strings.TrimSpace(start[0]), "@ECHO OFF") {
		a = append(a, start[0])
		start = start[1:]
	}
	a = append(a, "@"+genMarker)
	a = append(a, start...)
	for i, it := range p.Items {
		a = append(a, fmt.Sprintf(`IF "%%CONFIG%%"=="%s" GOTO %s`, configValue(p, i), it.ID))
	}
	a = append(a, "GOTO "+endLabel)
	for _, it := range p.Items {
		a = append(a, "", ":"+it.ID)
		a = append(a, it.Autoexec...)
		a = append(a, "GOTO "+endLabel)
	}
	a = append(a, "", ":"+endLabel)
	a = append(a, p.End.Autoexec...)
	return a
}
