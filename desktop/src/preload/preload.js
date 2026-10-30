'use strict';

const { contextBridge, ipcRenderer } = require('electron');

/**
 * 渲染层与主进程之间的安全桥接。
 * 渲染层无法直接访问 Node API，只能调用这里显式暴露的方法。
 */
contextBridge.exposeInMainWorld('crazynote', {
  // 配置
  getConfig: () => ipcRenderer.invoke('config:get'),
  setConfig: (patch) => ipcRenderer.invoke('config:set', patch),
  resetConfig: () => ipcRenderer.invoke('config:reset'),

  // 认证
  login: (url, password) => ipcRenderer.invoke('auth:login', { url, password }),
  logout: () => ipcRenderer.invoke('auth:logout'),
  session: () => ipcRenderer.invoke('auth:session'),

  // 通用请求
  request: (method, path, body, query) =>
    ipcRenderer.invoke('api:request', { method, path, body, query }),

  // 取高亮主题 CSS（服务端返回 chroma 配色规则）
  highlightCSS: (theme) => ipcRenderer.invoke('api:highlight-css', theme),

  // 剪贴板摘录
  clipClipboard: (category, title) => ipcRenderer.invoke('clip:clipboard', { category, title }),

  // 外部链接
  openExternal: (url) => ipcRenderer.invoke('shell:open', url),
  showItem: (p) => ipcRenderer.invoke('shell:showItem', p),

  // 窗口控制
  minimize: () => ipcRenderer.invoke('win:minimize'),
  maximize: () => ipcRenderer.invoke('win:maximize'),
  close: () => ipcRenderer.invoke('win:close'),
  isMaximized: () => ipcRenderer.invoke('win:isMaximized'),

  // 菜单事件
  onMenu: (channel, handler) => {
    const allowed = ['menu:new-note', 'menu:clip', 'menu:search', 'menu:settings'];
    if (!allowed.includes(channel)) return () => {};
    const listener = () => handler();
    ipcRenderer.on(channel, listener);
    return () => ipcRenderer.removeListener(channel, listener);
  },
});
