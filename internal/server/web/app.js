/* CrazyNote - 前端逻辑 */
(function () {
  'use strict';

  var BASE = window.CRAZYNOTE_BASE || '';

  // 阅读视图的 Markdown 样式主题，与服务端白名单保持一致
  var THEMES = ['default', 'reading', 'paper', 'card', 'compact'];

  var state = {
    categories: [],
    notes: [],
    current: null,          // 当前笔记对象
    currentCategory: '',    // 当前分类筛选
    currentTag: '',
    searchQuery: '',
    previewMode: true,
    session: null,
    settings: null,
    markdownTheme: 'default'
  };

  var $ = function (id) { return document.getElementById(id); };

  // ---------- API ----------
  function api(path, options) {
    options = options || {};
    options.headers = options.headers || {};
    if (options.body && typeof options.body !== 'string') {
      options.headers['Content-Type'] = 'application/json';
      options.body = JSON.stringify(options.body);
    }
    options.credentials = 'same-origin';
    return fetch(BASE + path, options).then(function (res) {
      return res.text().then(function (text) {
        var data = {};
        try { data = text ? JSON.parse(text) : {}; } catch (e) { data = { error: text }; }
        if (res.status === 401) {
          showLogin();
          throw new Error(data.error || '未认证');
        }
        if (!res.ok) throw new Error(data.error || ('请求失败 ' + res.status));
        return data;
      });
    });
  }

  // ---------- 提示 ----------
  var toastTimer = null;
  function toast(msg) {
    var el = $('toast');
    el.textContent = msg;
    el.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { el.hidden = true; }, 2400);
  }

  function status(msg) { $('status-bar').textContent = msg || ''; }

  // ---------- 登录 ----------
  function showLogin() {
    $('login-overlay').hidden = false;
    $('login-password').focus();
  }
  function hideLogin() { $('login-overlay').hidden = true; }

  function doLogin() {
    var pwd = $('login-password').value;
    api('/api/login', { method: 'POST', body: { password: pwd } })
      .then(function () {
        hideLogin();
        $('login-password').value = '';
        return bootstrap();
      })
      .catch(function (e) { toast(e.message); });
  }

  // ---------- 启动 ----------
  function bootstrap() {
    return api('/api/session')
      .then(function (s) {
        state.session = s;
        $('user-badge').textContent = s.user + (s.is_admin ? ' · 管理员' : '');
        return Promise.all([loadCategories(), loadSettings()]);
      })
      .then(function () { return loadNotes(); })
      .catch(function (e) {
        if (e.message !== '未认证') toast(e.message);
      });
  }

  function loadSettings() {
    return api('/api/settings').then(function (s) {
      state.settings = s;
      if (s.title) {
        $('app-title').textContent = s.title;
        document.title = s.title;
      }
      $('set-title').value = s.title || '';
      $('set-root').value = s.root || '';
      $('set-version').textContent = '版本 ' + (s.version || '未知');
      // 顶栏常驻版本号，不用进设置就能确认当前运行的是哪一版
      $('app-version').textContent = s.version ? 'v' + s.version : '';
      applyMarkdownTheme(s.markdown_theme || 'default');
      buildBookmarklet(s.root);
    });
  }

  // ---------- Markdown 样式 ----------
  // 切换阅读视图的样式主题，并同步设置面板的选中态。
  function applyMarkdownTheme(theme) {
    var t = THEMES.indexOf(theme) >= 0 ? theme : 'default';
    var el = $('preview');
    if (el) {
      for (var i = 0; i < THEMES.length; i++) {
        el.classList.remove('md-theme-' + THEMES[i]);
      }
      el.classList.add('md-theme-' + t);
    }
    state.markdownTheme = t;
    var cards = document.querySelectorAll('#theme-picker .theme-card');
    for (var k = 0; k < cards.length; k++) {
      cards[k].classList.toggle('active', cards[k].getAttribute('data-theme') === t);
    }
  }

  // 保存样式主题（非法值服务端会忽略）
  function saveMarkdownTheme(theme) {
    return api('/api/settings', { method: 'PUT', body: { markdown_theme: theme } })
      .then(function () { return loadSettings(); })
      .then(function () { toast('样式已切换'); })
      .catch(function (e) { toast(e.message); });
  }

  function loadCategories() {
    return api('/api/categories').then(function (d) {
      state.categories = d.categories || [];
      renderCategories();
    });
  }

  function loadNotes() {
    var path = '/api/notes';
    var params = [];
    if (state.currentCategory) params.push('category=' + encodeURIComponent(state.currentCategory));
    if (params.length) path += '?' + params.join('&');
    return api(path).then(function (d) {
      state.notes = d.notes || [];
      if (state.searchQuery) {
        var q = state.searchQuery.toLowerCase();
        state.notes = state.notes.filter(function (n) {
          return (n.title || '').toLowerCase().indexOf(q) >= 0 ||
                 (n.source || '').toLowerCase().indexOf(q) >= 0 ||
                 (n.tags || []).join(',').toLowerCase().indexOf(q) >= 0;
        });
      }
      if (state.currentTag) {
        state.notes = state.notes.filter(function (n) {
          return (n.tags || []).indexOf(state.currentTag) >= 0;
        });
      }
      renderNoteList();
      loadTags();
    });
  }

  function loadTags() {
    return api('/api/tags').then(function (d) {
      var tags = d.tags || {};
      var keys = Object.keys(tags).sort();
      var box = $('tag-cloud');
      box.innerHTML = '';
      keys.forEach(function (t) {
        var chip = document.createElement('span');
        chip.className = 'tag-chip' + (state.currentTag === t ? ' active' : '');
        chip.textContent = t + ' ' + tags[t];
        chip.onclick = function () {
          state.currentTag = state.currentTag === t ? '' : t;
          loadNotes();
        };
        box.appendChild(chip);
      });
    });
  }

  // ---------- 渲染 ----------
  function renderCategories() {
    var ul = $('category-list');
    ul.innerHTML = '';

    var all = document.createElement('li');
    all.className = 'category-item' + (state.currentCategory === '' ? ' active' : '');
    all.innerHTML = '<span class="cat-name">📚 全部笔记</span>';
    all.onclick = function () {
      state.currentCategory = '';
      state.currentTag = '';
      $('list-title').textContent = '全部笔记';
      renderCategories();
      loadNotes();
    };
    ul.appendChild(all);

    state.categories.forEach(function (c) {
      var li = document.createElement('li');
      li.className = 'category-item' + (state.currentCategory === c.name ? ' active' : '');
      li.innerHTML =
        '<span class="cat-name">📁 ' + escapeHtml(c.name) + '</span>' +
        '<span class="cat-count">' + c.count + '</span>' +
        '<button class="cat-del" title="删除分类">✕</button>';
      li.querySelector('.cat-name').onclick = function () {
        state.currentCategory = c.name;
        state.currentTag = '';
        $('list-title').textContent = c.name;
        renderCategories();
        loadNotes();
      };
      li.querySelector('.cat-del').onclick = function (e) {
        e.stopPropagation();
        confirmPrompt('删除分类', '将删除分类「' + c.name + '」及其全部笔记，是否继续？', function (val) {
          if (val !== c.name) { toast('请输入分类名以确认'); return false; }
          api('/api/categories/' + encodeURIComponent(c.name), { method: 'DELETE' })
            .then(function (d) {
              state.categories = d.categories || [];
              if (state.currentCategory === c.name) {
                state.currentCategory = '';
                $('list-title').textContent = '全部笔记';
              }
              renderCategories();
              loadNotes();
              toast('分类已删除');
            })
            .catch(function (err) { toast(err.message); });
          return true;
        });
      };
      ul.appendChild(li);
    });
  }

  function renderNoteList() {
    var ul = $('note-list');
    ul.innerHTML = '';
    $('list-empty').hidden = state.notes.length > 0;

    state.notes.forEach(function (n) {
      var li = document.createElement('li');
      li.className = 'note-item' + (state.current && state.current.file === n.file &&
                                    state.current.category === n.category ? ' active' : '');
      var date = new Date(n.updated);
      var dateStr = isNaN(date.getTime()) ? '' :
        date.getFullYear() + '-' + pad(date.getMonth() + 1) + '-' + pad(date.getDate()) +
        ' ' + pad(date.getHours()) + ':' + pad(date.getMinutes());

      var tagsHtml = '';
      (n.tags || []).forEach(function (t) {
        tagsHtml += '<span class="note-tag-inline">' + escapeHtml(t) + '</span>';
      });

      li.innerHTML =
        '<div class="note-title">' + escapeHtml(n.title || '未命名笔记') + '</div>' +
        '<div class="note-meta"><span>' + dateStr + '</span>' +
        (n.source ? '<span>🌐 网页</span>' : '') + '</div>' +
        (tagsHtml ? '<div class="note-tags">' + tagsHtml + '</div>' : '');

      li.onclick = function () { openNote(n.category, n.file); };
      ul.appendChild(li);
    });
  }

  function openNote(category, file) {
    api('/api/notes/' + encodeURIComponent(category) + '/' + encodeURIComponent(file))
      .then(function (n) {
        state.current = n;
        $('note-title').value = n.title || '';
        $('note-tags').value = (n.tags || []).join(', ');
        $('note-body').value = n.body || '';
        setMode(true);
        renderNoteList();
        status('已打开: ' + n.title);
      })
      .catch(function (e) { toast(e.message); });
  }

  // 预览渲染：交由服务端渲染，代码块带语法高亮。
  // 输入有防抖，避免每次按键都发请求。
  var previewTimer = null;
  function renderPreview() {
    var body = $('note-body').value;
    clearTimeout(previewTimer);
    previewTimer = setTimeout(function () {
      api('/api/render', { method: 'POST', body: { markdown: body } })
        .then(function (data) {
          $('preview-inner').innerHTML = data.html || '';
          bindPreviewLinks();
        })
        .catch(function () {
          // 服务端渲染失败时退回本地轻量渲染
          $('preview-inner').innerHTML = simpleMarkdown(body);
        });
    }, 180);
  }

  // 预览区里的外链交给系统浏览器打开
  function bindPreviewLinks() {
    var links = $('preview').querySelectorAll('a[href]');
    for (var i = 0; i < links.length; i++) {
      (function (a) {
        a.onclick = function (e) {
          e.preventDefault();
          window.open(a.getAttribute('href'), '_blank', 'noopener');
        };
      })(links[i]);
    }
  }

  // 极简 Markdown 渲染：够用于实时预览
  function simpleMarkdown(src) {
    var lines = src.split('\n');
    var out = [];
    var inCode = false, inList = false, inTable = false;
    var closeList = function () { if (inList) { out.push('</ul>'); inList = false; } };
    var closeTable = function () { if (inTable) { out.push('</tbody></table>'); inTable = false; } };

    for (var i = 0; i < lines.length; i++) {
      var line = lines[i];
      if (line.trim().indexOf('```') === 0) {
        closeList(); closeTable();
        out.push(inCode ? '</code></pre>' : '<pre><code>');
        inCode = !inCode;
        continue;
      }
      if (inCode) { out.push(escapeHtml(line)); continue; }

      // 表格
      if (/^\s*\|.*\|\s*$/.test(line)) {
        var cells = line.trim().replace(/^\||\|$/g, '').split('|').map(function (c) { return c.trim(); });
        if (/^[\s|:-]+$/.test(line)) continue;
        if (!inTable) {
          closeList();
          out.push('<table><thead><tr>' + cells.map(function (c) {
            return '<th>' + inlineMd(c) + '</th>';
          }).join('') + '</tr></thead><tbody>');
          inTable = true;
        } else {
          out.push('<tr>' + cells.map(function (c) { return '<td>' + inlineMd(c) + '</td>'; }).join('') + '</tr>');
        }
        continue;
      }
      closeTable();

      var h = line.match(/^(#{1,6})\s+(.*)$/);
      if (h) { closeList(); out.push('<h' + h[1].length + '>' + inlineMd(h[2]) + '</h' + h[1].length + '>'); continue; }

      if (/^\s*>\s?/.test(line)) {
        closeList();
        out.push('<blockquote>' + inlineMd(line.replace(/^\s*>\s?/, '')) + '</blockquote>');
        continue;
      }

      var li = line.match(/^\s*[-*+]\s+(.*)$/);
      if (li) {
        if (!inList) { out.push('<ul>'); inList = true; }
        out.push('<li>' + inlineMd(li[1]) + '</li>');
        continue;
      }
      closeList();

      if (/^\s*(-{3,}|\*{3,})\s*$/.test(line)) { out.push('<hr>'); continue; }
      if (line.trim() === '') continue;
      out.push('<p>' + inlineMd(line) + '</p>');
    }
    closeList(); closeTable();
    if (inCode) out.push('</code></pre>');
    return out.join('\n');
  }

  function inlineMd(text) {
    var s = escapeHtml(text);
    s = s.replace(/`([^`]+)`/g, '<code>$1</code>');
    s = s.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    s = s.replace(/(^|[^*])\*([^*]+)\*/g, '$1<em>$2</em>');
    s = s.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
    s = s.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, '<img src="$2" alt="$1">');
    return s;
  }

  function escapeHtml(s) {
    return String(s == null ? '' : s)
      .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
  }

  function pad(n) { return n < 10 ? '0' + n : '' + n; }

  // ---------- 图片上传 ----------
  function uploadImage(file) {
    var fd = new FormData();
    fd.append('image', file);
    status('正在上传图片…');
    fetch(BASE + '/api/upload', { method: 'POST', body: fd, credentials: 'same-origin' })
      .then(function (res) {
        return res.text().then(function (text) {
          var data = {};
          try { data = text ? JSON.parse(text) : {}; } catch (e) { data = { error: text }; }
          if (res.status === 401) { showLogin(); throw new Error(data.error || '未认证'); }
          if (!res.ok) throw new Error(data.error || ('上传失败 ' + res.status));
          return data;
        });
      })
      .then(function (d) {
        // 笔记里保存稳定的应用内绝对路径（与访问方式无关），
        // 网关前缀由服务端在渲染时补上，避免笔记被访问方式绑定。
        var url = d.url;
        var alt = (file.name || 'image').replace(/\.[^.]+$/, '');
        insertAtCursor('![' + alt + '](' + url + ')');
        status('图片已插入: ' + alt);
        toast('图片已上传并插入');
      })
      .catch(function (e) { toast(e.message); });
  }

  // 在文本框光标处插入文本（无选区时追加到末尾）。
  function insertAtCursor(text) {
    var ta = $('note-body');
    if (!ta) return;
    var start = ta.selectionStart || 0;
    var end = ta.selectionEnd || 0;
    var val = ta.value;
    ta.value = val.slice(0, start) + text + val.slice(end);
    var pos = start + text.length;
    ta.selectionStart = ta.selectionEnd = pos;
    ta.focus();
    if (state.previewMode) renderPreview();
  }

  // 阅读/编辑模式切换：默认阅读（预览，代码块带高亮），点击正文进入编辑。
  function setMode(reading) {
    state.previewMode = reading;
    $('editor-pane').classList.toggle('preview-mode', reading);
    $('btn-preview').textContent = reading ? '✏️ 编辑' : '👁 预览';
    if (reading) {
      renderPreview();
      status('阅读模式 · 点击内容进入编辑');
    } else {
      $('note-body').focus();
      status('编辑模式 · Ctrl+S 保存');
    }
  }

  // ---------- 备份与恢复 ----------
  // 导出：把整个笔记目录打包成 ZIP 下载（走 fetch 以带上会话 Cookie）。
  function exportBackup() {
    status('正在打包备份…');
    fetch(BASE + '/api/backup', { credentials: 'same-origin' })
      .then(function (res) {
        if (!res.ok) {
          return res.text().then(function (t) {
            var d = {};
            try { d = JSON.parse(t); } catch (e) { d = { error: t }; }
            throw new Error(d.error || ('导出失败 ' + res.status));
          });
        }
        return res.blob();
      })
      .then(function (blob) {
        var d = new Date();
        var name = 'crazynote-backup-' + d.getFullYear() + pad(d.getMonth() + 1) + pad(d.getDate()) +
          '-' + pad(d.getHours()) + pad(d.getMinutes()) + '.zip';
        var a = document.createElement('a');
        a.href = URL.createObjectURL(blob);
        a.download = name;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        setTimeout(function () { URL.revokeObjectURL(a.href); }, 2000);
        status('备份已导出: ' + name);
        toast('备份已导出');
      })
      .catch(function (e) { status(''); toast(e.message); });
  }

  // 恢复：用备份内容覆盖当前笔记（服务端会先自动快照现有内容）。
  function restoreBackup(file) {
    confirmPrompt('恢复备份',
      '恢复会用备份文件覆盖当前全部笔记。系统会先自动快照现有内容，出问题可从笔记目录下的 .crazynote/restore-backups/ 找回。确定继续？',
      function () {
        var fd = new FormData();
        fd.append('file', file);
        status('正在恢复…');
        fetch(BASE + '/api/restore', {
          method: 'POST',
          body: fd,
          credentials: 'same-origin',
          headers: { 'X-CrazyNote-Confirm': 'restore' }
        })
          .then(function (res) {
            return res.text().then(function (t) {
              var d = {};
              try { d = JSON.parse(t); } catch (e) { d = { error: t }; }
              if (!res.ok) throw new Error(d.error || ('恢复失败 ' + res.status));
              return d;
            });
          })
          .then(function (d) {
            var st = d.stats || {};
            toast('恢复完成：' + (st.notes || 0) + ' 篇笔记 / ' + (st.categories || 0) + ' 个分类');
            status('恢复完成');
            return bootstrap();
          })
          .catch(function (e) { status(''); toast(e.message); });
        return true;
      });
  }

  // ---------- 笔记操作 ----------
  function newNote() {
    var cat = state.currentCategory || (state.categories[0] && state.categories[0].name) || '默认';
    api('/api/notes', { method: 'POST', body: { category: cat, title: '未命名笔记', body: '' } })
      .then(function (n) {
        return loadCategories().then(function () { return loadNotes(); }).then(function () {
          openNote(n.category, n.file);
        });
      })
      .catch(function (e) { toast(e.message); });
  }

  function saveNote() {
    var title = $('note-title').value.trim() || '未命名笔记';
    var tags = $('note-tags').value.split(',').map(function (t) { return t.trim(); })
      .filter(function (t) { return t.length > 0; });
    var body = $('note-body').value;

    if (!state.current) {
      api('/api/notes', { method: 'POST', body: { category: state.currentCategory || '默认', title: title, body: body, tags: tags } })
        .then(function (n) {
          return loadCategories().then(function () { return loadNotes(); }).then(function () {
            openNote(n.category, n.file);
            toast('已保存');
          });
        })
        .catch(function (e) { toast(e.message); });
      return;
    }

    var c = state.current.category;
    var f = state.current.file;
    api('/api/notes/' + encodeURIComponent(c) + '/' + encodeURIComponent(f), {
      method: 'PUT', body: { title: title, body: body, tags: tags, source: state.current.source || '' }
    })
      .then(function (n) {
        state.current = n;
        $('note-title').value = n.title;
        return loadCategories().then(function () { return loadNotes(); });
      })
      .then(function () { toast('已保存'); status('保存于 ' + new Date().toLocaleTimeString()); })
      .catch(function (e) { toast(e.message); });
  }

  function deleteNote() {
    if (!state.current) { toast('没有选中的笔记'); return; }
    var c = state.current.category, f = state.current.file;
    confirmPrompt('删除笔记', '确定删除「' + state.current.title + '」？', function () {
      api('/api/notes/' + encodeURIComponent(c) + '/' + encodeURIComponent(f), { method: 'DELETE' })
        .then(function () {
          state.current = null;
          $('note-title').value = '';
          $('note-tags').value = '';
          $('note-body').value = '';
          $('preview-inner').innerHTML = '';
          return loadCategories().then(function () { return loadNotes(); });
        })
        .then(function () { toast('已删除'); })
        .catch(function (e) { toast(e.message); });
      return true;
    });
  }

  function moveNote() {
    if (!state.current) { toast('没有选中的笔记'); return; }
    var names = state.categories.map(function (c) { return c.name; });
    if (!names.length) { toast('还没有其他分类'); return; }
    promptInput('移动到分类', '可选分类: ' + names.join(' / '), names[0], function (target) {
      if (!target) return;
      if (names.indexOf(target) < 0) { toast('分类不存在: ' + target); return; }
      var c = state.current.category, f = state.current.file;
      api('/api/notes/' + encodeURIComponent(c) + '/' + encodeURIComponent(f) + '/move', {
        method: 'POST', body: { target: target }
      })
        .then(function () {
          state.current = null;
          return loadCategories().then(function () { return loadNotes(); });
        })
        .then(function () { toast('已移动到 ' + target); })
        .catch(function (e) { toast(e.message); });
    });
  }

  // ---------- 分类 ----------
  function newCategory() {
    promptInput('新分类名称', '分类会对应硬盘上的一个子目录', '', function (name) {
      if (!name) return;
      api('/api/categories', { method: 'POST', body: { name: name } })
        .then(function (d) {
          state.categories = d.categories || [];
          renderCategories();
          toast('分类已创建');
        })
        .catch(function (e) {
          // 分类已存在时直接选中它，不当作错误
          if (e.message && e.message.indexOf('已存在') >= 0) {
            state.currentCategory = name;
            $('list-title').textContent = name;
            loadCategories().then(function () {
              renderCategories();
              loadNotes();
            });
            toast('分类已存在，已切换到该分类');
            return;
          }
          toast(e.message);
        });
    });
  }

  // ---------- 弹窗 ----------
  var promptCb = null;
  // 确认弹窗：cb 返回 false 表示校验未通过，保持弹窗打开
  function confirmPrompt(title, desc, cb) {
    $('prompt-title').textContent = title;
    $('prompt-desc').textContent = desc;
    $('prompt-input').value = '';
    $('prompt-input').style.display = '';
    $('prompt-overlay').hidden = false;
    promptCb = cb;
    setTimeout(function () { $('prompt-input').focus(); }, 0);
  }

  // 输入弹窗：cb 收到输入值
  function promptInput(title, desc, def, cb) {
    $('prompt-title').textContent = title;
    $('prompt-desc').textContent = desc;
    $('prompt-input').value = def || '';
    $('prompt-input').style.display = '';
    $('prompt-overlay').hidden = false;
    promptCb = cb;
    setTimeout(function () { $('prompt-input').focus(); $('prompt-input').select(); }, 0);
  }

  // ---------- 设置 ----------
  function openSettings() {
    $('settings-overlay').hidden = false;
    $('set-title').value = (state.settings && state.settings.title) || 'CrazyNote';
  }

  function saveSettings() {
    api('/api/settings', {
      method: 'PUT',
      body: { title: $('set-title').value, markdown_theme: state.markdownTheme }
    })
      .then(function () { return loadSettings(); })
      .then(function () { toast('设置已保存'); })
      .catch(function (e) { toast(e.message); });
  }

  function genToken() {
    api('/api/tokens', { method: 'POST', body: {} })
      .then(function (d) {
        $('set-token').value = d.token;
        buildBookmarklet(state.settings && state.settings.root);
        toast('令牌已生成，请妥善保存');
      })
      .catch(function (e) { toast(e.message); });
  }

  function changePassword() {
    var pwd = $('set-password').value;
    if (!pwd) { toast('请输入新口令'); return; }
    api('/api/settings', { method: 'PUT', body: { password: pwd } })
      .then(function () { $('set-password').value = ''; toast('口令已更新'); })
      .catch(function (e) { toast(e.message); });
  }

  function buildBookmarklet(root) {
    var token = $('set-token').value || '';
    var host = window.location.origin + (BASE || '');
    var code = "javascript:(function(){var c=window.__crazyNoteClip||(window.__crazyNoteClip={});" +
      "fetch('" + host + "/api/clip',{method:'POST',credentials:'same-origin'," +
      "headers:{'Content-Type':'application/json'" + (token ? ",'X-CrazyNote-Token':'" + token + "'" : "") + "}," +
      "body:JSON.stringify({category:'摘录',title:document.title,url:location.href," +
      "html:document.documentElement.outerHTML,selection:String(window.getSelection())})})" +
      ".then(function(r){return r.json()}).then(function(d){alert(d.error||'已保存到 NAS')});})();";
    $('set-bookmarklet').value = code;
  }

  // ---------- 事件绑定 ----------
  function bind() {
    $('btn-login').onclick = doLogin;
    $('btn-login-cancel').onclick = function () {
      hideLogin();
      if (!state.session) {
        // 未登录时允许在网关模式下继续尝试
      }
    };
    $('login-password').onkeydown = function (e) { if (e.key === 'Enter') doLogin(); };

    $('btn-new-note').onclick = newNote;
    $('btn-new-category').onclick = newCategory;
    $('btn-save').onclick = saveNote;
    $('btn-delete').onclick = deleteNote;
    $('btn-move').onclick = moveNote;
    $('btn-settings').onclick = openSettings;
    $('btn-settings-close').onclick = function () { $('settings-overlay').hidden = true; };

    $('btn-preview').onclick = function () { setMode(!state.previewMode); };

    // 阅读模式下点击正文切换到编辑（链接除外，避免打断跳转）
    $('preview').onclick = function (e) {
      if (e.target.closest('a')) return;
      setMode(false);
    };

    $('btn-upload').onclick = function () { setMode(false); $('file-image').click(); };
    $('file-image').onchange = function (e) {
      var f = e.target.files && e.target.files[0];
      if (f) uploadImage(f);
      e.target.value = '';
    };

    $('btn-gen-token').onclick = genToken;
    $('btn-change-password').onclick = changePassword;
    $('btn-save-settings').onclick = saveSettings;

    $('btn-backup').onclick = exportBackup;
    $('btn-restore').onclick = function () { $('file-backup').click(); };
    $('file-backup').onchange = function (e) {
      var f = e.target.files && e.target.files[0];
      if (f) restoreBackup(f);
      e.target.value = '';
    };

    // Markdown 样式选择：点击即生效并保存
    $('theme-picker').onclick = function (e) {
      var card = e.target.closest ? e.target.closest('.theme-card') : null;
      if (!card) return;
      var theme = card.getAttribute('data-theme');
      if (!theme || theme === state.markdownTheme) return;
      applyMarkdownTheme(theme);
      saveMarkdownTheme(theme);
    };
    $('btn-copy-token').onclick = function () {
      var v = $('set-token').value;
      if (!v) { toast('请先生成令牌'); return; }
      navigator.clipboard.writeText(v).then(function () { toast('已复制'); });
    };

    $('btn-prompt-ok').onclick = function () {
      var val = $('prompt-input').value;
      var cb = promptCb;
      $('prompt-overlay').hidden = true;
      promptCb = null;
      if (cb) cb(val);
    };
    $('btn-prompt-cancel').onclick = function () {
      $('prompt-overlay').hidden = true;
      promptCb = null;
    };

    $('search').oninput = function (e) {
      state.searchQuery = e.target.value.trim();
      if (state.searchQuery) {
        api('/api/search?q=' + encodeURIComponent(state.searchQuery))
          .then(function (d) {
            state.notes = d.notes || [];
            renderNoteList();
          })
          .catch(function () {});
      } else {
        loadNotes();
      }
    };

    $('note-body').oninput = function () {
      if (state.previewMode) renderPreview();
    };
    $('note-title').onkeydown = function (e) {
      if (e.key === 'Enter') { e.preventDefault(); saveNote(); }
    };

    document.onkeydown = function (e) {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault();
        saveNote();
      }
      if (e.key === 'Escape') {
        $('settings-overlay').hidden = true;
        $('prompt-overlay').hidden = true;
      }
    };

    $('settings-overlay').onclick = function (e) {
      if (e.target === $('settings-overlay')) $('settings-overlay').hidden = true;
    };
    $('prompt-overlay').onclick = function (e) {
      if (e.target === $('prompt-overlay')) { $('prompt-overlay').hidden = true; promptCb = null; }
    };

    // 默认进入阅读模式
    setMode(true);
    applyMarkdownTheme(state.markdownTheme);
  }

  // 启动
  document.addEventListener('DOMContentLoaded', function () {
    bind();
    bootstrap();
  });

  window.__crazyNote = { state: state, api: api, reload: loadNotes };
})();
