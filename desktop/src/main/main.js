'use strict';

const { app, BrowserWindow, ipcMain, session, shell, dialog, clipboard, nativeImage } = require('electron');
const path = require('node:path');
const fs = require('node:fs');
const os = require('node:os');

const Store = require('./store');
const ApiClient = require('./api');
const Tray = require('./tray');
const Menu = require('./menu');

const isDev = process.argv.includes('--dev');

// 允许通过 --remote-debugging-port 调试（自动化测试用）
const debugPortArg = process.argv.find((a) => a.startsWith('--remote-debugging-port='));
if (debugPortArg) {
  const port = parseInt(debugPortArg.split('=')[1], 10);
  if (port > 0) {
    app.commandLine.appendSwitch('remote-debugging-port', String(port));
  }
}

/** @type {BrowserWindow|null} */
let mainWindow = null;
/** @type {Tray|null} */
let tray = null;
/** @type {Store} */
let store;
/** @type {ApiClient} */
let api;

const APP_DIR = path.join(__dirname, '..', '..');

// ---------------------------------------------------------------- 窗口

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 820,
    minWidth: 940,
    minHeight: 600,
    show: false,
    backgroundColor: '#f5f6f8',
    title: 'CrazyNote',
    icon: path.join(APP_DIR, 'build', 'icon.png'),
    webPreferences: {
      preload: path.join(__dirname, '..', 'preload', 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
      spellcheck: false,
    },
  });

  mainWindow.loadFile(path.join(__dirname, '..', 'renderer', 'index.html'));

  mainWindow.once('ready-to-show', () => {
    if (!isDev) mainWindow.show();
    else mainWindow.show();
  });

  // 外部链接用系统浏览器打开
  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    if (/^https?:\/\//i.test(url)) {
      shell.openExternal(url);
      return { action: 'deny' };
    }
    return { action: 'allow' };
  });

  // 阻止导航到外部站点
  mainWindow.webContents.on('will-navigate', (event, url) => {
    const current = mainWindow.webContents.getURL();
    if (url !== current && !url.startsWith('file://')) {
      event.preventDefault();
      if (/^https?:\/\//i.test(url)) shell.openExternal(url);
    }
  });

  mainWindow.on('close', (event) => {
    if (!app.isQuitting && tray) {
      event.preventDefault();
      mainWindow.hide();
    }
  });

  mainWindow.on('closed', () => {
    mainWindow = null;
  });
}

// ---------------------------------------------------------------- IPC

function registerIpc() {
  // 配置读写
  ipcMain.handle('config:get', () => store.getPublicConfig());
  ipcMain.handle('config:set', (_e, patch) => store.setConfig(patch));
  ipcMain.handle('config:reset', () => store.reset());

  // 认证
  ipcMain.handle('auth:login', async (_e, { url, password }) => {
    const result = await api.login(url, password);
    if (result.ok) {
      store.setSession({ url, password });
    }
    return result;
  });
  ipcMain.handle('auth:logout', async () => {
    await api.logout();
    store.clearSession();
    return { ok: true };
  });
  ipcMain.handle('auth:session', async () => {
    const sess = store.getSession();
    if (!sess) return { ok: false, error: '未配置服务器' };
    return api.getSession(sess.url);
  });

  // 通用请求代理（规避浏览器 CORS 限制）
  ipcMain.handle('api:request', async (_e, { method, path: apiPath, body, query }) => {
    const sess = store.getSession();
    if (!sess) return { ok: false, error: '未连接服务器' };
    return api.request(sess.url, method, apiPath, { body, query });
  });

  // 取高亮主题 CSS：用于桌面端渲染代码块配色
  ipcMain.handle('api:highlight-css', async (_e, theme) => {
    const sess = store.getSession();
    if (!sess) return { ok: false, error: '未连接服务器' };
    const res = await api.request(sess.url, 'GET', '/api/highlight.css', { query: { theme } });
    if (!res.ok) return { ok: false, error: res.error };
    // 该接口返回纯 CSS，不是 JSON
    const raw = res.data && res.data.raw ? res.data.raw : '';
    return { ok: true, css: raw };
  });

  // 快速摘录：把剪贴板文本存到指定分类
  ipcMain.handle('clip:clipboard', async (_e, { category, title }) => {
    const sess = store.getSession();
    if (!sess) return { ok: false, error: '未连接服务器' };
    const text = clipboard.readText();
    if (!text || !text.trim()) return { ok: false, error: '剪贴板为空' };
    return api.request(sess.url, 'POST', '/api/clip', {
      body: {
        category: category || '摘录',
        title: title || (text.trim().split('\n')[0].slice(0, 60) || '剪贴板摘录'),
        body: text,
      },
    });
  });

  // 打开外部链接
  ipcMain.handle('shell:open', (_e, url) => {
    if (/^https?:\/\//i.test(url)) shell.openExternal(url);
    return { ok: true };
  });

  // 在系统默认编辑器中打开笔记文件
  ipcMain.handle('shell:showItem', (_e, p) => {
    if (p) shell.showItemInFolder(p);
    return { ok: true };
  });

  // 窗口控制
  ipcMain.handle('win:minimize', () => mainWindow && mainWindow.minimize());
  ipcMain.handle('win:maximize', () => {
    if (!mainWindow) return false;
    if (mainWindow.isMaximized()) mainWindow.unmaximize();
    else mainWindow.maximize();
    return mainWindow.isMaximized();
  });
  ipcMain.handle('win:close', () => mainWindow && mainWindow.close());
  ipcMain.handle('win:isMaximized', () => (mainWindow ? mainWindow.isMaximized() : false));

  // 托盘触发的操作
  ipcMain.on('tray:new-note', () => {
    if (mainWindow) {
      mainWindow.show();
      mainWindow.focus();
      mainWindow.webContents.send('menu:new-note');
    }
  });
  ipcMain.on('tray:clip', () => {
    if (mainWindow) {
      mainWindow.show();
      mainWindow.focus();
      mainWindow.webContents.send('menu:clip');
    }
  });
  ipcMain.on('tray:settings', () => {
    if (mainWindow) {
      mainWindow.show();
      mainWindow.focus();
      mainWindow.webContents.send('menu:settings');
    }
  });

  // 渲染层请求打开设置
  ipcMain.on('app:settings', () => {
    if (mainWindow) mainWindow.webContents.send('menu:settings');
  });
}

// ---------------------------------------------------------------- 生命周期

const singleInstance = app.requestSingleInstanceLock();
if (!singleInstance) {
  app.quit();
} else {
  app.on('second-instance', () => {
    if (mainWindow) {
      if (mainWindow.isMinimized()) mainWindow.restore();
      mainWindow.show();
      mainWindow.focus();
    }
  });

  app.whenReady().then(() => {
    store = new Store(path.join(app.getPath('userData'), 'config.json'));
    api = new ApiClient();

    registerIpc();
    createWindow();

    Menu.build({
      onNewNote: () => mainWindow && mainWindow.webContents.send('menu:new-note'),
      onClip: () => mainWindow && mainWindow.webContents.send('menu:clip'),
      onSearch: () => mainWindow && mainWindow.webContents.send('menu:search'),
      onSettings: () => mainWindow && mainWindow.webContents.send('menu:settings'),
      onToggleDevTools: () => mainWindow && mainWindow.webContents.toggleDevTools(),
      onQuit: () => {
        app.isQuitting = true;
        app.quit();
      },
      getWindow: () => mainWindow,
    });

    // 托盘
    tray = new Tray({
      onShow: () => {
        if (mainWindow) {
          mainWindow.show();
          mainWindow.focus();
        }
      },
      onNewNote: () => mainWindow && mainWindow.webContents.send('menu:new-note'),
      onClip: () => mainWindow && mainWindow.webContents.send('menu:clip'),
      onQuit: () => {
        app.isQuitting = true;
        app.quit();
      },
    });

    app.on('activate', () => {
      if (mainWindow) mainWindow.show();
    });
  });

  app.on('window-all-closed', () => {
    if (process.platform !== 'darwin' && !tray) app.quit();
  });

  app.on('before-quit', () => {
    app.isQuitting = true;
  });
}
