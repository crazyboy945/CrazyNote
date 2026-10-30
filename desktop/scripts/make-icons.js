#!/usr/bin/env node
/**
 * 生成桌面端图标（PNG 与 ICO）。
 * 纯 Node 实现，不依赖图形库。
 */
const fs = require('node:fs');
const path = require('node:path');
const zlib = require('node:zlib');

const OUT_DIR = path.join(__dirname, '..', 'build');

const BLUE = [47, 111, 235];
const DARK = [37, 88, 196];
const WHITE = [255, 255, 255];

/** 生成一个尺寸的 RGBA 像素缓冲 */
function renderIcon(size) {
  const px = Array.from({ length: size }, () => new Array(size * 4).fill(0));
  const radius = size * 0.22;

  const setPx = (x, y, [r, g, b], a = 255) => {
    if (x < 0 || y < 0 || x >= size || y >= size) return;
    const i = x * 4;
    px[y][i] = r; px[y][i + 1] = g; px[y][i + 2] = b; px[y][i + 3] = a;
  };

  // 圆角方块
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      const cx = Math.min(x, size - 1 - x);
      const cy = Math.min(y, size - 1 - y);
      if (cx < radius && cy < radius) {
        const dx = radius - cx;
        const dy = radius - cy;
        if (dx * dx + dy * dy > radius * radius) continue; // 透明
      }
      setPx(x, y, BLUE);
    }
  }

  // 顶部高光条（渐变效果）
  const bandH = Math.max(1, Math.round(size * 0.06));
  for (let y = 0; y < bandH; y++) {
    for (let x = 0; x < size; x++) {
      const t = y / bandH;
      const c = [
        Math.round(BLUE[0] + (DARK[0] - BLUE[0]) * t),
        Math.round(BLUE[1] + (DARK[1] - BLUE[1]) * t),
        Math.round(BLUE[2] + (DARK[2] - BLUE[2]) * t),
      ];
      setPx(x, y, c);
    }
  }

  // 笔记横线
  const lineH = Math.max(1, Math.round(size / 12));
  const gap = Math.max(2, Math.round(size / 7));
  const top = Math.round(size * 0.3);
  const left = Math.round(size * 0.2);
  const right = size - left;
  for (let i = 0; i < 3; i++) {
    const y0 = top + i * (lineH + gap);
    for (let y = y0; y < Math.min(y0 + lineH, size - 1); y++) {
      for (let x = left; x < right; x++) setPx(x, y, WHITE);
    }
  }

  // 装订线
  const bindX = Math.round(size * 0.2) - Math.max(1, Math.round(size / 16));
  const bindW = Math.max(1, Math.round(size / 22));
  for (let y = Math.round(size * 0.24); y < size - Math.round(size * 0.18); y++) {
    for (let x = bindX; x < bindX + bindW; x++) setPx(x, y, DARK);
  }

  return px;
}

/** 组装 PNG */
function encodePNG(px, size) {
  const raw = Buffer.alloc((size * 4 + 1) * size);
  let p = 0;
  for (let y = 0; y < size; y++) {
    raw[p++] = 0; // filter: none
    for (let x = 0; x < size * 4; x++) raw[p++] = px[y][x];
  }

  const chunk = (type, data) => {
    const len = Buffer.alloc(4);
    len.writeUInt32BE(data.length);
    const body = Buffer.concat([Buffer.from(type, 'ascii'), data]);
    const crc = Buffer.alloc(4);
    crc.writeUInt32BE(crc32(body) >>> 0);
    return Buffer.concat([len, body, crc]);
  };

  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(size, 0);
  ihdr.writeUInt32BE(size, 4);
  ihdr[8] = 8;   // bit depth
  ihdr[9] = 6;   // RGBA
  ihdr[10] = 0; ihdr[11] = 0; ihdr[12] = 0;

  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', ihdr),
    chunk('IDAT', zlib.deflateSync(raw, { level: 9 })),
    chunk('IEND', Buffer.alloc(0)),
  ]);
}

let crcTable = null;
function crc32(buf) {
  if (!crcTable) {
    crcTable = new Int32Array(256);
    for (let n = 0; n < 256; n++) {
      let c = n;
      for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1;
      crcTable[n] = c;
    }
  }
  let c = -1;
  for (let i = 0; i < buf.length; i++) c = crcTable[(c ^ buf[i]) & 0xff] ^ (c >>> 8);
  return c ^ -1;
}

/** 组装 ICO（内嵌 PNG） */
function encodeICO(images) {
  const count = images.length;
  const header = Buffer.alloc(6);
  header.writeUInt16LE(0, 0);
  header.writeUInt16LE(1, 2);
  header.writeUInt16LE(count, 4);

  const entries = [];
  const datas = [];
  let offset = 6 + count * 16;

  for (const { size, data } of images) {
    const e = Buffer.alloc(16);
    e[0] = size >= 256 ? 0 : size;
    e[1] = size >= 256 ? 0 : size;
    e[2] = 0; e[3] = 0;
    e.writeUInt16LE(1, 4);
    e.writeUInt16LE(32, 6);
    e.writeUInt32LE(data.length, 8);
    e.writeUInt32LE(offset, 12);
    offset += data.length;
    entries.push(e);
    datas.push(data);
  }

  return Buffer.concat([header, ...entries, ...datas]);
}

function main() {
  fs.mkdirSync(OUT_DIR, { recursive: true });

  for (const size of [16, 32, 48, 64, 128, 256]) {
    const px = renderIcon(size);
    const png = encodePNG(px, size);
    const name = size === 256 ? 'icon.png' : `icon${size}.png`;
    fs.writeFileSync(path.join(OUT_DIR, name), png);
    console.log(`${name}: ${png.length} bytes (${size}x${size})`);
  }

  // 托盘图标（16x16，半透明深色以便在浅色托盘上也可见）
  const trayPx = renderIcon(16);
  const trayPng = encodePNG(trayPx, 16);
  fs.writeFileSync(path.join(OUT_DIR, 'tray.png'), trayPng);
  console.log(`tray.png: ${trayPng.length} bytes (16x16)`);

  // ICO：包含 16/32/48/64/128/256
  const icoSizes = [16, 32, 48, 64, 128, 256];
  const icoImages = icoSizes.map((size) => ({
    size,
    data: encodePNG(renderIcon(size), size),
  }));
  const ico = encodeICO(icoImages);
  fs.writeFileSync(path.join(OUT_DIR, 'icon.ico'), ico);
  console.log(`icon.ico: ${ico.length} bytes (${icoSizes.join('/')})`);
}

main();
