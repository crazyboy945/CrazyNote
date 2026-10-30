"""用 Playwright 实际打开界面，验证前端交互并截图。"""
import asyncio
import os
import re
import sys
import time

BASE = os.environ.get('CRAZYNOTE_BASE', 'http://127.0.0.1:18899')
OUT = os.environ.get(
    'CRAZYNOTE_SHOT_DIR',
    r'C:\Users\Administrator\AppData\Local\Temp\crazynote-test\shots',
)
PWD = ''

log_path = os.environ.get('CRAZYNOTE_LOG', r'C:\Users\Administrator\AppData\Local\Temp\crazynote-test\server.log')


def read_password():
    """从服务端日志读取初始口令；服务可能尚未启动完成，需重试。"""
    for _ in range(30):
        if os.path.exists(log_path):
            content = open(log_path, encoding='utf-8', errors='ignore').read()
            m = re.search(r'访问口令: (\S+)', content)
            if m:
                return m.group(1)
        time.sleep(1)
    return ''


async def main():
    global PWD
    PWD = read_password()
    if not PWD:
        print('无法从日志读取访问口令')
        return 1
    from playwright.async_api import async_playwright
    os.makedirs(OUT, exist_ok=True)
    errors = []
    failed = []

    # 本地已有 Chromium 时直接指定可执行文件，避免重复下载
    launch_kwargs = {}
    local_chrome = os.path.expanduser(
        '~/AppData/Local/ms-playwright/chromium-1223/chrome-win64/chrome.exe')
    if os.path.exists(local_chrome):
        launch_kwargs['executable_path'] = local_chrome

    async with async_playwright() as p:
        browser = await p.chromium.launch(**launch_kwargs)
        page = await browser.new_page(viewport={'width': 1440, 'height': 900})

        page.on('console', lambda msg: errors.append(f'[console.{msg.type}] {msg.text}') if msg.type == 'error' else None)
        page.on('pageerror', lambda e: errors.append(f'[pageerror] {e}'))
        page.on('response', lambda r: failed.append(f'{r.status} {r.url}') if r.status >= 400 else None)

        await page.goto(BASE, wait_until='networkidle')

        # 登录
        if await page.locator('#login-overlay:not([hidden])').count() > 0:
            await page.fill('#login-password', PWD)
            await page.click('#btn-login')
            await page.wait_for_timeout(1500)

        print('标题栏:', await page.locator('#user-badge').inner_text())

        # 清理旧数据，保证测试幂等（重复执行不会产生重名笔记）
        await page.evaluate("""async () => {
            const cats = await fetch('/api/categories', {credentials:'same-origin'}).then(r=>r.json());
            for (const c of (cats.categories||[])) {
                await fetch('/api/categories/' + encodeURIComponent(c.name), {method:'DELETE', credentials:'same-origin'});
            }
        }""")
        await page.wait_for_timeout(1200)
        await page.reload(wait_until='networkidle')
        await page.wait_for_timeout(800)

        # 新建分类
        await page.click('#btn-new-category')
        await page.wait_for_timeout(300)
        await page.fill('#prompt-input', '技术')
        await page.click('#btn-prompt-ok')
        await page.wait_for_timeout(800)

        await page.click('#btn-new-category')
        await page.wait_for_timeout(300)
        await page.fill('#prompt-input', '读书')
        await page.click('#btn-prompt-ok')
        await page.wait_for_timeout(800)

        cats = await page.locator('.category-item .cat-name').all_inner_texts()
        print('分类列表:', cats)

        # 新建笔记
        await page.click('#btn-new-note')
        await page.wait_for_timeout(900)
        await page.fill('#note-title', 'CrazyNote 部署记录')
        await page.fill('#note-tags', '部署, 飞牛')
        await page.fill('#note-body', '''# CrazyNote

笔记以 **Markdown** 存储在 NAS 硬盘上。

## 分类即目录

- 每个分类一个子目录
- 纯 .md 文件，无数据库

## 网页摘录

> 一键把网页正文存成 Markdown

| 项目 | 说明 |
|------|------|
| 扩展 | Readability 抽正文 |
| 书签 | 任意浏览器可用 |

```bash
./CrazyNote -data /vol1/1000/Notes
```
''')
        await page.click('#btn-save')
        await page.wait_for_timeout(1200)

        # 预览
        await page.click('#btn-preview')
        await page.wait_for_timeout(600)
        await page.screenshot(path=f'{OUT}/01-editor-preview.png')

        preview_html = await page.locator('#preview').inner_html()
        checks = {
            '预览渲染表格': '<table>' in preview_html,
            '预览渲染代码块': '<pre>' in preview_html,
            '预览渲染加粗': '<strong>' in preview_html,
            '预览渲染引用': '<blockquote>' in preview_html,
            '预览渲染列表': '<ul>' in preview_html or '<li>' in preview_html,
        }
        for k, v in checks.items():
            print(('  [PASS] ' if v else '  [FAIL] ') + k)
            if not v:
                errors.append('预览渲染失败: ' + k)

        # 回到编辑模式，选中笔记
        await page.click('#btn-preview')
        await page.wait_for_timeout(400)
        await page.screenshot(path=f'{OUT}/02-editor.png')

        # 侧栏与列表
        note_items = await page.locator('.note-item .note-title').all_inner_texts()
        print('笔记列表:', note_items)
        if not note_items:
            errors.append('笔记列表为空')

        # 搜索
        await page.fill('#search', 'Markdown')
        await page.wait_for_timeout(900)
        print('搜索结果:', await page.locator('.note-item .note-title').all_inner_texts())

        await page.fill('#search', '')
        await page.wait_for_timeout(700)

        # 设置弹窗
        await page.click('#btn-settings')
        await page.wait_for_timeout(500)
        await page.click('#btn-gen-token')
        await page.wait_for_timeout(800)
        token = await page.locator('#set-token').input_value()
        print('令牌已生成:', token[:12] + '...' if token else '失败')
        if not token:
            errors.append('令牌生成失败')
        bm = await page.locator('#set-bookmarklet').input_value()
        print('书签脚本长度:', len(bm))
        if not bm.startswith('javascript:'):
            errors.append('书签脚本未生成')
        await page.screenshot(path=f'{OUT}/03-settings.png')
        await page.click('#btn-settings-close')

        # 整页截图
        await page.screenshot(path=f'{OUT}/04-full.png', full_page=True)

        await browser.close()

    print('\n=== 失败请求 ===')
    for f in failed:
        print('  ', f)
    print('\n=== 控制台错误 ===')
    if errors:
        for e in errors:
            print('  ', e)
    else:
        print('  无')
    return 1 if errors else 0


if __name__ == '__main__':
    sys.exit(asyncio.run(main()))
