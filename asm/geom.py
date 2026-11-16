import sys; import os; sys.path.insert(0, os.path.dirname(os.path.abspath(__file__))); from asm import *
# GEOM.COM: prints the geometry the BIOS gives DOS for the first hard disk
# (INT 13h, AH=08h), in the form Easy DOS Deploy wants.
def prt(lab): return [('imm16',[0xBA],lab), ('b',[0xB4,0x09]), ('b',[0xCD,0x21])]   # mov dx,lab; mov ah,9; int 21h
g = [
  ('imm16',[0xBA],'m_head'), ('b',[0xB4,0x09]), ('b',[0xCD,0x21]),
  ('b',[0xB4,0x08]),            # mov ah,8
  ('b',[0xB2,0x80]),            # mov dl,80h
  ('b',[0x31,0xFF]),            # xor di,di (some BIOSes want ES:DI = 0)
  ('b',[0x8E,0xC7]),            # mov es,di
  ('b',[0xCD,0x13]),            # int 13h
  ('jcc8',0x72,'nodisk'),       # jc nodisk
  ('b',[0x52]),                 # push dx (DH = last head)
  ('b',[0x51]),                 # push cx
  *prt('m_cyl'),
  ('b',[0x59]),                 # pop cx
  ('b',[0x51]),                 # push cx
  ('b',[0x88,0xE8]),            # mov al,ch
  ('b',[0x88,0xCC]),            # mov ah,cl
  ('b',[0xB1,0x06]),            # mov cl,6
  ('b',[0xD2,0xEC]),            # shr ah,cl     AX = last cylinder number
  ('call16','num'),
  *prt('m_hd'),
  ('b',[0x59]),                 # pop cx
  ('b',[0x5A]),                 # pop dx
  ('b',[0x51]),                 # push cx
  ('b',[0x88,0xF0]),            # mov al,dh
  ('b',[0x30,0xE4]),            # xor ah,ah
  ('b',[0x40]),                 # inc ax       heads = last head + 1
  ('b',[0x89,0xC6]),            # mov si,ax    (kept for the 256-head check)
  ('call16','num'),
  *prt('m_sec'),
  ('b',[0x59]),                 # pop cx
  ('b',[0x88,0xC8]),            # mov al,cl
  ('b',[0x25,0x3F,0x00]),       # and ax,3Fh   sectors per track
  ('call16','num'),
  ('b',[0x81,0xFE,0x00,0x01]),  # cmp si,256
  ('jcc8',0x75,'ok'),           # jne ok
  *prt('m_256'),                # 256 heads: MS-DOS cannot use it
  ('b',[0xB8,0x02,0x4C]),       # mov ax,4C02h
  ('b',[0xCD,0x21]),            # int 21h
'ok:',
  *prt('m_tail'),
  ('b',[0xB8,0x00,0x4C]),       # mov ax,4C00h
  ('b',[0xCD,0x21]),            # int 21h
'nodisk:',
  *prt('m_none'),
  ('b',[0xB8,0x01,0x4C]),       # mov ax,4C01h
  ('b',[0xCD,0x21]),            # int 21h
'num:',                         # print AX in decimal
  ('b',[0xBB,0x0A,0x00]),       # mov bx,10
  ('b',[0x31,0xC9]),            # xor cx,cx
'n1:',
  ('b',[0x31,0xD2]),            # xor dx,dx
  ('b',[0xF7,0xF3]),            # div bx
  ('b',[0x52]),                 # push dx
  ('b',[0x41]),                 # inc cx
  ('b',[0x09,0xC0]),            # or ax,ax
  ('jcc8',0x75,'n1'),           # jnz n1
'n2:',
  ('b',[0x5A]),                 # pop dx
  ('b',[0x80,0xC2,0x30]),       # add dl,'0'
  ('b',[0xB4,0x02]),            # mov ah,2
  ('b',[0xCD,0x21]),            # int 21h
  ('jcc8',0xE2,'n2'),           # loop n2
  ('b',[0xC3]),                 # ret
'm_head:', ('s','GEOM - hard disk geometry for Easy DOS Deploy\r\n\r\n$'),
'm_cyl:',  ('s','Cylinders: $'),
'm_hd:',   ('s','   Heads: $'),
'm_sec:',  ('s','   Sectors/track: $'),
'm_tail:', ('s','\r\n\r\nEnter these three numbers in the program.\r\n$'),
'm_256:',  ('s','\r\n\r\nWARNING: 256 heads. MS-DOS 6.x and Windows 9x cannot use a disk\r\n'
                'that the BIOS gives like this. In BIOS setup, set the drive to\r\n'
                'Type User with fewer cylinders (Easy DOS Deploy, "From BIOS setup...",\r\n'
                'tells how many), then run GEOM again.\r\n$'),
'm_none:', ('s','No hard disk found (INT 13h, drive 80h).\r\n$'),
]
code, labels = assemble(g, 0x100)
print(len(code), file=sys.stderr)
print(golit('geomCOM', code, 'geomCOM is GEOM.COM, a DOS program that prints the INT 13h geometry of the first hard disk (%d bytes, source: asm/geom.py).' % len(code)))
