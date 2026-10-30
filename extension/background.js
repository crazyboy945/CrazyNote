/* background.js — 右键菜单摘录与安装引导 */
(function () {
  'use strict';

  chrome.runtime.onInstalled.addListener(() => {
    chrome.contextMenus.create({
      id: 'crazynote-clip-page',
      title: '摘录此页到 CrazyNote',
      contexts: ['page', 'selection'],
    });
    chrome.contextMenus.create({
      id: 'crazynote-clip-selection',
      title: '摘录选中内容到 CrazyNote',
      contexts: ['selection'],
    });
  });

  chrome.contextMenus.onClicked.addListener((info, tab) => {
    if (info.menuItemId !== 'crazynote-clip-page' && info.menuItemId !== 'crazynote-clip-selection') return;

    chrome.storage.local.get({ server: '', token: '' }, (settings) => {
      if (!settings.server) {
        chrome.notifications.create({
          type: 'basic',
          iconUrl: 'icons/icon128.png',
          title: 'CrazyNote',
          message: '请先点击扩展图标完成服务器设置',
        });
        return;
      }
      const selection = info.selectionText || '';
      chrome.scripting.executeScript({
        target: { tabId: tab.id },
        func: function () {
          if (window.__crazyNoteExtract) return window.__crazyNoteExtract();
          return {
            title: document.title,
            content: '',
            html: document.documentElement.outerHTML,
            url: location.href,
            siteName: location.hostname,
          };
        },
      }, (results) => {
        const data = results && results[0] && results[0].result;
        if (!data) return;

        const payload = {
          category: '摘录',
          title: data.title || '',
          url: data.url || '',
          tags: [],
        };
        if (selection) {
          payload.selection = selection;
          payload.body = selection;
        } else {
          payload.html = data.content || data.html || '';
        }

        fetch(settings.server.replace(/\/$/, '') + '/api/clip', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            ...(settings.token ? { 'X-CrazyNote-Token': settings.token } : {}),
          },
          body: JSON.stringify(payload),
        })
          .then((r) => r.json())
          .then((d) => {
            chrome.notifications.create({
              type: 'basic',
              iconUrl: 'icons/icon128.png',
              title: d.error ? '摘录失败' : '已摘录到 CrazyNote',
              message: d.error || (d.title + ' → ' + d.category),
            });
          })
          .catch((e) => {
            chrome.notifications.create({
              type: 'basic',
              iconUrl: 'icons/icon128.png',
              title: '摘录失败',
              message: e.message,
            });
          });
      });
    });
  });
})();
