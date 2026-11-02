"""验证桌面端「网页摘录」：粘贴链接 → 主进程抓取正文 → 服务端提取并保存为笔记。"""
import asyncio
import os
import re
import sys
import threading
import http.server

CDP = os.environ.get('CRAZYNOTE_CDP', 'http://127.0.0.1:9222')
BASE = os.environ.get('CRAZYNOTE_BASE', 'http://127.0.0.1:18920')
LOG = os.environ.get(
    'CRAZYNOTE_LOG',
    r'C:\Users\Administrator\AppData\Local\Temp\workbuddy-update-x64\cn-hl\server.log')
OUT = os.environ.get(
    'CRAZYNOTE_SHOT_DIR',
    r'C:\Users\Administrator\AppData\Local\Temp\cn-hl\desktop-shots')

# 一段带导航/页脚噪声的文章，用于验证服务端正文提取能过滤噪声。
ARTICLE = """<!doctype html><html><head><title>测试文章标题</title>
<meta property="og:title" content="测试文章标题"></head><body>
<nav><ul><li>菜单一</li><li>菜单二</li></ul></nav>
<article>
  <h1>测试文章标题</h1>
  <p>这是正文第一段，用于验证桌面端网页摘录功能能把链接抓取为正文。</p>
  <p>这是第二段，包含更多内容以便提取出足够的文本。</p>
</article>
<footer>页脚版权信息不应出现在摘录结果里</footer>
</body></html>"""


class _Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = ARTICLE.encode('utf-8')
        self.send_response(200)
        self.send_header('Content-Type', 'text/html; charset=utf-8')
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *a):
        pass


def read_password():
    for _ in range(30):
        if os.path.exists(LOG):
            m = re.search(r'访问口令: (\S+)', open(LOG, encoding='utf-8', errors='ignore').read())
            if m:
                return m.group(1)
        import time
        time.sleep(1)
    return ''


async def main():
    from playwright.async_api import async_playwright

    os.makedirs(OUT, exist_ok=True)
    srv = http.server.HTTPServer(('127.0.0.1', 0), _Handler)
    port = srv.server_address[1]
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    article_url = f'http://127.0.0.1:{port}/'

    pwd = read_password()
    if not pwd:
        print('无法读取服务端口令')
        return 1

    errors = []
    async with async_playwright() as p:
        browser = await p.chromium.connect_over_cdp(CDP)
        ctx = browser.contexts[0]
        page = ctx.pages[0] if ctx.pages else await ctx.new_page()
        page.on('pageerror', lambda e: errors.append(f'[pageerror] {e}'))
        page.on('console', lambda m: errors.append(f'[console.error] {m.text}') if m.type == 'error' else None)

        await page.wait_for_timeout(800)
        if await page.locator('#connect-screen:not([hidden])').count() > 0:
            await page.fill('#server-url', BASE)
            await page.fill('#server-password', pwd)
            await page.click('#btn-connect')
            await page.wait_for_timeout(2500)

        if not await page.locator('#main-screen:not([hidden])').count() > 0:
            print('未进入主界面')
            return 1

        # 打开网页摘录弹窗
        await page.click('#btn-webclip')
        await page.wait_for_timeout(400)
        opened = await page.locator('#webclip-overlay:not([hidden])').count() > 0
        print('网页摘录弹窗已打开:', opened)
        if not opened:
            errors.append('网页摘录弹窗未打开')

        await page.fill('#webclip-url', article_url)
        await page.click('#btn-webclip-save')
        await page.wait_for_timeout(2500)

        notes = await page.locator('.note-item .note-title').all_inner_texts()
        print('笔记列表:', notes)
        if not any('测试文章标题' in n for n in notes):
            errors.append('网页摘录未生成笔记')

        # 打开该笔记确认正文中不含页脚噪声、且包含正文
        try:
            await page.locator('.note-item .note-title', has_text='测试文章标题').first.click()
            await page.wait_for_timeout(800)
            body = await page.locator('#note-body').input_value()
            print('摘录正文片段:', body[:80].replace('\n', ' '))
            if '页脚版权信息' in body:
                errors.append('摘录正文混入了页脚噪声')
            if '这是正文第一段' not in body:
                errors.append('摘录正文缺失')
        except Exception as e:
            errors.append(f'打开摘录笔记失败: {e}')

        await page.screenshot(path=f'{OUT}/desktop-webclip.png', full_page=True)
        srv.shutdown()

    print('\n=== 控制台错误 ===')
    if errors:
        for e in errors:
            print('  ', e)
    else:
        print('  无')

    ok = not errors
    print(f'\n结果: {"通过" if ok else "未通过"}')
    return 0 if ok else 1


if __name__ == '__main__':
    sys.exit(asyncio.run(main()))
