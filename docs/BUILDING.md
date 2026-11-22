# Building

Go 1.24 or later. No other dependencies: the user interface uses plain Win32
calls, without cgo. The Go code is in the `src` folder.

```
cd src
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w -H windowsgui" -o EasyDOSDeploy.exe .
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
go run ./tools/winres
```

(On Windows, `go generate` does the same.)

## 16-bit code

The MBR boot code of new hard disk images is written in a tiny assembler in
Python; `GEOM.COM` (`asm/geom.asm`) and the FreeDOS boot sector are built
with [NASM](https://nasm.us), which the scripts call. See
[asm/README.md](../asm/README.md).

## GitHub Actions

Every push is tested and built; the program is attached to the run as an
artifact. A `v*` tag (for example `v0.5.0`) also makes a release with a zip.

## Texts and translations

Every text the user sees is written as `T("English text")` in the code. A
test checks that each of them is in the Hungarian table (`src/i18n_hu.go`) and
in the built-in German, French, Spanish and Italian files (`src/lang/*.lng`),
with the same `%s` / `%d` placeholders, and that no menu has two items with the
same `&` key. So a new or changed text needs all five translations.
