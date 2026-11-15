# Building

Go 1.24 or later. No other dependencies: the user interface uses plain Win32
calls, without cgo. The Go code is in the `src` folder.

```
cd src
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -H windowsgui" -o EasyRSB.exe .
go test ./...
```

The build works on any operating system. The tests also run on any operating
system; they cover the menu generator, the importer, the FAT12/16/32 writer,
the image builder, the translations, and the 16-bit code (through a small 8086
emulator).

On other systems than Windows, `go build` makes a small command line program
(`main_other.go`) that is only used for testing.

## Windows resources

The icon, the version information and the manifest come from
`src/rsrc_windows_amd64.syso`. It is committed, so a plain `go build` includes
it. To make it again (for example after changing the version number):

```
cd src
go generate
```

(`go generate` runs `go run ./tools/winres`.)

## 16-bit code

The MBR boot code of new hard disk images and `GEOM.COM` are written in a tiny
assembler in Python; see [asm/README.md](../asm/README.md).

## GitHub Actions

Every push is tested and built; the program is attached to the run as an
artifact. A `v*` tag (for example `v0.5.0`) also makes a release with a zip.
