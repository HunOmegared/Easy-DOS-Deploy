# Old IDE disks that a USB-IDE adapter cannot handle

Easy DOS Deploy writes a disk through a USB card reader or a USB-IDE adapter.
These adapters address the disk by **LBA**, and most IDE disks from the
mid-1990s on can do that. Many older disks, often smaller than 504 MB, only
know **CHS** addressing: the adapter does not see them, or sees a wrong size.

**Check first:** connect the disk through the adapter and look at it in
Windows Disk Management, or in the disk list of *Write to disk...*. If it is
there with the right size, the normal way works (see the
[CF card guide](CF-GUIDE.md)). If not, use one of the ways below.

**Keep a copy of everything on the old disk before you start.** Partitioning
and formatting erase it.

## Way 1: a CF card as a helper (recommended)

You build the system on a CompactFlash card as usual, boot the target machine
from the card, and copy the system onto the old disk there.

You need a CF card with a CF-IDE adapter, and a free place on the IDE cable
of the target machine.

1. **Build the system on the CF card** with Easy DOS Deploy, with the
   geometry the target machine gives to *the card* (see the CF card guide).
   Your files can go onto it too, as long as they fit on the old disk later.
   Try it in 86Box, then write it to the card.
2. **Put both into the target machine:** the CF card as **master** (the first
   disk), the old disk as **slave** on the same cable, or as master on the
   second cable. Set the jumpers of the old disk accordingly. In BIOS setup
   let the BIOS detect both drives (Auto, or the old disk's own values from
   its label; for a CHS-only disk use NORMAL mode, not LBA).
3. **Boot from the CF card.** In the boot menu choose a plain item without
   drivers; the card is C:.
4. **Partition the old disk:** run `FDISK`.
   * Choose *5. Change current fixed disk drive* and select disk **2**. Make
     sure the size shown is the old disk's, not the card's.
   * *1. Create DOS partition*, then *1. Create Primary DOS Partition*, with
     the largest size (at most 2 GB for MS-DOS 6.x).
   * *2. Set active partition*, partition 1 of disk 2. Some FDISK versions
     answer that only partitions on drive 1 can be made active: then leave
     it, it is done in step 7.
   * Leave FDISK; the machine restarts. Boot from the card again.
5. **Format the old disk with the system on it.** DOS gives the old disk's
   partition the letter **D:** (DOS names the first partition of every disk
   before the logical drives, so this holds even if the card has more
   drives):

   ```
   FORMAT D: /S
   ```

6. **Copy the system and the rest:**

   ```
   XCOPY C:\*.* D:\ /S /E
   ```

   `FORMAT /S` already put the hidden system files (IO.SYS, MSDOS.SYS) and
   COMMAND.COM onto D:; XCOPY copies the folders (DOS, DRIVERS, your games),
   CONFIG.SYS and AUTOEXEC.BAT. The lines in them name `C:\...`, which is
   right: the old disk becomes C: once the card is removed.
7. **Make the old disk the only disk:** switch the machine off, remove the
   CF card, set the old disk's jumper to **master** (or single), let the BIOS
   detect it again, and boot.

**If it does not boot** ("Missing operating system", "No ROM BASIC", or
nothing happens), the partition is not active yet, or the disk has no DOS
boot code in its master boot record. Boot from a DOS floppy with FDISK on it,
with the old disk as the only disk:

* run `FDISK`, *2. Set active partition*, partition 1;
* run `FDISK /MBR` (it writes the boot code of the first disk);

then restart.

**FreeDOS:** the same steps work. `FORMAT D: /S` becomes `FORMAT D:` followed
by `SYS D:`, and the configuration file is `FDCONFIG.SYS`.

## Way 2: install DOS on the target machine and bring the rest

Without a CF card. You prepare the old disk on the target machine itself and
use Easy DOS Deploy only for the boot menu and the drivers.

1. Boot the target machine from your **DOS Setup disk 1** (or a DOS boot
   floppy), let Setup partition and format the disk and install DOS (or run
   `FDISK`, then `FORMAT C: /S`).
2. In Easy DOS Deploy, make the boot menu and choose the drivers, then
   *Output > Save to folder...*, and answer *Yes* when it asks about the
   drivers: the CONFIG.SYS, AUTOEXEC.BAT and the drivers go into a folder on
   the PC, with the same folders as on the DOS disk (`DRIVERS\...`). Your
   own files (games, utilities) are on the PC already; bring them the same
   way.
3. Bring the folder to the target machine and copy it onto C::
   * on **floppies**, if it fits;
   * over the **network** with
     [Easy Commander](https://github.com/HunOmegared/Easy-Commander) (a
     packet driver and mTCP on the DOS machine), which is also the easiest way
     for big games;
   * or with a **serial or parallel cable** (INTERLNK/INTERSVR of MS-DOS 6.x).
4. Restart the target machine: the boot menu appears.

## Way 3: a PC with a real IDE port

An older PC whose motherboard has an IDE port can reach the old disk
directly. Build the image with Easy DOS Deploy as usual (with the geometry
the target machine gives to the old disk), then write it there with a raw
disk tool, for example from a Linux live USB stick:

```
dd if=dos.img of=/dev/sdX bs=1M
```

where `/dev/sdX` is the old disk. **Check it twice** (`lsblk` shows the
disks and their sizes): a wrong name overwrites another disk. Easy DOS Deploy
itself needs Windows 10 or later, which such PCs rarely run.

## Which way?

| You have | Use |
|---|---|
| A CF card and a CF-IDE adapter | Way 1: the whole system, built and tried in 86Box first |
| DOS Setup floppies and a floppy drive in the target machine | Way 2, with the network or floppies for the rest |
| An older PC with an IDE port | Way 3 |
