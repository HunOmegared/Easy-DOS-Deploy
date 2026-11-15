import sys, os, subprocess, tempfile
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__))); from asm import golit
# The FreeDOS boot sector (asm/freedos/boot.asm, from the FreeDOS kernel 2043,
# GNU GPL 2 or later), built with NASM for FAT12 and FAT16. It loads KERNEL.SYS
# from the root folder. The OEM name is set as FreeDOS SYS does.
here = os.path.dirname(os.path.abspath(__file__))
out = []
for fat in (12, 16):
    with tempfile.TemporaryDirectory() as d:
        b = os.path.join(d, 'boot.bin')
        subprocess.run(['nasm', '-dISFAT%d' % fat, os.path.join(here, 'freedos', 'boot.asm'), '-o', b], check=True)
        code = bytearray(open(b, 'rb').read())
    assert len(code) == 512 and code[510:] == b'\x55\xaa'
    code[3:11] = b'FRDOS4.1'
    out.append(golit('fdBoot%d' % fat, bytes(code), 'fdBoot%d is the FreeDOS boot sector for FAT%d (source: asm/freedos/boot.asm, built by asm/fdboot.py).' % (fat, fat)))
print('\n\n'.join(out))
