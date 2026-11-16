# Drivers

The program has a built-in list of DOS drivers and tools. Those that are free
to distribute can be downloaded from the FreeDOS 1.3 package repository (with
a mirror as a fallback). The others are **browsed for**: you choose a file, a
zip, or a disk image (for example a Windows 98 boot floppy image), and the
program searches it for the file it needs.

Open the list with *Drivers > Manage drivers*. Every entry can be edited, and new
entries can be added; *Restore built-in list* brings it back.

| Driver | What it is for | How to get it |
|---|---|---|
| NE2000 packet driver (Crynwr) | the default network card of 86Box/PCem and real NE2000 cards | download |
| 3C509 packet driver (Crynwr) | 3Com EtherLink III cards | download |
| mTCP (DHCP, PING, FTP, HTGET) | TCP/IP programs by Michael Brutman | download |
| MTCP.CFG sample | a basic mTCP setup (packet driver at 0x60, DHCP) | made by the program |
| Easy Commander client (EC.EXE) | file transfer from a modern PC | browse (from its release) |
| CuteMouse | PS/2 and serial mouse | download |
| JemmEx / Jemm386 | memory manager, replaces HIMEM + EMM386 (386+) | download (GitHub release) |
| HimemX | free HIMEM.SYS replacement | download |
| OAKCDROM.SYS | generic ATAPI CD-ROM driver, works on old ISA controllers too | browse: Windows 98 boot disk, or `WINDOWS\COMMAND\EBD` |
| UNISOUND | sets up ISA PnP sound cards (ESS, Creative, Yamaha, Crystal...) | browse: download it from the [Vogons topic](https://www.vogons.org/viewtopic.php?p=833659) |
| UDVD2 | IDE/ATAPI CD/DVD driver | download |
| SHSUCDX | free MSCDEX replacement | download |
| FreeDOS kernel (KERNEL.SYS) | the FreeDOS system, kernel 2043 (8086+, FAT12/FAT16) | download (GitHub release) |
| FreeCOM (COMMAND.COM) | the FreeDOS command shell, 0.86 | download (GitHub release) |

The drivers are separate programs under their own licenses (mostly GPL or
freeware). Nothing of them is included in this repository.

The FreeDOS kernel and FreeCOM are used for FreeDOS disks made without a
system source; the program downloads them when needed. With them it also puts
JemmEx, CuteMouse and SHSUCDX into `C:\FREEDOS\BIN` when the menu loads them
from there, as on a full FreeDOS installation.

Downloaded files are kept in the `drivers` folder: next to the program if that
folder is writable, otherwise in `%APPDATA%\EasyDOSDeploy\drivers`.

If a menu item uses a driver that is missing, the program says so before
writing, and offers to download or browse for it.
