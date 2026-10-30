"""用真实浏览器验证「导出备份 → 删数据 → 恢复备份」的完整链路。"""
import asyncio
import glob
import json
import os
import re
import sys
import time
import zipfile
import urllib.request
import urllib.parse
import http.cookiejar

BASE = os.environ.get('CRAZYNOTE_BASE', 'http://127.0.0.1:18889')
LOG = os.environ.get('CRAZYNOTE_LOG', r'C:\Users\Administrator\AppData\Local\Temp\crazynote-test\server.log')
OUT = os.environ.get('CRAZYNOTE_SHOT_DIR', r'G:\WorkBuddy\CrazyNote\test\theme-shots')
WORK = os.environ.get('CRAZYNOTE_WORK', r'C:\Users\Administrator\AppData\Local\Temp\crazynote-test')


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

    from playwright.async_api import async_playwright
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
        ctx = await browser.new_context(accept_downloads=True)
        page = await ctx.new_page()
        page.on('console', lambda m: errors.append(f'[console.{m.type}] {m.text}') if m.type == 'error' else None)
        page.on('pageerror', lambda e: errors.append(f'[pageerror] {e}'))

        await page.goto(BASE + '/', wait_until='networkidle')
        if await page.locator('#login-overlay').is_visible():
            await page.fill('#login-password', pwd)
            await page.click('#btn-login')
            await page.wait_for_timeout(1200)

        # 造一篇笔记（默认是阅读态，先切到编辑态）
        await page.click('#btn-new-note')
        await page.wait_for_timeout(1000)
        if not await page.locator('#note-body').is_visible():
            await page.click('#btn-preview')
            await page.wait_for_timeout(300)
        await page.fill('#note-title', '备份功能测试笔记')
        await page.fill('#note-body', '这是用于验证备份与恢复的内容。')
        await page.click('#btn-save')
        await page.wait_for_timeout(1200)
        print('✓ 已创建测试笔记')

        # ---- 导出备份 ----
        await page.click('#btn-settings')
        await page.wait_for_timeout(400)
        async with page.expect_download() as dl_info:
            await page.click('#btn-backup')
        dl = await dl_info.value
        zip_path = os.path.join(WORK, 'browser-backup.zip')
        await dl.save_as(zip_path)
        print(f'✓ 导出备份: {dl.suggested_filename} -> {os.path.getsize(zip_path)} bytes')

        with zipfile.ZipFile(zip_path) as zf:
            names = zf.namelist()
        print('  ZIP 条目:', names)
        if not any('备份功能测试笔记' in n for n in names):
            print('✗ 备份里没有测试笔记')
            ok = False

        await page.click('#btn-settings-close')
        await page.wait_for_timeout(300)

        # ---- 删掉这篇笔记 ----
        await page.evaluate("""async () => {
            const notes = await window.__crazyNote.api('/api/notes');
            const t = notes.notes.find(n => n.title === '备份功能测试笔记');
            if (t) await window.__crazyNote.api('/api/notes/' + encodeURIComponent(t.category) + '/' + encodeURIComponent(t.file), {method:'DELETE'});
        }""")
        await page.reload(wait_until='networkidle')
        await page.wait_for_timeout(800)
        left = await page.evaluate("""async () => {
            const r = await window.__crazyNote.api('/api/notes');
            return r.notes.filter(n => n.title === '备份功能测试笔记').length;
        }""")
        print(f'  删除后同名笔记数: {left}')
        if left != 0:
            print('✗ 删除未生效')
            ok = False

        # ---- 恢复备份 ----
        await page.click('#btn-settings')
        await page.wait_for_timeout(400)
        await page.set_input_files('#file-backup', zip_path)
        await page.wait_for_timeout(600)
        # 确认弹窗
        if await page.locator('#prompt-overlay').is_visible():
            await page.click('#btn-prompt-ok')
            await page.wait_for_timeout(2500)

        back = await page.evaluate("""async () => {
            const r = await window.__crazyNote.api('/api/notes');
            return r.notes.filter(n => n.title === '备份功能测试笔记').length;
        }""")
        print(f'  恢复后同名笔记数: {back}')
        if back != 1:
            print('✗ 恢复未生效')
            ok = False
        else:
            print('✓ 恢复成功，笔记已找回')

        await page.locator('#settings-overlay').screenshot(path=os.path.join(OUT, 'backup-ui.png'))
        await browser.close()

    if errors:
        print('\n前端错误:')
        for e in errors:
            print('  ', e)
    print('\n结果:', '全部通过 ✓' if ok else '存在问题 ✗')
    return 0 if ok else 1


if __name__ == '__main__':
    sys.exit(asyncio.run(main()))
