"""在运行中的 Electron 渲染层里直接调用 API，定位列表为空的原因。"""
import asyncio
import os
import sys

CDP = os.environ.get('CRAZYNOTE_CDP', 'http://127.0.0.1:9222')


async def main():
    from playwright.async_api import async_playwright
    async with async_playwright() as p:
        browser = await p.chromium.connect_over_cdp(CDP)
        page = browser.contexts[0].pages[0]
        await page.wait_for_timeout(500)

        print('=== 直接调 GET /api/notes ===')
        r = await page.evaluate("""async () => {
            const res = await window.crazynote.request('GET', '/api/notes');
            return res;
        }""")
        print('ok =', r.get('ok'), '| error =', r.get('error'))
        data = r.get('data') or {}
        notes = data.get('notes') or []
        print('笔记数:', len(notes))
        for n in notes[:5]:
            print('   -', n.get('title'), '|', n.get('category'))

        print('\n=== 带 query 调用 ===')
        r2 = await page.evaluate("""async () => {
            return await window.crazynote.request('GET', '/api/notes', null, {category: '技术'});
        }""")
        print('ok =', r2.get('ok'), '| error =', r2.get('error'))
        d2 = r2.get('data') or {}
        print('笔记数:', len(d2.get('notes') or []))

        print('\n=== 渲染层 state ===')
        st = await page.evaluate("""() => {
            return {
                currentCategory: window.__state ? window.__state.currentCategory : 'n/a',
                hasState: !!window.__state,
            };
        }""")
        print(st)

        print('\n=== DOM 检查 ===')
        print('note-list 子元素数:', await page.locator('#note-list > *').count())
        print('列表 innerHTML 片段:', (await page.locator('#note-list').inner_html())[:200])


if __name__ == '__main__':
    asyncio.run(main())
