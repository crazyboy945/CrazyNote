/* popup.js — 扩展弹窗逻辑 */
(function () {
  'use strict';

  const $ = (id) => document.getElementById(id);
  const DEFAULTS = { server: '', token: '' };

  let settings = { ...DEFAULTS };
  let extracted = null;

  // ---------- 存储 ----------
  function loadSettings() {
    return new Promise((resolve) => {
      chrome.storage.local.get(DEFAULTS, (data) => {
        settings = { ...DEFAULTS, ...data };
        resolve(settings);
      });
    });
  }

  function saveSettings() {
    return new Promise((resolve) => {
      chrome.storage.local.set({ server: $('server').value.trim(), token: $('token').value.trim() }, resolve);
    });
  }

  // ---------- 状态 ----------
  function setStatus(msg, type) {
    const el = $('status');
    el.textContent = msg;
    el.className = 'status' + (type ? ' ' + type : '');
    if (type === 'ok') {
      setTimeout(() => { el.className = 'status'; }, 3000);
    }
  }

  // ---------- 提取正文 ----------
  function extractPage() {
    return new Promise((resolve) => {
      chrome.tabs.query({ active: true, currentWindow: true }, (tabs) => {
        if (!tabs.length || !tabs[0].id) { resolve({ error: '找不到当前标签页' }); return; }
        const tabId = tabs[0].id;
        chrome.scripting.executeScript({
          target: { tabId },
          func: function () {
            if (window.__crazyNoteExtract) return window.__crazyNoteExtract();
            // content script 尚未注入时，回退到整页 HTML
            return {
              title: document.title,
              content: '',
              html: document.documentElement.outerHTML,
              url: location.href,
              siteName: location.hostname,
              fallback: true,
            };
          }
        }, (results) => {
          if (chrome.runtime.lastError || !results || !results.length) {
            resolve({ error: '无法读取页面内容（可能受浏览器限制）' });
            return;
          }
          resolve(results[0].result || { error: '提取失败' });
        });
      });
    });
  }

  function getSelection() {
    return new Promise((resolve) => {
      chrome.tabs.query({ active: true, currentWindow: true }, (tabs) => {
        if (!tabs.length || !tabs[0].id) { resolve(''); return; }
        chrome.scripting.executeScript({
          target: { tabId: tabs[0].id },
          func: function () {
            if (window.__crazyNoteGetSelection) return window.__crazyNoteGetSelection();
            const s = window.getSelection();
            return s ? String(s).trim() : '';
          }
        }, (results) => {
          if (chrome.runtime.lastError || !results || !results.length) { resolve(''); return; }
          resolve(results[0].result || '');
        });
      });
    });
  }

  // ---------- 分类 ----------
  function loadCategories() {
    if (!settings.server) { showSetup(); return; }
    const sel = $('category');
    sel.innerHTML = '<option value="">加载中…</option>';

    fetch(settings.server.replace(/\/$/, '') + '/api/categories', {
      headers: settings.token ? { 'X-CrazyNote-Token': settings.token } : {},
    })
      .then((r) => {
        if (!r.ok) throw new Error('HTTP ' + r.status);
        return r.json();
      })
      .then((d) => {
        const cats = d.categories || [];
        sel.innerHTML = '';
        cats.forEach((c) => {
          const o = document.createElement('option');
          o.value = c.name;
          o.textContent = c.name + ' (' + c.count + ')';
          sel.appendChild(o);
        });
        if (!cats.find((c) => c.name === '摘录')) {
          const o = document.createElement('option');
          o.value = '摘录';
          o.textContent = '摘录 (新建)';
          sel.appendChild(o);
        }
      })
      .catch((e) => {
        sel.innerHTML = '<option value="">加载失败</option>';
        setStatus('无法连接 NAS: ' + e.message + '（请检查地址与令牌）', 'err');
      });
  }

  function createCategory(name) {
    if (!settings.server) { showSetup(); return; }
    fetch(settings.server.replace(/\/$/, '') + '/api/categories', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(settings.token ? { 'X-CrazyNote-Token': settings.token } : {}),
      },
      body: JSON.stringify({ name: name }),
    })
      .then((r) => r.json())
      .then((d) => {
        if (d.error) { setStatus(d.error, 'err'); return; }
        setStatus('分类已创建', 'ok');
        loadCategories();
        $('category').value = name;
      })
      .catch((e) => setStatus('创建失败: ' + e.message, 'err'));
  }

  // ---------- 保存 ----------
  function save() {
    const category = $('category').value || '摘录';
    const title = $('title').value.trim() || (extracted ? extracted.title : '');
    const tags = $('tags').value.split(',').map((t) => t.trim()).filter(Boolean);
    const useSelection = $('use-selection').checked;

    if (!settings.server) { showSetup(); return; }

    const payload = {
      category: category,
      title: title,
      url: extracted ? extracted.url : '',
      tags: tags,
    };

    if (useSelection) {
      payload.selection = selectionText || '';
      payload.body = selectionText || '';
    } else {
      // 优先发送 HTML，让服务端做正文提取与降噪；content 是 Readability 已清洗的结果
      if (extracted && extracted.content) {
        payload.html = extracted.content;
      } else if (extracted && extracted.html) {
        payload.html = extracted.html;
      }
    }

    if (!payload.html && !payload.body) {
      setStatus('没有可保存的内容', 'err');
      return;
    }

    setStatus('保存中…', 'info');
    fetch(settings.server.replace(/\/$/, '') + '/api/clip', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(settings.token ? { 'X-CrazyNote-Token': settings.token } : {}),
      },
      body: JSON.stringify(payload),
    })
      .then((r) => r.json().then((d) => ({ ok: r.ok, data: d })))
      .then((res) => {
        if (!res.ok || res.data.error) {
          setStatus('保存失败: ' + (res.data.error || '未知错误'), 'err');
          return;
        }
        setStatus('✅ 已保存到「' + res.data.category + '」/ ' + res.data.title, 'ok');
      })
      .catch((e) => setStatus('保存失败: ' + e.message, 'err'));
  }

  let selectionText = '';

  // ---------- 初始化 ----------
  function showSetup() {
    $('setup-box').hidden = false;
    $('main-box').hidden = true;
  }

  function init() {
    loadSettings().then(() => {
      $('server').value = settings.server;
      $('token').value = settings.token;

      if (!settings.server) { showSetup(); return; }

      extractPage().then((res) => {
        if (res.error) {
          setStatus(res.error, 'err');
          $('preview').value = '';
          return;
        }
        extracted = res;
        $('title').value = res.title || '';
        const body = res.textContent || stripHtml(res.content || '');
        $('preview').value = body.slice(0, 800);
        $('stats').textContent = '正文约 ' + (res.length || body.length) + ' 字符 · ' + (res.siteName || '');
        getSelection().then((s) => {
          selectionText = s;
          if (s) {
            $('use-selection').checked = true;
            $('preview').value = s.slice(0, 800);
            $('stats').textContent = '已选中 ' + s.length + ' 字符';
          }
        });
        loadCategories();
      });
    });
  }

  function stripHtml(h) {
    const d = document.createElement('div');
    d.innerHTML = h;
    return d.textContent || '';
  }

  // 事件
  $('btn-save').addEventListener('click', save);
  $('btn-save-setup').addEventListener('click', () => {
    saveSettings().then(() => {
      settings.server = $('server').value.trim();
      settings.token = $('token').value.trim();
      $('setup-box').hidden = true;
      $('main-box').hidden = false;
      init();
    });
  });
  $('link-setup').addEventListener('click', () => {
    $('setup-box').hidden = false;
    $('main-box').hidden = true;
  });
  $('link-refresh').addEventListener('click', loadCategories);
  $('link-new-cat').addEventListener('click', () => {
    const name = prompt('新分类名称');
    if (name) createCategory(name.trim());
  });

  init();
})();
