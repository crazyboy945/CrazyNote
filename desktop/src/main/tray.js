'use strict';

const { Tray, Menu, nativeImage } = require('electron');
const path = require('node:path');

const APP_DIR = path.join(__dirname, '..', '..');

/**
 * 系统托盘：常驻后台，提供快速摘录与新建入口。
 */
class AppTray {
  /** @param {{onShow:Function,onNewNote:Function,onClip:Function,onQuit:Function}} handlers */
  constructor(handlers) {
    this.handlers = handlers;
    this.tray = null;
  }

  create() {
    const iconPath = path.join(APP_DIR, 'build', 'tray.png');
    let image = nativeImage.createFromPath(iconPath);
    if (image.isEmpty()) {
      // 找不到托盘图标时用空图标占位，避免启动崩溃
      image = nativeImage.createEmpty();
    } else if (process.platform === 'darwin') {
      image = image.resize({ width: 16, height: 16 });
    }

    this.tray = new Tray(image);
    this.tray.setToolTip('CrazyNote');
    this.refreshMenu();
    this.tray.on('click', () => this.handlers.onShow());
    this.tray.on('double-click', () => this.handlers.onShow());
    return this.tray;
  }

  refreshMenu() {
    if (!this.tray) return;
    const menu = Menu.buildFromTemplate([
      { label: '显示主窗口', click: () => this.handlers.onShow() },
      { type: 'separator' },
      { label: '新建笔记', click: () => this.handlers.onNewNote() },
      { label: '摘录剪贴板', click: () => this.handlers.onClip() },
      { type: 'separator' },
      { label: '退出', click: () => this.handlers.onQuit() },
    ]);
    this.tray.setContextMenu(menu);
  }

  /** 更新托盘提示文字（如显示当前分类） */
  setStatus(text) {
    if (this.tray) this.tray.setToolTip(text ? `CrazyNote — ${text}` : 'CrazyNote');
  }

  destroy() {
    if (this.tray) {
      this.tray.destroy();
      this.tray = null;
    }
  }
}

module.exports = AppTray;
