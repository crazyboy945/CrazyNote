'use strict';

/**
 * CrazyNote 桌面端渲染层。
 * 所有服务端请求经 IPC 交给主进程发起，避免 CORS 限制。
 */

const api = window.crazynote;

const state = {
  config: null,
  categories: [],
  tags: [],
  currentCategory: '',
  currentTag: '',
  notes: [],
  current: null,      // { category, file, title, body, tags }
  dirty: false,
  preview: false,
  searchTimer: null,
};

// ---------------------------------------------------------------- 工具

const $ = (id) => document.getElementById(id);

function toast(msg, isError) {
  const el = $('toast');
  el.textContent = msg;
  el.className = 'toast show' + (isError ? ' err' : '');
  clearTimeout(toast._t);
  toast._t = setTimeout(() => { el.className = 'toast'; }, 2400);
}

function setStatus(text, kind) {
  const el = $('status-text');
  el.textContent = text;
  el.className = kind || '';
}

function escapeHtml(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

/**
 * 预览渲染：交由服务端渲染，代码块带语法高亮。
 *
 * 输入有防抖，避免每次按键都发请求。
 */
let previewTimer = null;
function schedulePreview() {
  clearTimeout(previewTimer);
  previewTimer = setTimeout(updatePreview, 180);
}

async function updatePreview() {
  const body = $('note-body').value;
  const res = await api.request('POST', '/api/render', { markdown: body });
  if (res.ok && res.data && res.data.html) {
    $('preview').innerHTML = res.data.html;
  } else {
    // 服务端不可用时回退到简单渲染
    $('preview').innerHTML = '<p style="color:var(--text-faint)">预览不可用：' +
      (res.error || '未能连接服务端') + '</p>';
  }
  bindPreviewLinks();
}

/** 预览区里的外链交给系统浏览器打开 */
function bindPreviewLinks() {
  const links = $('preview').querySelectorAll('a[href]');
  links.forEach((a) => {
    a.addEventListener('click', (e) => {
      e.preventDefault();
      api.openExternal(a.getAttribute('href'));
    });
  });
}

/**
 * 注入语法高亮主题样式。
 *
 * 桌面端渲染层是本地页面，取不到服务端静态资源，
 * 因此连接成功后从服务端拉一份高亮 CSS 注入到文档中。
 */
async function loadHighlightCSS() {
  const dark = window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches;
  const res = await api.highlightCSS(dark ? 'github-dark' : 'github');
  if (!res.ok || !res.css) return;
  let el = document.getElementById('highlight-theme');
  if (!el) {
    el = document.createElement('style');
    el.id = 'highlight-theme';
    document.head.appendChild(el);
  }
  el.textContent = res.css;
}

// ---------------------------------------------------------------- 数据

async function loadCategories() {
  const res = await api.request('GET', '/api/categories');
  if (!res.ok) { toast(res.error || '加载分类失败', true); return; }
  state.categories = res.data.categories || [];
  renderCategories();
  renderCategorySelect();
}

async function loadTags() {
  const res = await api.request('GET', '/api/tags');
  if (!res.ok) return;
  // 服务端返回 {标签名: 数量} 对象
  state.tags = res.data.tags || {};
  renderTags();
}

async function loadNotes() {
  const query = {};
  if (state.currentCategory) query.category = state.currentCategory;
  if (state.currentTag) query.tag = state.currentTag;

  const res = await api.request('GET', '/api/notes', null, query);
  if (!res.ok) { toast(res.error || '加载笔记失败', true); return; }
  state.notes = res.data.notes || [];
  renderNoteList();
}

async function searchNotes(q) {
  if (!q) { state.currentTag = ''; await loadNotes(); return; }
  const res = await api.request('GET', '/api/search', null, { q });
  if (!res.ok) { toast(res.error || '搜索失败', true); return; }
  state.notes = res.data.notes || [];
  renderNoteList();
  $('list-title').textContent = `搜索「${q}」`;
}

async function openNote(category, file) {
  const res = await api.request('GET', `/api/notes/${encodeURIComponent(category)}/${encodeURIComponent(file)}`);
  if (!res.ok) { toast(res.error || '打开笔记失败', true); return; }
  const n = res.data;
  state.current = {
    category: n.category,
    file: n.file,
    title: n.title || '',
    body: n.body || '',
    tags: n.tags || [],
    created: n.created,
    updated: n.updated,
    source: n.source || '',
  };
  state.dirty = false;
  renderEditor();
  renderNoteList();
}

function markDirty() {
  state.dirty = true;
  $('save-state').textContent = '未保存';
  $('save-state').className = 'saving';
}

async function saveNote() {
  if (!state.current) return;
  const title = $('note-title').value.trim();
  const body = $('note-body').value;
  const tags = $('note-tags').value.split(',').map((t) => t.trim()).filter(Boolean);
  const category = $('note-category').value || state.current.category;

  const res = await api.request('PUT', `/api/notes/${encodeURIComponent(state.current.category)}/${encodeURIComponent(state.current.file)}`, {
    title, body, tags, category,
  });
  if (!res.ok) { toast(res.error || '保存失败', true); return; }

  const saved = res.data;
  state.current.file = saved.file;
  state.current.title = saved.title;
  state.current.category = saved.category;
  // 同步已保存的正文，否则 renderEditor 会用旧内容覆盖用户输入
  state.current.body = body;
  state.current.tags = tags;
  state.dirty = false;
  $('save-state').textContent = '已保存';
  $('save-state').className = 'saved';
  $('note-title').value = saved.title;
  setStatus('已保存');
  await Promise.all([loadCategories(), loadNotes()]);
  renderEditor();
}

async function createNote() {
  const category = state.currentCategory || (state.categories[0] && state.categories[0].name) || '笔记';
  const res = await api.request('POST', '/api/notes', {
    category,
    title: '未命名笔记',
    body: '',
    tags: [],
  });
  if (!res.ok) { toast(res.error || '新建失败', true); return; }
  await loadCategories();
  await openNote(res.data.category, res.data.file);
  $('note-title').focus();
  $('note-title').select();
  setStatus('已新建笔记');
}

async function deleteCurrent() {
  if (!state.current) return;
  if (!confirm(`确定删除「${state.current.title}」？此操作不可撤销。`)) return;
  const res = await api.request('DELETE', `/api/notes/${encodeURIComponent(state.current.category)}/${encodeURIComponent(state.current.file)}`);
  if (!res.ok) { toast(res.error || '删除失败', true); return; }
  state.current = null;
  $('editor').hidden = true;
  $('empty-state').hidden = false;
  await Promise.all([loadCategories(), loadNotes()]);
  toast('已删除');
}

// ---------------------------------------------------------------- 渲染

function renderCategories() {
  const list = $('cat-list');
  const total = state.categories.reduce((s, c) => s + (c.count || 0), 0);
  const items = [{ name: '全部笔记', count: total, special: true }].concat(state.categories);

  list.innerHTML = '';
  items.forEach((c) => {
    const li = document.createElement('li');
    li.className = 'cat-item' + (state.currentCategory === c.name || (c.special && !state.currentCategory) ? ' active' : '');
    li.innerHTML = `
      <span class="cat-icon">${c.special ? '📚' : '📁'}</span>
      <span class="cat-name"></span>
      <span class="cat-count">${c.count || 0}</span>`;
    li.querySelector('.cat-name').textContent = c.name;
    li.addEventListener('click', () => {
      state.currentCategory = c.special ? '' : c.name;
      state.currentTag = '';
      $('list-title').textContent = c.name;
      loadNotes();
      renderCategories();
      renderTags();
    });
    if (!c.special) {
      const del = document.createElement('button');
      del.className = 'cat-del';
      del.textContent = '×';
      del.title = '删除分类';
      del.addEventListener('click', async (e) => {
        e.stopPropagation();
        if (!confirm(`删除分类「${c.name}」？分类下的笔记会一并移到回收区。`)) return;
        const r = await api.request('DELETE', `/api/categories/${encodeURIComponent(c.name)}`);
        if (!r.ok) { toast(r.error || '删除分类失败', true); return; }
        if (state.currentCategory === c.name) state.currentCategory = '';
        await loadCategories();
        await loadNotes();
        toast('分类已删除');
      });
      li.appendChild(del);
    }
    list.appendChild(li);
  });
}

function renderTags() {
  const box = $('tag-cloud');
  box.innerHTML = '';
  // 服务端返回的是 {标签名: 数量} 对象
  const tags = state.tags || {};
  const names = Object.keys(tags).sort();
  names.forEach((name) => {
    const el = document.createElement('span');
    el.className = 'tag-pill' + (state.currentTag === name ? ' active' : '');
    el.textContent = `${name} (${tags[name]})`;
    el.addEventListener('click', () => {
      state.currentTag = state.currentTag === name ? '' : name;
      $('list-title').textContent = state.currentTag ? `标签：${state.currentTag}` : (state.currentCategory || '全部笔记');
      renderTags();
      loadNotes();
    });
    box.appendChild(el);
  });
}

function renderCategorySelect() {
  const sel = $('note-category');
  const cur = sel.value;
  sel.innerHTML = '';
  state.categories.forEach((c) => {
    const o = document.createElement('option');
    o.value = c.name;
    o.textContent = c.name;
    sel.appendChild(o);
  });
  if (cur) sel.value = cur;
}

function renderNoteList() {
  const box = $('note-list');
  box.innerHTML = '';
  $('list-count').textContent = state.notes.length ? `${state.notes.length} 篇` : '';
  $('item-count').textContent = `共 ${state.notes.length} 篇`;

  if (!state.notes.length) {
    const empty = document.createElement('div');
    empty.style.cssText = 'padding:24px 14px;color:var(--text-faint);font-size:12px;text-align:center';
    empty.textContent = '没有匹配的笔记';
    box.appendChild(empty);
    return;
  }

  state.notes.forEach((n) => {
    const div = document.createElement('div');
    const active = state.current && state.current.category === n.category && state.current.file === n.file;
    div.className = 'note-item' + (active ? ' active' : '');
    const time = n.updated ? new Date(n.updated).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '';
    div.innerHTML = `<div class="note-title"></div><div class="note-sub"><span class="sub-cat"></span><span class="dot">·</span><span>${escapeHtml(time)}</span></div>`;
    div.querySelector('.note-title').textContent = n.title || '未命名笔记';
    div.querySelector('.sub-cat').textContent = n.category;
    div.addEventListener('click', () => openNote(n.category, n.file));
    box.appendChild(div);
  });
}

function renderEditor() {
  const n = state.current;
  if (!n) { $('editor').hidden = true; $('empty-state').hidden = false; return; }
  $('editor').hidden = false;
  $('empty-state').hidden = true;
  $('note-title').value = n.title;
  $('note-tags').value = (n.tags || []).join(', ');
  $('note-body').value = n.body;
  $('note-category').value = n.category;

  const bits = [];
  if (n.created) bits.push(`创建于 ${new Date(n.created).toLocaleString('zh-CN')}`);
  if (n.updated) bits.push(`更新于 ${new Date(n.updated).toLocaleString('zh-CN')}`);
  if (n.source) bits.push('含来源链接');
  $('editor-meta').textContent = bits.join('  ·  ');

  if (state.preview) schedulePreview();
  $('save-state').textContent = '';
}

// ---------------------------------------------------------------- 预览

function togglePreview() {
  state.preview = !state.preview;
  $('note-body').hidden = state.preview;
  $('preview').hidden = !state.preview;
  $('btn-preview').textContent = state.preview ? '编辑' : '预览';
  if (state.preview) schedulePreview();
}

// ---------------------------------------------------------------- 弹窗

function promptDialog(title, label, value) {
  return new Promise((resolve) => {
    $('prompt-title').textContent = title;
    $('prompt-label').textContent = label;
    $('prompt-input').value = value || '';
    $('prompt-overlay').hidden = false;
    $('prompt-input').focus();
    $('prompt-input').select();

    const done = (v) => {
      $('prompt-overlay').hidden = true;
      $('btn-prompt-ok').removeEventListener('click', ok);
      $('btn-prompt-cancel').removeEventListener('click', cancel);
      $('prompt-input').removeEventListener('keydown', onKey);
      resolve(v);
    };
    const ok = () => done($('prompt-input').value.trim());
    const cancel = () => done(null);
    const onKey = (e) => {
      if (e.key === 'Enter') { e.preventDefault(); ok(); }
      else if (e.key === 'Escape') { e.preventDefault(); cancel(); }
    };
    $('btn-prompt-ok').addEventListener('click', ok);
    $('btn-prompt-cancel').addEventListener('click', cancel);
    $('prompt-input').addEventListener('keydown', onKey);
  });
}

// ---------------------------------------------------------------- 连接

async function doConnect(url, password) {
  const btn = $('btn-connect');
  btn.disabled = true;
  btn.textContent = '连接中…';
  const st = $('connect-status');
  st.className = 'status show info';
  st.textContent = '正在连接 ' + url + ' …';

  const res = await api.login(url, password);
  btn.disabled = false;
  btn.textContent = '连接';

  if (!res.ok) {
    st.className = 'status show err';
    st.textContent = '连接失败：' + (res.error || '未知错误');
    return false;
  }
  st.className = 'status show ok';
  st.textContent = `已连接，当前用户：${res.user || 'admin'}`;

  await enterApp();
  return true;
}

async function enterApp() {
  $('connect-screen').hidden = true;
  $('main-screen').hidden = false;

  const cfg = await api.getConfig();
  state.config = cfg;
  $('tb-server').textContent = cfg.currentUrl || '';
  $('conn-state').textContent = '已连接';
  $('conn-state').className = 'conn-state';

  await loadCategories();
  await loadTags();
  await loadNotes();
  loadHighlightCSS();
  setStatus('就绪');
}

async function disconnect() {
  await api.logout();
  const cfg = await api.resetConfig();
  state.config = cfg;
  $('main-screen').hidden = true;
  $('connect-screen').hidden = false;
  $('connect-status').className = 'status';
  $('server-password').value = '';
  toast('已断开连接');
}

// ---------------------------------------------------------------- 事件

function bindEvents() {
  // 连接页
  $('btn-connect').addEventListener('click', async () => {
    const url = $('server-url').value.trim();
    if (!url) { toast('请填写服务器地址', true); return; }
    await doConnect(url, $('server-password').value);
  });

  $('btn-test').addEventListener('click', async () => {
    const url = $('server-url').value.trim();
    if (!url) { toast('请填写服务器地址', true); return; }
    const st = $('connect-status');
    st.className = 'status show info';
    st.textContent = '测试中…';
    const res = await api.request('GET', '/api/health');
    if (res.ok) { st.className = 'status show ok'; st.textContent = '服务器可达，服务正常。'; }
    else { st.className = 'status show err'; st.textContent = '无法连接：' + (res.error || '未知错误'); }
  });

  $('server-password').addEventListener('keydown', (e) => {
    if (e.key === 'Enter') $('btn-connect').click();
  });

  // 主界面
  $('btn-new').addEventListener('click', createNote);
  $('btn-clip').addEventListener('click', doClip);
  $('btn-settings').addEventListener('click', openSettings);
  $('btn-sidebar').addEventListener('click', () => $('sidebar').classList.toggle('collapsed'));
  $('btn-disconnect').addEventListener('click', disconnect);
  $('btn-save').addEventListener('click', saveNote);
  $('btn-preview').addEventListener('click', togglePreview);
  $('btn-delete').addEventListener('click', deleteCurrent);
  $('btn-new-cat').addEventListener('click', async () => {
    const name = await promptDialog('新建分类', '分类名称（对应硬盘上的一个子目录）', '');
    if (!name) return;
    const res = await api.request('POST', '/api/categories', { name });
    if (!res.ok) {
      if ((res.error || '').includes('已存在')) { toast('该分类已存在', true); await loadCategories(); return; }
      toast(res.error || '创建分类失败', true);
      return;
    }
    await loadCategories();
    toast('分类已创建');
  });

  $('note-title').addEventListener('input', markDirty);
  $('note-tags').addEventListener('input', markDirty);
  $('note-body').addEventListener('input', () => { markDirty(); if (state.preview) schedulePreview(); });

  $('search').addEventListener('input', (e) => {
    clearTimeout(state.searchTimer);
    const q = e.target.value.trim();
    state.searchTimer = setTimeout(() => {
      if (q) { $('list-title').textContent = `搜索「${q}」`; searchNotes(q); }
      else { $('list-title').textContent = state.currentCategory || '全部笔记'; loadNotes(); }
    }, 260);
  });

  // 设置
  $('btn-settings-close').addEventListener('click', () => { $('settings-overlay').hidden = true; });
  $('btn-settings-save').addEventListener('click', async () => {
    const url = $('set-url').value.trim();
    const pwd = $('set-password').value;
    const cat = $('set-default-cat').value.trim();
    await api.setConfig({ server: { url } });
    if (pwd) await api.login(url, pwd);
    if (cat) await api.setConfig({ lastCategory: cat });
    $('settings-overlay').hidden = true;
    toast('设置已保存');
    if (url && url !== state.config.currentUrl) {
      toast('服务器地址已变更，正在重连…');
      await doConnect(url, pwd);
    }
  });

  // 快捷键
  document.addEventListener('keydown', (e) => {
    const mod = e.ctrlKey || e.metaKey;
    if (mod && e.key.toLowerCase() === 's') { e.preventDefault(); saveNote(); }
    else if (mod && e.key.toLowerCase() === 'n') { e.preventDefault(); createNote(); }
    else if (mod && e.shiftKey && e.key.toLowerCase() === 'v') { e.preventDefault(); doClip(); }
    else if (mod && e.key.toLowerCase() === 'f') { e.preventDefault(); $('search').focus(); $('search').select(); }
    else if (mod && e.key === ',') { e.preventDefault(); openSettings(); }
  });

  // 菜单事件
  api.onMenu('menu:new-note', createNote);
  api.onMenu('menu:clip', doClip);
  api.onMenu('menu:search', () => { $('search').focus(); $('search').select(); });
  api.onMenu('menu:settings', openSettings);

  // 离开前提醒未保存
  window.addEventListener('beforeunload', (e) => {
    if (state.dirty) { e.preventDefault(); e.returnValue = ''; }
  });
}

async function doClip() {
  const cat = state.config && state.config.lastCategory ? state.config.lastCategory : '摘录';
  const res = await api.clipClipboard(cat);
  if (!res.ok) { toast(res.error || '摘录失败', true); return; }
  toast('已存入「' + (res.data && res.data.category) + '」');
  await loadCategories();
  await loadNotes();
}

async function openSettings() {
  const cfg = state.config || (await api.getConfig());
  $('set-url').value = cfg.currentUrl || (cfg.server && cfg.server.url) || '';
  $('set-password').value = '';
  $('set-default-cat').value = cfg.lastCategory || '摘录';
  $('settings-overlay').hidden = false;
}

// ---------------------------------------------------------------- 启动

(async function init() {
  bindEvents();
  const cfg = await api.getConfig();
  state.config = cfg;
  if (cfg.currentUrl) {
    $('server-url').value = cfg.currentUrl;
    const st = $('connect-status');
    st.className = 'status show info';
    st.textContent = '正在连接已保存的服务器…';
    const res = await api.session();
    if (res.ok) { await enterApp(); return; }
    st.className = 'status show err';
    st.textContent = '需要重新登录：' + (res.error || '会话已失效');
  }
})();
