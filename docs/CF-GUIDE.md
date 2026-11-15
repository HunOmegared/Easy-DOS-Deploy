# Making a bootable CF card or IDE disk

This guide is for a CompactFlash card (through a CF-IDE adapter) or an IDE
hard disk that goes into a DOS PC. On the Windows side you need a USB card
reader, or a USB-IDE adapter for a hard disk.

**Keep a copy of every disk image and card you work on.**

## Why geometry matters

MS-DOS 6.x boots with the cylinders / heads / sectors per track that the BIOS
reports for the drive. These depend on the machine, the adapter and the BIOS
translation mode (NORMAL, LBA or LARGE). A card partitioned with a different
geometry usually does not boot, even if the size is the same. So the image
has to be made with the exact geometry of the **target machine**.

Example: an Amstrad Integra 486 (Phoenix BIOS, 1995) with a 1 GB SiliconDrive
CF card. BIOS setup shows 2030 / 16 / 63. With LBA on, DOS gets 1013 / 32 / 63;
with LBA off, 1022 / 16 / 63.

There are three ways to find the geometry:

| Way | When to use it |
|---|---|
| **A backup of a card that already boots** on that machine | The most reliable: the program reads the geometry from the backup. |
| **`MSD.EXE`** (Disk Drives) or **`GEOM.COM`** on the target machine | `GEOM.COM` is a tiny DOS program that prints what the BIOS gives. *Disk image > Save GEOM.COM* or *Put GEOM.COM onto a boot floppy image*. |
| **The values in BIOS setup** | *From BIOS setup...* in the image window works out what DOS gets in NORMAL, LBA or LARGE mode. |

Save the geometry as a **profile**, and you do not have to type it again.

BIOSes do not all translate the same way. *From BIOS setup...* offers the two
usual LBA methods: **heads doubled** (early BIOSes such as the Phoenix of the
Integra) and **LBA-assisted, up to 255 heads** (most BIOSes from 1996). For
cards up to about 1 GB they give the same result; above that, check with
`GEOM.COM`.

## Big cards (more than 2 GB)

MS-DOS 6.x can use one FAT16 partition of at most 2 GB, so the image of a big
card uses only its first 2 GB; the rest of the card stays unused. The program
cuts the geometry to 2 GB by itself when it comes from *From BIOS setup...*.

Some early BIOSes give DOS **256 heads** for a big card, and MS-DOS 6.x and
Windows 9x cannot boot from such a disk. `GEOM.COM` warns about it, and the
program refuses to make an image with 256 heads. The way out is to set the
drive in BIOS setup to type **User** with fewer cylinders, so the BIOS gives
fewer heads. *From BIOS setup...* works out the number.

Tested: an 8 GB Lexar CF card in the Integra. With *Auto*, BIOS setup shows
15506 / 16 / 63 and DOS gets 967 / **256** / 63, which MS-DOS cannot use. With
type *User* 4096 / 16 / 63, DOS gets 510 / 128 / 63 (2 GB); MSD shows the same,
and the card made from that image boots.

## Where does DOS come from?

The start-up wizard (*File > Start wizard*, then *Make a bootable CF card or
IDE disk*) asks this. The three answers:

### 1. From a working card, or a backup of one

You already have a card that boots on the machine, or a backup image of one.
A backup can be the source for a different card too: only the geometry
changes.

1. *Disk image > Back up a card to an image file.* The program offers to use
   the backup as the source: geometry, boot code and system files.
2. *Disk image > New bootable disk image:* check the geometry, choose the boot
   menu, and optionally copy the rest of the files from the backup.

### 2. From the DOS install disks

You have the MS-DOS (or FreeDOS) install floppies, but no working system.

1. The program makes an **empty image** of the exact size.
2. In 86Box (or PCem), attach the image as a hard disk with the **same**
   geometry and install DOS with its own Setup.
3. Back in the program, write the boot menu and the drivers into the image
   (*Output > Write to disk image*).

### 3. From a DOS boot floppy image

You have a bootable DOS floppy image (for example an MS-DOS 6.22 boot disk).

The program copies the system files (and, if you like, every other file) from
the floppy image to a new hard disk image. A floppy has no MBR, so the program
writes its own MBR boot code.

## Try it, then write it

1. Try the image in 86Box with the **same** geometry. If it boots there and the
   menu works, it will boot on the real machine with the same BIOS settings.
2. *Disk image > Write an image to a card.* This needs administrator rights;
   the program offers to restart itself.
   * Only removable, USB and SD disks are offered, never the Windows disk.
   * Before writing, the program shows what is on the disk now, warns loudly
     about Windows, Linux or GPT partitions, and asks you to type the disk
     number.
   * Empty areas are skipped, and everything written is read back and compared.

Do not change the BIOS drive settings afterwards, or the card will not boot.

## IDE hard disks on a USB-IDE adapter

The same steps work, as long as the adapter can read the drive. Very old
drives that only know CHS addressing often do not work on USB adapters.
