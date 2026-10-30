"""验证网页端的代码语法高亮是否真正生效（检查实际计算颜色）。"""
import asyncio
import json
import os
import re
import sys

BASE = os.environ.get('CRAZYNOTE_BASE', 'http://127.0.0.1:18920')
LOG = os.environ.get('CRAZYNOTE_LOG', r'C:\Users\Administrator\AppData\Local\Temp\cn-hl\server.log')
OUT = os.environ.get('CRAZYNOTE_SHOT_DIR', r'C:\Users\Administrator\AppData\Local\Temp\cn-hl\shots')

MD = '''# 代码高亮测试

## Go

```go
package main

import "fmt"

func main() {
	fmt.Println("Hello, 世界")
}
```

## Python

```python
def greet(name: str) -> None:
    # 注释
    print(f"你好, {name}")
    return None
```

## JavaScript

```js
const arr = [1, 2, 3];
const doubled = arr.map(x => x * 2);
console.log(doubled);
```

## SQL

```sql
SELECT id, name FROM users WHERE age > 18 ORDER BY name;
```

## 无语言标记

```
plain text block
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
        print('无法读取口令')
        return 1

    errors = []
    async with async_playwright() as p:
        browser = await p.chromium.launch(
            executable_path=os.path.expanduser(
                '~/AppData/Local/ms-playwright/chromium-1223/chrome-win64/chrome.exe'))
        page = await browser.new_page(viewport={'width': 1280, 'height': 900})

        await page.goto(BASE, wait_until='networkidle')
        if await page.locator('#login-overlay:not([hidden])').count() > 0:
            await page.fill('#login-password', pwd)
            await page.click('#btn-login')
            await page.wait_for_timeout(1500)

        # 新建分类与笔记
        await page.click('#btn-new-category')
        await page.wait_for_timeout(300)
        await page.fill('#prompt-input', '高亮测试')
        await page.click('#btn-prompt-ok')
        await page.wait_for_timeout(700)

        await page.click('#btn-new-note')
        await page.wait_for_timeout(900)
        await page.fill('#note-title', '代码高亮测试')
        await page.evaluate("""(md) => {
            const ta = document.getElementById('note-body');
            ta.value = md;
            ta.dispatchEvent(new Event('input', {bubbles: true}));
        }""", MD)
        await page.click('#btn-save')
        await page.wait_for_timeout(1000)

        # 切到预览
        await page.click('#btn-preview')
        await page.wait_for_timeout(1500)

        # 检查代码块结构
        blocks = await page.locator('#preview pre.cnchroma').count()
        print(f'代码块数量（pre.cnchroma）: {blocks}')

        # 检查 token 数量
        tokens = await page.locator('#preview pre.cnchroma span[class^="cn"]').count()
        print(f'高亮 token 数量: {tokens}')

        # 关键：检查不同 token 是否有不同颜色
        colors = await page.evaluate("""() => {
            const out = {};
            const els = document.querySelectorAll('#preview pre.cnchroma span[class^="cn"]');
            for (const el of els) {
                const cls = el.className;
                const color = getComputedStyle(el).color;
                if (!out[cls]) out[cls] = color;
            }
            return out;
        }""")
        distinct = sorted(set(colors.values()))
        print(f'不同 token class 数: {len(colors)}')
        print(f'实际渲染出的不同颜色数: {len(distinct)}')
        for cls, color in list(colors.items())[:8]:
            print(f'  {cls} -> {color}')

        # 检查 CSS 是否加载
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
        print(f'高亮 CSS 已加载: {css_loaded}')

        await page.screenshot(path=f'{OUT}/highlight-light.png', full_page=True)

        # 暗色模式
        await page.emulate_media(color_scheme='dark')
        await page.wait_for_timeout(600)
        dark_colors = await page.evaluate("""() => {
            const els = document.querySelectorAll('#preview pre.cnchroma span[class^="cn"]');
            const set = new Set();
            for (const el of els) set.add(getComputedStyle(el).color);
            return set.size;
        }""")
        print(f'暗色模式下不同颜色数: {dark_colors}')
        await page.screenshot(path=f'{OUT}/highlight-dark.png', full_page=True)

        await browser.close()

    ok = blocks == 5 and tokens > 50 and len(distinct) > 5 and css_loaded
    print(f'\n结果: {"通过" if ok else "未通过"}')
    return 0 if ok else 1


if __name__ == '__main__':
    sys.exit(asyncio.run(main()))
