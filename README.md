# Easy Retro System Builder

A Windows program that builds the **boot menu** of a DOS PC and puts it, with
the drivers it needs, onto a **disk image** or straight onto a **CompactFlash
card**. It writes the CONFIG.SYS / AUTOEXEC.BAT pair for MS-DOS 6.x and the
FDCONFIG.SYS style menu for FreeDOS, so you do not have to remember the
`[MENU]` / `MENUITEM` syntax or the `%CONFIG%` branches, and the files always
have DOS line endings.

![The main window: menu items on the left, their CONFIG.SYS and AUTOEXEC.BAT lines on the right](docs/screenshot.png)

## Status

Early version (0.x). The file generation, the importer, the FAT12/16/32 writer
and the image builder have automated tests. The Windows user interface, the
raw card reading/writing and booting the results on real machines are being
tested now. **Keep a copy of every disk image and card you work on.**

## What it does

* **Boot menu editor.** Menu items with their own CONFIG.SYS and AUTOEXEC.BAT
  lines, plus common lines before and after them, a default item and a
  timeout. Output for **MS-DOS 6.x** (`[MENU]` blocks) or **FreeDOS**
  (`MENU` lines and `1?` prefixes). Preview before writing.
* **Templates:** Easy Commander with networking or plain DOS, a games menu
  (EMS / no EMS / clean), with or without CD-ROM, for both systems.
* **Import an existing menu** from a disk image or a folder, including
  `INCLUDE=` blocks and both `IF "%CONFIG%"==` and `GOTO %CONFIG%` styles.
  Anything that cannot be mapped is listed, not silently dropped.
* **Drivers.** A built-in list of freely available drivers (Crynwr NE2000 and
  3C509 packet drivers, mTCP, CuteMouse, JemmEx, HimemX, UDVD2, SHSUCDX) that
  can be downloaded from the FreeDOS 1.3 package repository. If a download fails
  the program says where to get the file and lets you browse for it (a file or
  a zip). The list can be edited in the program.
* **Write into disk images** without mounting them: raw floppy images, hard
  disk images with an MBR (86Box `.img`) and fixed-size `.vhd`, FAT12/16/32.
  The previous CONFIG/AUTOEXEC is kept as `.BAK`.
* **Make bootable disk images:** floppies from 360 KB to 2.88 MB, and hard
  disk / CF images with the exact cylinder/head/sector geometry the target
  machine's BIOS gives to DOS. The boot code and system files are taken from a
  bootable image you already have (for example a backup of a working card), so
  no MS-DOS files are distributed with the program.
* **CF cards:** back up a card from a card reader to an image file, read its
  geometry from the backup, and write an image back to a card. Only removable,
  USB and SD disks are offered, never the Windows disk. Before writing, the
  program shows what is on the disk now, warns loudly if it finds a Windows,
  Linux or GPT partition, and asks you to type the disk number. Empty areas are
  skipped and everything written is read back and compared. This also works
  with an IDE hard disk on a USB-IDE adapter (as long as the adapter can read
  the drive; very old CHS-only drives often cannot).
* **English and Hungarian** user interface; other languages can be added with a
  plain text file (Language > Make a translation file).

## Making a bootable CF card

The catch with CF cards is geometry: MS-DOS 6.22 boots with the
cylinders/heads/sectors that the BIOS reports, and these depend on the
machine, the adapter and the BIOS translation mode (NORMAL / LBA / LARGE).

1. Put the card into the target machine and boot DOS. Run `MSD.EXE`
   (Disk Drives) to see the geometry DOS gets, for example 1013 / 32 / 63.
   If the card already boots on that machine, a backup of it holds the same
   information (step 2).
2. *Disk image > Back up a card to an image file.* The program offers to
   read the geometry, boot code and system from the backup.
3. *Disk image > New bootable disk image:* check the geometry, choose the
   system source, and optionally copy the rest of its files and add the
   current menu with drivers. Save the geometry as a profile.
4. Try the image in 86Box with the **same** geometry.
5. *Disk image > Write an image to a card.* Needs administrator rights; the
   program offers to restart itself.

Do not change the BIOS drive settings afterwards, or the card will not boot.

## Download

Windows 10 or later, 64-bit. Download `EasyRSB.exe` from the
[Releases](../../releases) page. It is a single file; no installation.
Downloaded drivers, language files and settings are kept next to the program
if that folder is writable (portable use), otherwise in
`%APPDATA%\EasyRetroSystemBuilder`.

## Building

Go 1.24 or later, no other dependencies (plain Win32 calls, no cgo):

```
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H windowsgui" -o EasyRSB.exe .
go test ./...
```

The tests run on any operating system. GitHub Actions builds the program for
every push and attaches it to a release for every `v*` tag.

## Translations

*Language > Make a translation file* writes a text file with every English
text and an empty line for the translation. Name it after the language code
(`de.lng`), put it into the `lang` folder and choose *Language > Reload
language files*. Pull requests with new languages are welcome.

## License

GNU General Public License v3.0, see [LICENSE](LICENSE).

The drivers the program can download are separate programs under their own
licenses (mostly GPL or freeware) and are downloaded from their usual sources;
nothing of them is included here. MS-DOS is a trademark of Microsoft; no
Microsoft files are included or downloaded.
