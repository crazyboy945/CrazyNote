'use strict';

const { Menu, shell } = require('electron');

/**
 * 应用菜单（Windows / Linux）。
 */
function build(handlers) {
  const isMac = process.platform === 'darwin';

  const template = [
    ...(isMac
      ? [
          {
            label: 'CrazyNote',
            submenu: [
              { role: 'about', label: '关于 CrazyNote' },
              { type: 'separator' },
              { role: 'hide', label: '隐藏' },
              { role: 'hideOthers', label: '隐藏其他' },
              { role: 'unhide', label: '全部显示' },
              { type: 'separator' },
              { role: 'quit', label: '退出' },
            ],
          },
        ]
      : []),
    {
      label: '文件',
      submenu: [
        { label: '新建笔记', accelerator: 'CmdOrCtrl+N', click: () => handlers.onNewNote() },
        { label: '摘录剪贴板', accelerator: 'CmdOrCtrl+Shift+V', click: () => handlers.onClip() },
        { type: 'separator' },
        ...(isMac ? [] : [{ label: '设置', accelerator: 'CmdOrCtrl+,', click: () => handlers.onSettings() }]),
        { type: 'separator' },
        isMac ? { role: 'close', label: '关闭窗口' } : { role: 'quit', label: '退出' },
      ],
    },
    {
      label: '编辑',
      submenu: [
        { role: 'undo', label: '撤销' },
        { role: 'redo', label: '重做' },
        { type: 'separator' },
        { role: 'cut', label: '剪切' },
        { role: 'copy', label: '复制' },
        { role: 'paste', label: '粘贴' },
        { role: 'selectAll', label: '全选' },
        { type: 'separator' },
        { label: '搜索', accelerator: 'CmdOrCtrl+F', click: () => handlers.onSearch() },
      ],
    },
    {
      label: '视图',
      submenu: [
        { role: 'reload', label: '重新加载' },
        { role: 'forceReload', label: '强制刷新' },
        { type: 'separator' },
        { role: 'resetZoom', label: '实际大小' },
        { role: 'zoomIn', label: '放大' },
        { role: 'zoomOut', label: '缩小' },
        { type: 'separator' },
        { role: 'togglefullscreen', label: '全屏' },
        ...(process.argv.includes('--dev')
          ? [{ type: 'separator' }, { label: '开发者工具', accelerator: 'F12', click: () => handlers.onToggleDevTools() }]
          : []),
      ],
    },
    {
      label: '帮助',
      submenu: [
        {
          label: '项目主页',
          click: () => shell.openExternal('https://github.com/local/crazynote'),
        },
      ],
    },
  ];

  Menu.setApplicationMenu(Menu.buildFromTemplate(template));
}

module.exports = { build };
