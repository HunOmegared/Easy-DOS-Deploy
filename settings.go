package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings are remembered between runs.
type Settings struct {
	Lang     string        `json:"lang"`
	Profiles []GeomProfile `json:"profiles,omitempty"`
}

// GeomProfile remembers the geometry of a card in a machine.
type GeomProfile struct {
	Name  string `json:"name"`
	C     uint32 `json:"c"`
	H     uint32 `json:"h"`
	S     uint32 `json:"s"`
	Total int64  `json:"total,omitempty"`
}

// dataDir is next to the program when that folder is writable (portable
// use, e.g. from a USB stick); otherwise the user's settings folder.
func dataDir(exeDir string) string {
	test := filepath.Join(exeDir, ".write-test")
	if f, err := os.Create(test); err == nil {
		f.Close()
		os.Remove(test)
		return exeDir
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "EasyRetroSystemBuilder")
	}
	return exeDir
}

func loadSettings(dir string) Settings {
	var s Settings
	if b, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil {
		json.Unmarshal(b, &s)
	}
	return s
}

func saveSettings(dir string, s Settings) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(filepath.Join(dir, "settings.json"), b, 0644)
}
