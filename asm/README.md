# 16-bit code

`mbr.py` and `geom.py` are the sources of the two small pieces of x86 real-mode
code the program writes: the MBR boot code of new hard disk / CF images and
`GEOM.COM`. `asm.py` is a tiny two-pass assembler for the handful of 8086
instructions they use; each instruction is written as its bytes with the
mnemonic in a comment.

    python3 asm/mbr.py  > mbrcode.go.txt
    python3 asm/geom.py > geomcom.go.txt

The output is the Go byte slice in `mbrcode.go` / `geomcom.go`. The tests run
both through a small 8086 emulator (`x86emu_test.go`).
