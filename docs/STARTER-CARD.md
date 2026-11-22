# A starter card for almost any PC

A starter card is a CompactFlash card that boots DOS on nearly every PC with
an IDE port, 286s and 386s without auto-detection included, **without a
floppy drive, a network or an XT-IDE card**. It also has `GEOM.COM` on it, so
on the target machine you can find the geometry its BIOS gives to DOS, and
later build a card that uses the whole of it.

How it works: the card is made with a plain CHS geometry, **16 heads and 63
sectors per track**, that every BIOS with IDE support can take as a
user-defined drive type, and that nearly every CF card accepts. The price is
the size: DOS sees about **500 MB** of the card, whatever its real size.

**Keep a copy of everything on the card before you start.** Writing the
image erases it.

## What you need

* A CompactFlash card (512 MB or bigger; a bigger card works too, only the
  first 500 MB is used) and a CF-IDE adapter for the target machine.
* A USB card reader on the Windows PC.
* A DOS: your MS-DOS 6.x Setup disk images, or nothing at all, because
  FreeDOS can be downloaded by the program.

## 1. Make the card

1. Start the wizard (*File > Start wizard*) and choose **Make a starter card
   that boots on almost any PC**.
2. Choose the DOS and a boot menu. *Plain DOS* is enough for a start.
3. Choose where DOS comes from: *MS-DOS Setup disks* or *FreeDOS,
   downloaded by the program*.
4. The main window opens with the **Universal starter card** disk profile
   (1000 / 16 / 63) and *Put GEOM.COM onto the disk* ticked on the *Build*
   page. Add drivers or your own files if you like.
5. On the *Build* page: *Create*, try the image in 86Box if you like (hard
   disk with 1000 cylinders, 16 heads, 63 sectors, as the image), then *Write to disk...*
   and choose the card.

## 2. Set up the BIOS of the target machine

Put the card into the CF-IDE adapter as the **master** (first) disk on the
first IDE cable. Enter BIOS setup (usually Del, F1, F2 or Ctrl+Alt+Esc at
start-up, on some machines a setup program on a floppy) and set the hard disk
by hand:

| Field | Value |
|---|---|
| Type | **User** (on many BIOSes type 47) |
| Cylinders | **1024** (or the most it accepts, at least 1002) |
| Heads | **16** |
| Sectors | **63** |
| Precomp, Landing zone | anything; IDE disks do not use them (0 or 1024 is fine) |
| Mode (if there is one) | **NORMAL** or **CHS**, *not* LBA or LARGE |

**Do not use Auto.** Auto takes the geometry the card reports about itself,
which can have other heads and sectors (a small card often says 32 sectors);
then the card does not boot.

Why 1024 cylinders in the BIOS and 1000 in the image? Some BIOSes give DOS
one or two cylinders fewer than they are set to; the image stays inside
what every BIOS gives.

## 3. Boot it and run GEOM

Save the BIOS settings and restart. The boot menu appears; choose an item.
At the DOS prompt type:

```
GEOM
```

It shows the geometry the BIOS gives to DOS, for example 1022 / 16 / 63. With
the starter card, heads and sectors are 16 and 63, as set.

## 4. Then

* **Use it as it is:** it is a complete DOS system of about 500 MB. Add
  games and tools with *Your files* and write the card again.
* **Use the whole card:** if the BIOS has Auto or LBA, set the card back to
  Auto (with LBA on), note what BIOS setup shows and what `GEOM` shows then,
  and build a new card for this machine with *From BIOS setup...* or those
  values (see the [CF card guide](CF-GUIDE.md)).
* **Bring an old IDE disk to life:** the starter card is the helper card of
  [Old IDE disks](OLD-IDE-DISKS.md), Way 1.

## If it does not boot

* **Check the BIOS values** again: User type, 16 heads, 63 sectors, LBA off.
  The heads and sectors must match exactly; the cylinders need only be at
  least 1002.
* **"No ROM BASIC", "Missing operating system", or it hangs:** some CF cards
  do not accept a geometry set by the BIOS. Try another card (older,
  "industrial" or simple cards of known brands are usually fine).
* **The BIOS has no User type, or does not accept 63 sectors:** use a
  classic drive type instead, see below.

## A BIOS without a User type: classic drive types

Very early BIOSes (such as the original IBM AT and many 286 clones) only have
a fixed list of drive types. Nearly every clone kept the first 14 types of
the IBM AT BIOS unchanged, so two of them are built-in disk profiles too:

| Profile | BIOS type | BIOS shows | Size |
|---|---|---|---|
| Classic drive type 2 | **2** (the Seagate ST-225) | 615 / 4 / 17 | 20 MB |
| Classic drive type 9 | **9** | 900 / 15 / 17 | about 112 MB |

Type 2 is the safest: it is in practically every AT BIOS. Type 9 gives more
room, enough for a helper card. CF cards accept these geometries like any
other one set by the BIOS.

1. Make the card as above, but on the *Machine and disk* page choose the
   **Classic drive type** profile instead of the universal one (and keep
   *Put GEOM.COM onto the disk* ticked on the *Build* page).
2. In BIOS setup choose that **type number** for the hard disk, and **check
   that the BIOS shows the values in the table.** If it shows other values,
   that BIOS has its own table: use the type with those values that is
   closest, and build the image with exactly what it shows (two cylinders
   fewer, as the profiles do).
3. Boot and run `GEOM`: it shows 4 heads and 17 sectors (type 2) or 15 heads
   and 17 sectors (type 9).

The images use two cylinders fewer than the table (613 and 898), because
some BIOSes keep the last cylinder for themselves.

If nothing else works, an XT-IDE BIOS (XTIDE Universal BIOS) on an IDE
controller or a network card's boot ROM gives even the oldest machines
auto-detection.
