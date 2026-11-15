# Easy DDT — DOS Deployment Tool

A Windows program that builds the **boot menu** of a DOS PC and puts it, with
the drivers it needs, onto a **disk image** or straight onto a **CompactFlash
card**. It writes the CONFIG.SYS / AUTOEXEC.BAT pair for MS-DOS 6.x and the
FDCONFIG.SYS style menu for FreeDOS, so you do not have to remember the
`[MENU]` / `MENUITEM` syntax or the `%CONFIG%` branches, and the files always
have DOS line endings.

![The main window: menu items on the left, their CONFIG.SYS and AUTOEXEC.BAT lines on the right](docs/screenshot.png)

## Status

Early version (0.x). Earlier versions were called Easy Retro System Builder (EasyRSB);
the program still recognises the boot menus and settings they made. Tested end to end on real hardware: a backup of a
working 1 GB CompactFlash card (MS-DOS 6.22) was made through a USB card
reader, a new bootable image with a generated boot menu was built from it
with the geometry the BIOS gives to DOS (1013/32/63, LBA mode), tried in
86Box, written back to the card, and an **Amstrad Integra 486 (Phoenix BIOS)**
booted from it with the menu working. The same system was then put onto an
8 GB Lexar card with a different geometry (510/128/63, 2 GB) and booted on the
same machine. `GEOM.COM` and a FreeDOS boot floppy made by the program's own
FAT code were also tried there. The file generation, the importer, the
FAT12/16/32 writer, the image builder and the 16-bit code also have automated
tests. FreeDOS hard disk images and other machines have not been tested on real
hardware yet.
**Keep a copy of every disk image and card you work on.**

## What it does

* **Boot menu editor** for **MS-DOS 6.x** (`[MENU]` blocks) and **FreeDOS**
  (`MENU` lines and `1?` prefixes), with a default item, a timeout and a
  preview before writing.
* **Start-up wizard:** choose what to make (bootable floppy, CF card or IDE
  disk, emulator image, or a new menu for an existing system), then the DOS and
  the kind of menu.
* **Building blocks instead of typing:** memory manager, mouse, CD-ROM, Sound
  Blaster (with UNISOUND for ISA PnP cards) and network; the program writes the
  lines for MS-DOS or FreeDOS. Ready combinations from plain DOS to games and
  network. The lines can still be edited by hand.
* **Import an existing menu** from a disk image or a folder. Anything that
  cannot be mapped is listed, not silently dropped.
* **Drivers:** free drivers are downloaded, the others (such as OAKCDROM.SYS)
  are browsed for in a file, a zip or a disk image. See [Drivers](docs/DRIVERS.md).
* **Write into disk images** without mounting them: floppy images, hard disk
  images (86Box `.img`) and fixed-size `.vhd`, FAT12/16/32. The old
  CONFIG/AUTOEXEC is kept as `.BAK`.
* **Make bootable disk images:** floppies from 360 KB to 2.88 MB, and hard disk
  / CF images with the exact geometry the target BIOS gives to DOS. The system
  comes from a disk you already have; no MS-DOS files are distributed with the
  program.
* **CF cards and IDE disks:** back up a card to an image, and write an image
  back, safely. See the [CF card guide](docs/CF-GUIDE.md).
* **User interface in English, German, French, Spanish, Italian and
  Hungarian;** other languages can be added with a text file. See
  [Translating](docs/TRANSLATING.md).

## Quick start

1. Download `EasyDDT.exe` from the [Releases](../../releases) page (Windows 10
   or later, 64-bit). It is a single file; no installation.
2. Start it. The wizard asks what you want to make.
3. For a CF card or an IDE disk, read the [CF card guide](docs/CF-GUIDE.md)
   first: the geometry of the target machine matters.

Downloaded drivers, language files and settings are kept next to the program if
that folder is writable (portable use), otherwise in
`%APPDATA%\EasyDDT`.

## Documentation

| Document | Content |
|---|---|
| [CF card guide](docs/CF-GUIDE.md) | geometry, the three ways to start, trying in 86Box, writing the card |
| [Drivers](docs/DRIVERS.md) | the built-in driver list and where each driver comes from |
| [Translating](docs/TRANSLATING.md) | adding a new language |
| [Building](docs/BUILDING.md) | building and testing from source |

## Repository layout

| Folder | Content |
|---|---|
| `src/` | the Windows program (Go) and its tests |
| `src/lang/` | the built-in German, French, Spanish and Italian translations |
| `src/tools/winres/` | makes the icon, version information and manifest of the exe |
| `asm/` | the 16-bit code: the MBR boot code and `GEOM.COM`, with a tiny assembler |
| `docs/` | the guides above and the screenshot |
| `.github/workflows/` | the GitHub Actions build |

## License

Copyright (C) 2026 Daniel Szentmiklosi.

GNU General Public License v3.0, see [LICENSE](LICENSE).

The drivers the program can download are separate programs under their own
licenses (mostly GPL or freeware) and are downloaded from their usual sources;
nothing of them is included here. MS-DOS is a trademark of Microsoft; no
Microsoft files are included or downloaded.
