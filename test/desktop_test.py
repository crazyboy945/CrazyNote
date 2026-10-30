"""连接 Electron 桌面端，验证连接服务端、笔记读写与代码高亮。"""
import asyncio
import os
import re
import sys

CDP = os.environ.get('CRAZYNOTE_CDP', 'http://127.0.0.1:9222')
BASE = os.environ.get('CRAZYNOTE_BASE', 'http://127.0.0.1:18920')
LOG = os.environ.get(
    'CRAZYNOTE_LOG',
    r'C:\Users\Administrator\AppData\Local\Temp\workbuddy-update-x64\cn-hl\server.log')
OUT = os.environ.get(
    'CRAZYNOTE_SHOT_DIR',
    r'C:\Users\Administrator\AppData\Local\Temp\cn-hl\desktop-shots')

MD = '''# 桌面端高亮验证

```go
package main

import "fmt"

func main() {
	fmt.Println("Hello")
}
```

```python
def hello():
    return "world"
```
'''


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
    pwd = read_password()
    if not pwd:
        print('无法读取服务端口令')
        return 1

    errors = []
    async with async_playwright() as p:
        browser = await p.chromium.connect_over_cdp(CDP)
        ctx = browser.contexts[0]
        pages = ctx.pages
        if not pages:
            print('未找到应用窗口')
            return 1
        page = pages[0]
        page.on('pageerror', lambda e: errors.append(f'[pageerror] {e}'))
        page.on('console', lambda m: errors.append(f'[console.error] {m.text}') if m.type == 'error' else None)

        await page.wait_for_timeout(800)
        print('标题:', await page.title())

        # 连接服务端
        if await page.locator('#connect-screen:not([hidden])').count() > 0:
            print('检测到连接页，填写地址与口令…')
            await page.fill('#server-url', BASE)
            await page.fill('#server-password', pwd)
            await page.click('#btn-connect')
            await page.wait_for_timeout(2500)

        connected = await page.locator('#main-screen:not([hidden])').count() > 0
        print('已进入主界面:', connected)
        if not connected:
            status = await page.locator('#connect-status').inner_text()
            print('连接状态:', status)
            return 1

        print('顶栏服务器:', await page.locator('#tb-server').inner_text())

        # 分类
        cats = await page.locator('.cat-item .cat-name').all_inner_texts()
        print('分类列表:', cats)

        # 新建分类
        await page.click('#btn-new-cat')
        await page.wait_for_timeout(400)
        await page.fill('#prompt-input', '桌面端测试')
        await page.click('#btn-prompt-ok')
        await page.wait_for_timeout(1000)
        cats2 = await page.locator('.cat-item .cat-name').all_inner_texts()
        print('新建后分类:', cats2)
        if '桌面端测试' not in cats2:
            errors.append('新建分类失败')

        # 新建笔记
        await page.click('#btn-new')
        await page.wait_for_timeout(1500)
        await page.fill('#note-title', '桌面端代码高亮测试')
        await page.fill('#note-body', MD)
        await page.wait_for_timeout(300)
        await page.click('#btn-save')
        await page.wait_for_timeout(1500)

        notes = await page.locator('.note-item .note-title').all_inner_texts()
        print('笔记列表:', notes)
        if not any('桌面端代码高亮测试' in n for n in notes):
            errors.append('保存后未出现在列表')

        # 切到预览验证高亮
        await page.click('#btn-preview')
        await page.wait_for_timeout(1800)

        blocks = await page.locator('#preview pre.cnchroma').count()
        print('高亮代码块数:', blocks)
        tokens = await page.locator('#preview pre.cnchroma span[class^="cn"]').count()
        print('高亮 token 数:', tokens)

        colors = await page.evaluate("""() => {
            const set = new Set();
            document.querySelectorAll('#preview pre.cnchroma span[class^="cn"]').forEach(el => {
                set.add(getComputedStyle(el).color);
            });
            return [...set];
        }""")
        print('实际渲染颜色数:', len(colors))
        for c in colors[:6]:
            print('   ', c)

        css_loaded = await page.evaluate("""() => {
            for (const sheet of document.styleSheets) {
                try {
                    for (const rule of sheet.cssRules) {
                        if (rule.selectorText && rule.selectorText.includes('cnchroma')) return true;
                    }
                } catch (e) {}
            }
            return false;
        }""")
        print('高亮 CSS 已加载:', css_loaded)

        await page.screenshot(path=f'{OUT}/desktop-highlight.png', full_page=True)

        if blocks < 2:
            errors.append('代码块未高亮')
        if len(colors) < 3:
            errors.append('高亮颜色种类不足')
        if not css_loaded:
            errors.append('高亮 CSS 未加载')

        # 剪贴板摘录
        await page.evaluate("""() => {
            window.crazynote.request('POST', '/api/clip', {
                category: '桌面端测试', title: '摘录测试', body: '来自桌面端的摘录内容'
            });
        }""")
        await page.wait_for_timeout(1200)
        notes2 = await page.locator('.note-item .note-title').all_inner_texts()
        print('摘录后列表:', notes2)
        if not any('摘录测试' in n for n in notes2):
            errors.append('摘录未成功')

        await page.screenshot(path=f'{OUT}/desktop-main.png', full_page=True)

    print('\n=== 控制台错误 ===')
    if errors:
        for e in errors:
            print('  ', e)
    else:
        print('  无')

    ok = not any('失败' in e or '未' in e for e in errors)
    print(f'\n结果: {"通过" if ok else "未通过"}')
    return 0 if ok else 1


if __name__ == '__main__':
    sys.exit(asyncio.run(main()))
