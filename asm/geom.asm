; GEOM.COM - hard disk geometry for Easy DOS Deploy
; Copyright (C) 2026 Daniel Szentmiklosi, GNU GPL v3
;
; 1. What the BIOS gives DOS for every hard disk it knows (INT 13h, AH=08h):
;    the numbers to type into Easy DOS Deploy. With a starter card as the
;    first disk, the second disk can be measured with its own BIOS settings.
; 2. What the IDE devices say about themselves (ATA IDENTIFY DEVICE on the
;    primary and secondary channel): model, own geometry, size, and the
;    values to enter as type User in a BIOS without auto-detection.
;
; 8086 instructions only, so that it runs on any PC. Build: nasm -f bin.

cpu 8086
org 100h

start:
	mov dx, m_head
	call print

; ---- 1. the BIOS view ----
	mov dx, m_bios
	call print
	xor ax, ax
	mov es, ax
	mov al, [es:0475h]	; number of hard disks the BIOS knows
	cmp al, 0
	jne .have
	mov dx, m_none
	call print
	mov byte [exitcode], 1
	jmp ide
.have:
	cmp al, 4
	jbe .count
	mov al, 4
.count:
	mov [ndisk], al
	mov byte [drive], 80h
.disk:
	mov dx, m_disk
	call print
	mov al, [drive]
	sub al, 7Fh
	xor ah, ah
	call num
	mov dx, m_colon
	call print
	mov ah, 8
	mov dl, [drive]
	xor di, di		; some BIOSes want ES:DI = 0
	mov es, di
	int 13h
	jnc .got
	mov dx, m_noans
	call print
	jmp .next
.got:
	push dx			; DH = last head
	push cx
	mov dx, m_cyl
	call print
	pop cx
	push cx
	mov al, ch
	mov ah, cl
	mov cl, 6
	shr ah, cl		; AX = last cylinder number
	call num
	mov dx, m_hd
	call print
	pop cx
	pop dx
	push cx
	mov al, dh
	xor ah, ah
	inc ax			; heads = last head + 1
	push ax
	call num
	mov dx, m_sec
	call print
	pop si
	pop cx
	mov al, cl
	and ax, 3Fh		; sectors per track
	call num
	mov dx, m_crlf
	call print
	cmp si, 256
	jne .next
	mov byte [heads256], 1
	mov byte [exitcode], 2
.next:
	inc byte [drive]
	dec byte [ndisk]
	jnz .disk
	cmp byte [heads256], 0
	je .enter
	mov dx, m_256
	call print
	jmp ide
.enter:
	mov dx, m_enter
	call print

; ---- 2. the IDE devices ----
ide:
	mov dx, m_ide
	call print
	mov bx, 1F0h
	mov dx, m_pm
	mov cl, 0A0h
	call probe
	mov bx, 1F0h
	mov dx, m_ps
	mov cl, 0B0h
	call probe
	mov bx, 170h
	mov dx, m_sm
	mov cl, 0A0h
	call probe
	mov bx, 170h
	mov dx, m_ss
	mov cl, 0B0h
	call probe
	mov dx, m_tail
	call print
	mov ah, 4Ch
	mov al, [exitcode]
	int 21h

; probe: asks the device CL (A0h master, B0h slave) on the channel at BX for
; IDENTIFY DEVICE and prints what it says. DX = its name.
probe:
	call print
	mov dx, bx
	add dx, 206h		; device control: no interrupts from the drive
	mov al, 2
	out dx, al
	mov dx, bx
	add dx, 6
	mov al, cl
	out dx, al		; select the device
	call settle
	mov dx, bx
	add dx, 7
	in al, dx
	cmp al, 0FFh		; nothing on this channel
	je .none
	; is there a device? its registers keep what is written to them
	mov dx, bx
	add dx, 2
	mov al, 55h
	out dx, al
	inc dx
	mov al, 0AAh
	out dx, al
	dec dx
	in al, dx
	cmp al, 55h
	jne .none
	inc dx
	in al, dx
	cmp al, 0AAh
	jne .none
	mov dx, bx
	add dx, 7
	mov al, 0ECh		; IDENTIFY DEVICE
	out dx, al
	call settle
	in al, dx
	test al, al
	jz .none
	mov ah, 80h		; wait while busy
	mov al, 0
	call waitst
	jc .none
	mov dx, bx		; a CD-ROM (ATAPI) answers with a signature
	add dx, 4
	in al, dx
	mov ah, al
	inc dx
	in al, dx
	cmp ax, 14EBh
	je .atapi
	mov dx, bx
	add dx, 7
	in al, dx
	test al, 1		; error: not an ATA disk
	jnz .none
	mov ah, 08h		; wait for the data
	mov al, 08h
	call waitst
	jc .none
	mov dx, bx
	mov di, buf
	push ds
	pop es
	mov cx, 256
.read:
	in ax, dx
	stosw
	loop .read
	call done
	jmp show
.atapi:
	call done
	mov dx, m_atapi
	jmp print
.none:
	call done
	mov dx, m_nodev
	jmp print

; done: interrupts from the drive on again, the master selected
done:
	mov dx, bx
	add dx, 206h
	mov al, 0
	out dx, al
	mov dx, bx
	add dx, 6
	mov al, 0A0h
	out dx, al
	ret

; settle: about 400 ns, by reading the status a few times
settle:
	push dx
	push cx
	mov dx, bx
	add dx, 206h
	mov cx, 4
.s:
	in al, dx
	loop .s
	pop cx
	pop dx
	ret

; waitst: waits until (status AND AH) = AL, at most about 2 seconds (BIOS
; timer). CF set on timeout.
waitst:
	push es
	push si
	push cx
	mov cx, ax
	xor si, si
	mov es, si
	mov si, [es:046Ch]
	mov dx, bx
	add dx, 7
.w:
	in al, dx
	and al, ch
	cmp al, cl
	je .ok
	mov ax, [es:046Ch]
	sub ax, si
	cmp ax, 36
	jb .w
	stc
	jmp .out
.ok:
	clc
.out:
	pop cx
	pop si
	pop es
	ret

; show: prints the IDENTIFY data in buf
show:
	; the model: 40 characters, two in each word, high byte first
	mov si, buf + 54
	mov di, model
	push ds
	pop es
	mov cx, 20
.m:
	lodsw
	xchg al, ah
	stosw
	loop .m
	mov di, model + 39	; drop the spaces at the end
.trim:
	cmp byte [di], ' '
	jne .end
	cmp di, model
	je .end
	dec di
	jmp .trim
.end:
	mov byte [di+1], '$'
	mov dx, model
	call print
	mov dx, m_own
	call print
	mov ax, [buf+2]		; word 1: cylinders
	mov [c], ax
	call num
	mov dx, m_slash
	call print
	mov ax, [buf+6]		; word 3: heads
	mov [h], ax
	call num
	mov dx, m_slash
	call print
	mov ax, [buf+12]	; word 6: sectors per track
	mov [s], ax
	call num
	mov dx, m_comma
	call print
	; the size: LBA sectors (words 60-61) if it has LBA, else C*H*S
	test byte [buf+99], 2	; word 49 bit 9
	jz .chs
	mov ax, [buf+120]
	mov dx, [buf+122]
	jmp .size
.chs:
	mov ax, [c]
	mul word [h]		; DX:AX = C*H (fits in 32 bits)
	mov cx, dx
	mul word [s]		; low part * S
	push ax
	push dx
	mov ax, cx
	mul word [s]		; high part * S
	pop dx
	add dx, ax
	pop ax
.size:
	mov cx, 11		; sectors -> MB
.mb:
	shr dx, 1
	rcr ax, 1
	loop .mb
	test dx, dx
	jz .inmb
	mov cx, 10		; MB -> GB
.gb:
	shr dx, 1
	rcr ax, 1
	loop .gb
	call num
	mov dx, m_gb
	call print
	jmp .user
.inmb:
	call num
	mov dx, m_mb
	call print
.user:
	; type User in a BIOS without auto-detection: at most 1024 cylinders,
	; 16 heads and 63 sectors (the CHS limit, about 504 MB)
	mov ax, [h]
	cmp ax, 16
	ja .nouser
	test ax, ax
	jz .nouser
	mov ax, [s]
	cmp ax, 63
	ja .nouser
	test ax, ax
	jz .nouser
	mov dx, m_user
	call print
	mov ax, [c]
	cmp ax, 1024
	jbe .cok
	mov ax, 1024
.cok:
	call num
	mov dx, m_slash
	call print
	mov ax, [h]
	call num
	mov dx, m_slash
	call print
	mov ax, [s]
	call num
	cmp word [c], 1024
	jbe .crlf
	mov dx, m_cut
	call print
.crlf:
	mov dx, m_crlf
	jmp print
.nouser:
	mov dx, m_crlf
	jmp print

; print: the $-terminated string at DX
print:
	mov ah, 9
	int 21h
	ret

; num: prints AX in decimal
num:
	push bx
	mov bx, 10
	xor cx, cx
.n1:
	xor dx, dx
	div bx
	push dx
	inc cx
	or ax, ax
	jnz .n1
.n2:
	pop dx
	add dl, '0'
	mov ah, 2
	int 21h
	loop .n2
	pop bx
	ret

m_head	db 'GEOM - hard disk geometry for Easy DOS Deploy', 13, 10, 13, 10, '$'
m_bios	db 'What the BIOS gives DOS:', 13, 10, '$'
m_disk	db '  Disk $'
m_colon	db ': $'
m_cyl	db 'Cylinders: $'
m_hd	db '   Heads: $'
m_sec	db '   Sectors/track: $'
m_noans	db 'no answer', 13, 10, '$'
m_none	db '  No hard disk found by the BIOS.', 13, 10, '$'
m_enter	db 13, 10, 'Enter these three numbers in the program (disk 1 is the disk DOS', 13, 10
	db 'started from, disk 2 the next one).', 13, 10, '$'
m_256	db 13, 10, 'WARNING: 256 heads. MS-DOS 6.x and Windows 9x cannot use a disk', 13, 10
	db 'that the BIOS gives like this. In BIOS setup, set the drive to', 13, 10
	db 'Type User with fewer cylinders (Easy DOS Deploy, "From BIOS setup...",', 13, 10
	db 'tells how many), then run GEOM again.', 13, 10, '$'
m_ide	db 13, 10, 'What the IDE devices say about themselves:', 13, 10, '$'
m_pm	db '  Primary master:   $'
m_ps	db '  Primary slave:    $'
m_sm	db '  Secondary master: $'
m_ss	db '  Secondary slave:  $'
m_nodev	db '-', 13, 10, '$'
m_atapi	db 'CD-ROM or other ATAPI device', 13, 10, '$'
m_own	db 13, 10, '    Own geometry: $'
m_slash	db '/$'
m_comma	db ', $'
m_mb	db ' MB$'
m_gb	db ' GB$'
m_user	db 13, 10, '    Without auto-detection, BIOS type User: $'
m_cut	db 13, 10, '    (a BIOS without LBA uses at most 1024 cylinders, about 504 MB)$'
m_crlf	db 13, 10, '$'
m_tail	db 13, 10, 'Devices on other ports (XT-IDE, SCSI) are not shown.', 13, 10, '$'

exitcode db 0
heads256 db 0
ndisk	db 0
drive	db 0
c	dw 0
h	dw 0
s	dw 0

section .bss
model	resb 41
buf	resw 256
