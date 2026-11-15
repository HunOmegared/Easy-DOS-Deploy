import sys; import os; sys.path.insert(0, os.path.dirname(os.path.abspath(__file__))); from asm import *
# MBR: loaded by the BIOS at 0000:7C00, copies itself to 0000:0600, finds the
# active partition, loads its first sector with INT 13h (CHS from the table)
# to 0000:7C00 and jumps there with DL = boot drive and DS:SI = table entry.
# The drive number comes from the table entry (80h), as in the MS-DOS MBR:
# some old BIOSes do not pass it in DL (an Amstrad Integra 486 with a Phoenix
# BIOS from 1995 said "Disk read error" when DL was used).
mbr = [
  ('b',[0xFA]),                 # cli
  ('b',[0x31,0xC0]),            # xor ax,ax
  ('b',[0x8E,0xD0]),            # mov ss,ax
  ('b',[0xBC,0x00,0x7C]),       # mov sp,7C00h
  ('b',[0x8E,0xD8]),            # mov ds,ax
  ('b',[0x8E,0xC0]),            # mov es,ax
  ('b',[0xFB]),                 # sti
  ('b',[0xFC]),                 # cld
  ('b',[0xBE,0x00,0x7C]),       # mov si,7C00h
  ('b',[0xBF,0x00,0x06]),       # mov di,0600h
  ('b',[0xB9,0x00,0x01]),       # mov cx,256
  ('b',[0xF3,0xA5]),            # rep movsw
  ('imm16b',[0xEA],'go',[0x00,0x00]),   # jmp 0000:go
'go:',
  ('b',[0xBE,0xBE,0x07]),       # mov si,07BEh (table at 0600h+1BEh)
  ('b',[0xB9,0x04,0x00]),       # mov cx,4
'find:',
  ('b',[0x80,0x3C,0x80]),       # cmp byte [si],80h
  ('jcc8',0x74,'found'),        # je found
  ('b',[0x83,0xC6,0x10]),       # add si,16
  ('jcc8',0xE2,'find'),         # loop find
  ('imm16',[0xBE],'m_noact'),   # mov si,m_noact
  ('jcc8',0xEB,'error'),        # jmp error
'found:',
  ('b',[0xBF,0x05,0x00]),       # mov di,5 (tries)
'read:',
  ('b',[0x8B,0x14]),            # mov dx,[si]    DL = 80h (boot flag), DH = head
  ('b',[0x8B,0x4C,0x02]),       # mov cx,[si+2]  sector, cylinder
  ('b',[0xBB,0x00,0x7C]),       # mov bx,7C00h
  ('b',[0xB8,0x01,0x02]),       # mov ax,0201h   read 1 sector
  ('b',[0x57]),                 # push di
  ('b',[0x56]),                 # push si
  ('b',[0xCD,0x13]),            # int 13h
  ('b',[0x5E]),                 # pop si
  ('b',[0x5F]),                 # pop di
  ('jcc8',0x73,'loaded'),       # jnc loaded
  ('b',[0x31,0xC0]),            # xor ax,ax
  ('b',[0x56]),                 # push si
  ('b',[0xCD,0x13]),            # int 13h (reset)
  ('b',[0x5E]),                 # pop si
  ('b',[0x4F]),                 # dec di
  ('jcc8',0x75,'read'),         # jnz read
  ('imm16',[0xBE],'m_read'),    # mov si,m_read
  ('jcc8',0xEB,'error'),        # jmp error
'loaded:',
  ('b',[0x81,0x3E,0xFE,0x7D,0x55,0xAA]),  # cmp word [7DFEh],0AA55h
  ('jcc8',0x75,'nosys'),        # jne nosys
  ('b',[0xEA,0x00,0x7C,0x00,0x00]),       # jmp 0000:7C00h
'nosys:',
  ('imm16',[0xBE],'m_nosys'),   # mov si,m_nosys
'error:',
  ('b',[0xAC]),                 # lodsb
  ('b',[0x08,0xC0]),            # or al,al
  ('jcc8',0x74,'hang'),         # jz hang
  ('b',[0xB4,0x0E]),            # mov ah,0Eh
  ('b',[0xBB,0x07,0x00]),       # mov bx,7
  ('b',[0xCD,0x10]),            # int 10h
  ('jcc8',0xEB,'error'),        # jmp error
'hang:',
  ('b',[0xF4]),                 # hlt
  ('jcc8',0xEB,'hang'),         # jmp hang
'm_noact:', ('s','No active partition\r\n\0'),
'm_read:',  ('s','Disk read error\r\n\0'),
'm_nosys:', ('s','Missing operating system\r\n\0'),
]
code, labels = assemble(mbr, 0x600)
assert len(code) <= 440, len(code)
print(len(code), file=sys.stderr)
print(golit('mbrCode', code, 'mbrCode is the program\'s own MBR boot code (%d bytes, source: asm/mbr.py).' % len(code)))
