# 16-bit code

`mbr.py` and `geom.py` are the sources of the two small pieces of x86 real-mode
code the program writes: the MBR boot code of new hard disk / CF images and
`GEOM.COM`. `asm.py` is a tiny two-pass assembler for the handful of 8086
instructions they use; each instruction is written as its bytes with the
mnemonic in a comment.

Run them from the repository root:

    python3 asm/mbr.py
    python3 asm/geom.py

The output is the Go byte slice in `src/mbrcode.go` / `src/geomcom.go` (below
their four header lines). The
tests run both through a small 8086 emulator (`src/x86emu_test.go`).

## FreeDOS boot sector

`freedos/boot.asm` is the boot sector of the FreeDOS kernel 2043
(https://github.com/FDOS/kernel, GNU GPL 2 or later). It loads `KERNEL.SYS`
from the root folder. `fdboot.py` builds it with NASM for FAT12 and FAT16:

    python3 asm/fdboot.py

Its output is `src/fdboot.go` (below its four header lines). The program uses
it for FreeDOS disks it makes without a system source, with the FreeDOS kernel
and FreeCOM downloaded from their GitHub releases.
