//go:build !windows

package main

import "os"

// makeSparse: other systems make files sparse by themselves.
func makeSparse(f *os.File) {}
