import sys, os, subprocess, tempfile
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__))); from asm import golit
# GEOM.COM (asm/geom.asm, built with NASM): lists the hard disks the BIOS
# gives DOS (INT 13h, AH=08h) and asks the IDE devices for their own
# geometry (ATA IDENTIFY DEVICE), for BIOSes without auto-detection.
here = os.path.dirname(os.path.abspath(__file__))
with tempfile.TemporaryDirectory() as d:
    b = os.path.join(d, 'geom.com')
    subprocess.run(['nasm', '-f', 'bin', os.path.join(here, 'geom.asm'), '-o', b], check=True)
    code = open(b, 'rb').read()
print(golit('geomCOM', code, 'geomCOM is GEOM.COM, a DOS program that lists the BIOS hard disks and the IDE devices with their geometry (%d bytes, source: asm/geom.asm).' % len(code)))
