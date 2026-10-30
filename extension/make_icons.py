#!/usr/bin/env python3
"""生成扩展图标（纯标准库写出 PNG，无第三方依赖）。"""
import struct
import zlib
import os

BLUE = (47, 111, 235)
WHITE = (255, 255, 255)
DARK = (37, 88, 196)


def make_icon(size):
    """画一个圆角蓝色方块 + 白色笔记横线。"""
    px = [[BLUE for _ in range(size)] for _ in range(size)]
    radius = size * 0.22
    inset = size * 0.16

    # 圆角：四角外切
    for y in range(size):
        for x in range(size):
            cx = min(x, size - 1 - x)
            cy = min(y, size - 1 - y)
            if cx < radius and cy < radius:
                dx = radius - cx
                dy = radius - cy
                if dx * dx + dy * dy > radius * radius:
                    px[y][x] = (0, 0, 0)  # 透明占位，后面处理 alpha

    # 笔记横线
    line_h = max(1, size // 12)
    gap = max(2, size // 7)
    top = size // 4
    for i in range(3):
        y0 = top + i * (line_h + gap)
        x0 = inset
        x1 = size - inset
        for y in range(y0, min(y0 + line_h, size)):
            for x in range(int(x0), int(x1)):
                px[y][x] = WHITE

    # 竖线（笔记本装订线）
    for y in range(int(top - gap // 2), size - int(top - gap // 2)):
        for x in range(int(inset - gap // 2), int(inset + gap // 4)):
            if 0 <= x < size and 0 <= y < size:
                px[y][x] = DARK

    # 转成 RGBA：黑色占位改为透明
    raw = bytearray()
    for y in range(size):
        raw.append(0)  # filter type 0
        for x in range(size):
            r, g, b = px[y][x]
            if (r, g, b) == (0, 0, 0):
                raw += bytes((0, 0, 0, 0))
            else:
                raw += bytes((r, g, b, 255))
    return bytes(raw)


def write_png(path, size):
    raw = make_icon(size)
    # PNG signature
    sig = b'\x89PNG\r\n\x1a\n'

    def chunk(tag, data):
        c = struct.pack('>I', len(data)) + tag + data
        c += struct.pack('>I', zlib.crc32(tag + data) & 0xffffffff)
        return c

    ihdr = struct.pack('>IIBBBBB', size, size, 8, 6, 0, 0, 0)  # 8-bit RGBA
    idat = zlib.compress(raw, 9)
    png = sig + chunk(b'IHDR', ihdr) + chunk(b'IDAT', idat) + chunk(b'IEND', b'')
    with open(path, 'wb') as f:
        f.write(png)
    print(f'{path}: {len(png)} bytes ({size}x{size})')


if __name__ == '__main__':
    here = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'icons')
    os.makedirs(here, exist_ok=True)
    for s in (16, 48, 64, 128, 256):
        write_png(os.path.join(here, f'icon{s}.png'), s)
    # 应用图标
    write_png(os.path.join(os.path.dirname(here), 'icon.png'), 256)
