//go:build windows

package main

// The icon, version information and manifest are in rsrc_windows_amd64.syso.
//go:generate go run ./tools/winres

func main() { runGUI() }
