"""用真实浏览器验证「上传图片 → 插入笔记 → 保存 → 重开 → 图片可见」的完整链路。

覆盖用户反馈的「图片上传似乎有些问题」。
"""
import asyncio
import json
import os
import re
import struct
import sys
import time
import urllib.request
import zlib

BASE = os.environ.get('CRAZYNOTE_BASE', 'http://127.0.0.1:18892')
LOG = os.environ.get('CRAZYNOTE_LOG', r'C:\Users\Administrator\AppData\Local\Temp\crazynote-test\server.log')
OUT = os.environ.get('CRAZYNOTE_SHOT_DIR', r'G:\WorkBuddy\CrazyNote\test\theme-shots')
WORK = os.environ.get('CRAZYNOTE_WORK', r'C:\Users\Administrator\AppData\Local\Temp\crazynote-test')


def png_gradient(width=480, height=160):
    """生成一张真实可解码的 PNG（纯标准库）。"""
    rows = []
    for y in range(height):
        row = bytearray([0])
        for x in range(width):
            row += bytes((30 + x * 200 // width, 90 + y * 120 // height, 220 - x * 120 // width))
        rows.append(bytes(row))
    raw = b''.join(rows)

    def chunk(tag, data):
        c = tag + data
        return struct.pack('>I', len(data)) + c + struct.pack('>I', zlib.crc32(c) & 0xffffffff)

    return (b'\x89PNG\r\n\x1a\n'
            + chunk(b'IHDR', struct.pack('>IIBBBBB', width, height, 8, 2, 0, 0, 0))
            + chunk(b'IDAT', zlib.compress(raw, 9))
            + chunk(b'IEND', b''))


def read_password():
    for _ in range(30):
        if os.path.exists(LOG):
            m = re.search(r'INITIAL_PASSWORD=(\S+)', open(LOG, encoding='utf-8', errors='ignore').read())
            if m:
                return m.group(1)
        time.sleep(1)
    return ''


async def main():
    pwd = read_password()
    if not pwd:
        print('无法从日志读取访问口令')
        return 1
    os.makedirs(OUT, exist_ok=True)
    os.makedirs(WORK, exist_ok=True)

    png_path = os.path.join(WORK, '上传测试图片.png')
    with open(png_path, 'wb') as f:
        f.write(png_gradient())

    from playwright.async_api import async_playwright
    import glob
    chrome = None
    for pat in (r'C:\Users\Administrator\AppData\Local\ms-playwright\chromium-*/chrome-win64/chrome.exe',
                r'C:\Users\Administrator\AppData\Local\ms-playwright\chromium-*/chrome-win/chrome.exe'):
        hits = sorted(glob.glob(pat))
        if hits:
            chrome = hits[-1]
            break

    errors = []
    ok = True
    async with async_playwright() as p:
        browser = await p.chromium.launch(executable_path=chrome, args=['--no-sandbox'])
        page = await browser.new_page(viewport={'width': 1360, 'height': 900})
        page.on('console', lambda m: errors.append(f'[console.{m.type}] {m.text}') if m.type == 'error' else None)
        page.on('pageerror', lambda e: errors.append(f'[pageerror] {e}'))

        await page.goto(BASE + '/', wait_until='networkidle')
        if await page.locator('#login-overlay').is_visible():
            await page.fill('#login-password', pwd)
            await page.click('#btn-login')
            await page.wait_for_timeout(1200)

        # 新建笔记
        await page.click('#btn-new-note')
        await page.wait_for_timeout(1200)

        # 切到编辑态并清空
        await page.evaluate("""() => {
            const t = document.getElementById('note-body');
            t.value = '图片上传测试\\n\\n';
        }""")

        # 触发上传：直接给隐藏的 file input 塞文件（等价于用户在文件选择框里选图）
        await page.set_input_files('#file-image', png_path)
        await page.wait_for_timeout(2500)

        body = await page.input_value('#note-body')
        print('--- 正文内容 ---')
        print(body)
        m = re.search(r'!\[([^\]]*)\]\(([^)]+)\)', body)
        if not m:
            print('✗ 正文里没有插入图片 Markdown')
            ok = False
        else:
            alt, url = m.group(1), m.group(2)
            print(f'✓ 已插入: alt={alt!r} url={url!r}')
            if not url.startswith('/assets/images/'):
                print(f'✗ 图片地址不是稳定的应用内路径: {url}')
                ok = False
            if '%' in url:
                print('✗ 图片地址被百分号编码后存进笔记了（应存原始路径）')
                ok = False

        # 保存
        await page.click('#btn-save')
        await page.wait_for_timeout(1500)

        # 重新打开笔记，验证阅读态下图片能真正加载出来
        await page.evaluate("""() => {
            const t = document.getElementById('note-body');
            t.dispatchEvent(new Event('input', {bubbles:true}));
        }""")
        await page.wait_for_timeout(1500)

        img_info = await page.evaluate("""async () => {
            const img = document.querySelector('#preview-inner img');
            if (!img) return {found: false};
            if (!img.complete) await new Promise(r => { img.onload = r; img.onerror = r; setTimeout(r, 4000); });
            return {found: true, src: img.getAttribute('src'), w: img.naturalWidth, h: img.naturalHeight};
        }""")
        print('--- 预览中的图片 ---')
        print(img_info)
        if not img_info.get('found'):
            print('✗ 预览里没有 img 元素')
            ok = False
        elif not img_info.get('w'):
            print(f'✗ 图片加载失败（naturalWidth=0），src={img_info.get("src")}')
            ok = False
        else:
            print(f'✓ 图片已正常加载 {img_info["w"]}x{img_info["h"]}')

        await page.locator('#preview').screenshot(path=os.path.join(OUT, 'upload-e2e.png'))

        await browser.close()

    if errors:
        print('\n前端错误:')
        for e in errors:
            print('  ', e)
    print('\n结果:', '全部通过 ✓' if ok and not errors else '存在问题 ✗')
    return 0 if ok else 1


if __name__ == '__main__':
    sys.exit(asyncio.run(main()))
