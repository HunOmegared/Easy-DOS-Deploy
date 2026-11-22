# Making a bootable CF card or IDE disk

This guide is for a CompactFlash card (through a CF-IDE adapter) or an IDE
hard disk that goes into a DOS PC. On the Windows side you need a USB card
reader, or a USB-IDE adapter for a hard disk.

The main window has the steps of a project on the left: *Machine and disk*,
*System*, *Drivers*, *Boot menu*, *Your files* and *Build*. Each shows its
state; a yellow mark means something is still missing. The start-up wizard
(*File > Start wizard*, then *Build a DOS system on a CF card or IDE disk*)
leads through the choices and leaves you on the first step.

**Keep a copy of every disk image and card you work on.**

## 1. Machine and disk

### Why geometry matters

MS-DOS 6.x boots with the cylinders / heads / sectors per track that the BIOS
reports for the drive. These depend on the machine, the adapter and the BIOS
translation mode (NORMAL, LBA or LARGE). A card partitioned with a different
geometry usually does not boot, even if the size is the same. So the image
has to be made with the exact geometry of the **target machine**.

Example: an Amstrad Integra 486 (Phoenix BIOS, 1995) with a 1 GB SiliconDrive
CF card. BIOS setup shows 2030 / 16 / 63. With LBA on, DOS gets 1013 / 32 / 63;
with LBA off, 1022 / 16 / 63.

*Help with the disk profile* on the first page explains the three ways to
find the geometry, with a button for each:

| Way | How |
|---|---|
| **The values in BIOS setup** | *From BIOS setup...* works out what DOS gets in NORMAL, LBA or LARGE mode. |
| **`GEOM.COM`** on the target machine | The most exact way: a tiny DOS program that prints what the BIOS gives. The help makes a FreeDOS boot floppy that runs it by itself, puts it onto your own boot floppy image, or saves it. `MSD.EXE` (Disk Drives) shows the same. |
| **A backup of a card that already boots** on that machine | *From a disk backup...* reads the geometry from the backup. |

Save the geometry as a **disk profile**, and next time you only choose it
from the list.

BIOSes do not all translate the same way. *From BIOS setup...* offers the two
usual LBA methods: **heads doubled** (early BIOSes such as the Phoenix of the
Integra) and **LBA-assisted, up to 255 heads** (most BIOSes from 1996). For
cards up to about 1 GB they give the same result; above that, check with
`GEOM.COM`.

### Big disks (more than 2 GB)

One MS-DOS 6.x FAT16 drive can be at most 2 GB. On a bigger disk the program
makes C: as big as it can be and puts the rest into an extended partition
with logical drives D:, E: ... of at most 2 GB, as FDISK would, so none of the
disk is lost. The page shows the drives and their cluster sizes. DOS can
reach about 8 GB of a disk at most (1024 cylinders × 255 heads × 63 sectors).

Some CD-ROM games only work when the CD-ROM is drive D:. With D: on the disk
the CD-ROM gets the next free letter (the program moves the MSCDEX letter by
itself), so for such games tick *Only drive C:*: the space after C: then
stays unused.

Some early BIOSes give DOS **256 heads** for a big card, and MS-DOS 6.x and
Windows 9x cannot boot from such a disk. `GEOM.COM` warns about it, and the
program refuses to make an image with 256 heads. The way out is to set the
drive in BIOS setup to type **User** with fewer cylinders, so the BIOS gives
fewer heads. *From BIOS setup...* works out the most cylinders DOS can still
use; some old BIOSes do not handle more than 4096 cylinders, so if the drive
is not found after that, use 4096.

Tested: an 8 GB Lexar CF card in the Integra. With *Auto*, BIOS setup shows
15506 / 16 / 63 and DOS gets 967 / **256** / 63, which MS-DOS cannot use. With
type *User* 4096 / 16 / 63, DOS gets 510 / 128 / 63 (2 GB); MSD shows the same,
and the card made from that image boots.

## 2. System: where does DOS come from?

### MS-DOS from its Setup disks

The easiest way for a new system. Give the images of your MS-DOS 6.x Setup
disks (or folders with their files) in *Disks...*: choosing disk 1 adds the
other disks of the set from the same folder. The program expands the
compressed files into C:\DOS, as Setup would, and takes the boot code and the
system files from disk 1. Setup's choices are there: Backup, Anti-Virus and
Undelete for MS-DOS, Windows, both or none (MS-DOS by default, as Setup
offers without Windows), and DriveSpace (off: a CF card does not need disk
compression).

### FreeDOS, downloaded by the program

For an empty card and no DOS disks at all. The program downloads the FreeDOS
kernel and the FreeCOM command shell from their official releases on GitHub
(GNU GPL), and the programs the boot menu loads (JemmEx, CuteMouse, SHSUCDX),
and makes a bootable FreeDOS system with the menu. The files are kept in the
`drivers` folder for next time.

### A disk or image that already boots DOS

A card that boots on the machine (back it up first: *Disk image > Back up a
card to an image file*), a backup image of one, a hard disk image, or a DOS
boot floppy image. Its boot code and system files are taken; tick *Copy all
other files* to take the DOS folder and the rest too. A backup can be the
source for a different card: only the geometry changes. A floppy has no MBR,
so the program writes its own MBR boot code.

**The machine's own drivers.** The program reads the CONFIG.SYS and
AUTOEXEC.BAT of the source and finds the drivers the machine already uses:
mouse, CD-ROM (the device driver and MSCDEX), sound (for example CTSB16.SYS,
DIAGNOSE and the BLASTER line of a Sound Blaster 16, or ULTRINIT and ULTRASND
of a Gravis UltraSound), keyboard and code page. With *Use the drivers of the
system source* (on by default when something is found) the new menu loads
these instead of the program's own choices, so the drivers known to work on
that machine stay. The high-loading options MEMMAKER wrote (`LH /L:...`,
`DEVICEHIGH /L:...`) are left out; each menu item loads high again by its own
memory setting. A batch file started with `CALL` counts as what it loads.

A driver the machine has but does not load is used too: a line commented out
with `REM` (if the file it names is there), or a CD-ROM or mouse driver file
that no line names (for example `VIDE-CDD.SYS` next to an `MSCDEX` line with
no CD driver in CONFIG.SYS). The CD driver and MSCDEX get the same device
name. Drivers of the program's list that are on the source, such as
`OAKCDROM.SYS`, are taken from it, so they need not be browsed for.

### I'll install DOS myself (empty image)

An empty image with the right geometry, for installing DOS in 86Box (or PCem)
with its own Setup: attach the image as a hard disk with the **same**
geometry. Afterwards the boot menu and the drivers can be written into it
with *Output > Write to disk image*.

## 3. Drivers, 4. Boot menu, 5. Your files

* **Drivers:** tick what goes onto the disk; the ready menus tick what they
  need. See [Drivers](DRIVERS.md), also for adding your own driver.
* **Boot menu:** choose a ready menu, or *Edit the menu...* for the full
  editor.
* **Your files:** add folders and files of the PC (games, utilities, Norton
  Commander ...) and choose the drive and folder each goes into. Long names
  become 8.3 names as DOS shows them (`Norton Commander` → `NORTON~1`).

## 6. Build: try it, then write it

1. *Create* makes the image file (without a name, it asks where to save it).
   Big images are made quickly: the unused space takes no room on the PC.
2. Try the image in 86Box with the **same** geometry. If it boots there and the
   menu works, it will boot on the real machine with the same BIOS settings.
3. *Write to disk...* (or *Disk image > Write an image to a card*). This needs
   administrator rights; the program offers to restart itself and keeps the
   project.
   * Only removable disks are listed: CF and SD cards in a reader, IDE disks
     on a USB adapter, and USB drives, with their kind (USB, SD). The Windows
     system disk is never listed.
   * Before writing, the program shows what is on the disk now, warns loudly
     about Windows, Linux or GPT partitions, and asks you to type the disk
     number.
   * Only the used parts of every drive are written, and everything written
     is read back and compared.

Do not change the BIOS drive settings afterwards, or the card will not boot.

**Save the project** (*File > Save*): it keeps every step, so the same
system can be built again, or for another card by changing only the
geometry.

## IDE hard disks on a USB-IDE adapter

The same steps work, as long as the adapter can read the drive. Very old
drives that only know CHS addressing often do not work on USB adapters.
