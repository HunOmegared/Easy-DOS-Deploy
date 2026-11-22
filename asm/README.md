# 16-bit code

`mbr.py` is the source of the MBR boot code of new hard disk / CF images.
`asm.py` is a tiny two-pass assembler for the handful of 8086 instructions it
uses; each instruction is written as its bytes with the mnemonic in a comment.

`geom.asm` is the source of `GEOM.COM` (NASM, `cpu 8086`, so it runs on an XT
too). It lists the hard disks the BIOS gives DOS (INT 13h, AH=08h) and asks
the devices on the two standard IDE channels (1F0h and 170h, master and
slave) for their own geometry with ATA IDENTIFY DEVICE. `geom.py` builds it
with NASM.

Run them from the repository root:

    python3 asm/mbr.py
    python3 asm/geom.py

The output is the Go byte slice in `src/mbrcode.go` / `src/geomcom.go` (below
their four header lines). The tests run the MBR code through a small 8086
emulator (`src/x86emu_test.go`), and GEOM.COM through one with a BIOS and an
IDE controller with simulated devices (`src/geomemu_test.go`).

## FreeDOS boot sector

`freedos/boot.asm` is the boot sector of the FreeDOS kernel 2043
(https://github.com/FDOS/kernel, GNU GPL 2 or later). It loads `KERNEL.SYS`
from the root folder. `fdboot.py` builds it with NASM for FAT12 and FAT16:

    python3 asm/fdboot.py

Its output is `src/fdboot.go` (below its four header lines). The program uses
it for FreeDOS disks it makes without a system source, with the FreeDOS kernel
and FreeCOM downloaded from their GitHub releases.
