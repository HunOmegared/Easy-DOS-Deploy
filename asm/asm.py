# Tiny two-pass assembler for the few 8086 instructions we use.
import struct, sys
def assemble(prog, org):
    labels = {}
    for pass_ in (1, 2):
        out = bytearray(); pc = org
        for item in prog:
            if isinstance(item, str) and item.endswith(':'):
                labels[item[:-1]] = pc; continue
            kind = item[0]
            if kind == 'b':      # raw bytes
                b = bytes(item[1])
            elif kind == 's':    # string
                b = item[1].encode('latin-1')
            elif kind in ('jcc8',):   # opcode, label
                op, lab = item[1], item[2]
                t = labels.get(lab, pc + 2)
                rel = t - (pc + 2)
                if pass_ == 2 and not -128 <= rel <= 127: raise SystemExit('jump too far: ' + lab)
                b = bytes([op, rel & 0xFF])
            elif kind == 'call16':
                t = labels.get(item[1], pc + 3)
                b = bytes([0xE8]) + struct.pack('<h', t - (pc + 3))
            elif kind == 'imm16':  # opcode bytes + 16-bit label address
                t = labels.get(item[2], 0)
                b = bytes(item[1]) + struct.pack('<H', t & 0xFFFF)
            elif kind == 'imm16b':  # opcode bytes + 16-bit label address + trailing bytes
                t = labels.get(item[2], 0)
                b = bytes(item[1]) + struct.pack('<H', t & 0xFFFF) + bytes(item[3])
            else:
                raise SystemExit('bad ' + str(item))
            out += b; pc += len(b)
    return bytes(out), labels

def golit(name, data, comment):
    lines = ['// ' + comment, 'var %s = []byte{' % name]
    for i in range(0, len(data), 16):
        lines.append('\t' + ', '.join('0x%02X' % x for x in data[i:i+16]) + ',')
    lines.append('}')
    return '\n'.join(lines)
