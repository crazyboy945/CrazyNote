"""用 Playwright 打开真实界面，逐套切换 Markdown 样式并截图。

用法：先启动服务，再运行本脚本（口令从日志读取）。
"""
import asyncio
import glob
import json
import os
import re
import struct
import sys
import time
import zlib
import urllib.request

BASE = os.environ.get('CRAZYNOTE_BASE', 'http://127.0.0.1:18899')
LOG = os.environ.get('CRAZYNOTE_LOG', r'C:\Users\Administrator\AppData\Local\Temp\crazynote-test\server.log')
OUT = os.environ.get('CRAZYNOTE_SHOT_DIR', r'G:\WorkBuddy\CrazyNote\test\theme-shots')
THEMES = ['default', 'reading', 'paper', 'card', 'compact']

NOTE_MD = """# Markdown 样式预览

这是一段 **加粗**、*斜体*、~~删除线~~ 与 `行内代码` 的混排文字，用来检验正文的字重、字号与行距。

## 二级标题

普通段落文字，用于观察默认行距与段间距。中文与 English mixed content 混排测试，英文字体回退是否正常。

> 引用块：用于观察左侧竖线、灰化文字与浅色背景的搭配效果。

### 代码块（语法高亮）

```go
package main

import "fmt"

func main() {
	for i := 0; i < 3; i++ {
		fmt.Printf("hello %d\\n", i)
	}
}
```

### 列表与表格

- 第一项：观察列表缩进与项目符号
- 第二项：包含 `inline code` 的列表项
- 第三项：普通文本

| 样式 | 字体 | 行距 | 适用场景 |
|------|------|------|----------|
| 简约 | 无衬线 | 适中 | 通用 |
| 悦读 | 衬线 | 宽松 | 长文 |
| 学术 | 衬线 | 宽松 | 论文 |
| 卡片 | 无衬线 | 适中 | 长文卡片 |
| 紧凑 | 无衬线 | 紧密 | 技术笔记 |

---

最后一段用于检查分割线与文末间距。
"""


def png_gradient(width=520, height=180):
    """生成一张简单的渐变 PNG（纯标准库实现）。"""
    rows = []
    for y in range(height):
        row = bytearray([0])
        for x in range(width):
            r = 40 + int(x * 180 / width)
            g = 90 + int(y * 120 / height)
            b = 200 - int(x * 120 / width)
            row += bytes((r, g, b))
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
            content = open(LOG, encoding='utf-8', errors='ignore').read()
            m = re.search(r'访问口令: (\S+)', content)
            if m:
                return m.group(1)
        time.sleep(1)
    return ''


def api(path, method='GET', body=None, pwd=''):
    req = urllib.request.Request(BASE + path, method=method)
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        req.add_header('Content-Type', 'application/json')
    if pwd:
        req.add_header('Cookie', 'crazynote_session=' + pwd)
    with urllib.request.urlopen(req, data) as r:
        raw = r.read().decode()
    return json.loads(raw) if raw else {}


async def main():
    pwd = read_password()
    if not pwd:
        print('无法从日志读取访问口令')
        return 1
    os.makedirs(OUT, exist_ok=True)

    # 登录拿会话 Cookie
    import http.cookiejar
    cj = http.cookiejar.CookieJar()
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cj))
    req = urllib.request.Request(BASE + '/api/login', method='POST',
                                 data=json.dumps({'password': pwd}).encode(),
                                 headers={'Content-Type': 'application/json'})
    opener.open(req).read()
    cookie = None
    for c in cj:
        if c.name == 'crazynote_session':
            cookie = c.value
    if not cookie:
        print('登录失败')
        return 1
    print('登录成功')

    # 上传一张示例图片，返回可引用的 URL
    img_url = ''
    boundary = '----cntheme'
    body = (f'--{boundary}\r\nContent-Disposition: form-data; name="image"; filename="demo.png"\r\n'
            f'Content-Type: image/png\r\n\r\n').encode() + png_gradient() + \
        f'\r\n--{boundary}--\r\n'.encode()
    try:
        r = urllib.request.Request(BASE + '/api/upload', method='POST', data=body,
                                   headers={'Content-Type': f'multipart/form-data; boundary={boundary}',
                                            'Cookie': f'crazynote_session={cookie}'})
        with urllib.request.urlopen(r) as resp:
            img_url = json.loads(resp.read().decode()).get('url', '')
        print('图片上传成功:', img_url)
    except Exception as e:
        print('图片上传失败（不影响截图）:', e)

    md = NOTE_MD
    if img_url:
        md = md.replace('### 列表与表格', f'![示例图片]({img_url})\n\n### 列表与表格')

    # 建一篇笔记并打开
    note = api('/api/notes', 'POST',
               {'category': '默认', 'title': 'Markdown 样式预览', 'body': md}, cookie)
    print('笔记已建:', note.get('file'))

    from playwright.async_api import async_playwright
    chrome = None
    for pat in (r'C:\Users\Administrator\AppData\Local\ms-playwright\chromium-*/chrome-win/chrome.exe',
                r'C:\Users\Administrator\AppData\Local\ms-playwright\chromium-*/chrome-win64/chrome.exe'):
        hits = sorted(glob.glob(pat))
        if hits:
            chrome = hits[-1]
            break
    print('浏览器:', chrome)

    errors = []
    async with async_playwright() as p:
        browser = await p.chromium.launch(executable_path=chrome, args=['--no-sandbox'])
        page = await browser.new_page(viewport={'width': 1280, 'height': 860})
        page.on('console', lambda m: errors.append(f'[console.{m.type}] {m.text}') if m.type == 'error' else None)
        page.on('pageerror', lambda e: errors.append(f'[pageerror] {e}'))

        await page.goto(BASE + '/', wait_until='networkidle')
        # 登录（若出现遮罩）
        if await page.locator('#login-overlay').is_visible():
            await page.fill('#login-password', pwd)
            await page.click('#btn-login')
            await page.wait_for_timeout(1200)

        # 打开刚建的笔记
        await page.evaluate("""async (args) => {
            await window.__crazyNote.api('/api/notes/' + encodeURIComponent(args.c) + '/' + encodeURIComponent(args.f));
        }""", {'c': note['category'], 'f': note['file']})
        await page.wait_for_timeout(600)
        await page.evaluate("""(args) => {
            window.__crazyNote.state.current = {category: args.c, file: args.f};
            document.getElementById('note-body').value = args.body;
            document.getElementById('note-title').value = 'Markdown 样式预览';
        }""", {'c': note['category'], 'f': note['file'], 'body': md})
        await page.click('#btn-preview')   # 切到阅读态
        await page.wait_for_timeout(1200)

        for theme in THEMES:
            await page.evaluate("""async (t) => {
                await window.__crazyNote.api('/api/settings', {method:'PUT', body:{markdown_theme:t}});
            }""", theme)
            await page.reload(wait_until='networkidle')
            if await page.locator('#login-overlay').is_visible():
                await page.fill('#login-password', pwd)
                await page.click('#btn-login')
                await page.wait_for_timeout(1200)
            await page.evaluate("""(args) => {
                document.getElementById('note-body').value = args.body;
                document.getElementById('note-title').value = 'Markdown 样式预览';
            }""", {'body': md})
            # 直接用服务端渲染，保证内容完整
            html = await page.evaluate("""async (b) => {
                const d = await window.__crazyNote.api('/api/render', {method:'POST', body:{markdown:b}});
                return d.html || '';
            }""", md)
            await page.evaluate("""(h) => {
                document.getElementById('preview-inner').innerHTML = h;
            }""", html)
            await page.wait_for_timeout(500)
            path = os.path.join(OUT, f'theme-{theme}.png')
            await page.locator('#preview').screenshot(path=path)
            print('已截图:', path)

            # 暗色模式再来一张
            await page.emulate_media(color_scheme='dark')
            await page.wait_for_timeout(350)
            dark = os.path.join(OUT, f'theme-{theme}-dark.png')
            await page.locator('#preview').screenshot(path=dark)
            await page.emulate_media(color_scheme='light')

        # 设置弹窗里的主题选择器
        await page.click('#btn-settings')
        await page.wait_for_timeout(600)
        path = os.path.join(OUT, 'settings-themes.png')
        await page.locator('.dialog.wide').screenshot(path=path)
        print('已截图:', path)
        await page.click('#btn-settings-close')
        await page.wait_for_timeout(300)

        await browser.close()

    if errors:
        print('\n前端错误:')
        for e in errors:
            print('  ', e)
    else:
        print('\n无前端控制台错误')
    return 0


if __name__ == '__main__':
    sys.exit(asyncio.run(main()))
