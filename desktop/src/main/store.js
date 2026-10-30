'use strict';

const { app, safeStorage } = require('electron');
const fs = require('node:fs');
const path = require('node:path');

/**
 * 配置存储：普通配置明文保存，密码用 Electron safeStorage 加密。
 */
class Store {
  /** @param {string} file */
  constructor(file) {
    this.file = file;
    this.data = {
      server: { url: '', username: '' },
      window: { width: 1280, height: 820, x: undefined, y: undefined, maximized: false },
      lastCategory: '',
      theme: 'light',
    };
    this.load();
  }

  load() {
    try {
      if (fs.existsSync(this.file)) {
        const raw = JSON.parse(fs.readFileSync(this.file, 'utf8'));
        this.data = { ...this.data, ...raw, window: { ...this.data.window, ...(raw.window || {}) } };
      }
    } catch (e) {
      console.error('读取配置失败:', e.message);
    }
    this._session = null;
    // 会话信息单独存，密码加密
    const sessFile = this.file + '.session';
    try {
      if (fs.existsSync(sessFile)) {
        const raw = JSON.parse(fs.readFileSync(sessFile, 'utf8'));
        this._session = {
          url: raw.url || '',
          password: this._decrypt(raw.password),
        };
      }
    } catch (e) {
      console.error('读取会话失败:', e.message);
    }
  }

  save() {
    try {
      fs.mkdirSync(path.dirname(this.file), { recursive: true });
      fs.writeFileSync(this.file, JSON.stringify(this.data, null, 2), 'utf8');
    } catch (e) {
      console.error('保存配置失败:', e.message);
    }
  }

  _encrypt(plain) {
    if (!plain) return '';
    try {
      if (safeStorage.isEncryptionAvailable()) {
        return 'enc:' + safeStorage.encryptString(plain).toString('base64');
      }
    } catch (e) {
      console.error('加密失败:', e.message);
    }
    return 'plain:' + Buffer.from(plain, 'utf8').toString('base64');
  }

  _decrypt(raw) {
    if (!raw) return '';
    try {
      if (raw.startsWith('enc:')) {
        if (!safeStorage.isEncryptionAvailable()) return '';
        return safeStorage.decryptString(Buffer.from(raw.slice(4), 'base64'));
      }
      if (raw.startsWith('plain:')) {
        return Buffer.from(raw.slice(6), 'base64').toString('utf8');
      }
    } catch (e) {
      console.error('解密失败:', e.message);
    }
    return '';
  }

  /** 返回给渲染层的配置（不含密码） */
  getPublicConfig() {
    return {
      server: { ...this.data.server },
      window: { ...this.data.window },
      lastCategory: this.data.lastCategory,
      theme: this.data.theme,
      connected: !!(this._session && this._session.url),
      currentUrl: this._session ? this._session.url : '',
    };
  }

  setConfig(patch = {}) {
    if (patch.server) this.data.server = { ...this.data.server, ...patch.server };
    if (patch.window) this.data.window = { ...this.data.window, ...patch.window };
    if (patch.lastCategory !== undefined) this.data.lastCategory = patch.lastCategory;
    if (patch.theme) this.data.theme = patch.theme;
    this.save();
    return this.getPublicConfig();
  }

  getSession() {
    return this._session;
  }

  setSession({ url, password }) {
    this._session = { url: (url || '').replace(/\/+$/, ''), password: password || '' };
    const sessFile = this.file + '.session';
    try {
      fs.mkdirSync(path.dirname(sessFile), { recursive: true });
      fs.writeFileSync(
        sessFile,
        JSON.stringify({ url: this._session.url, password: this._encrypt(this._session.password) }, null, 2),
        'utf8'
      );
    } catch (e) {
      console.error('保存会话失败:', e.message);
    }
    this.data.server.url = this._session.url;
    this.save();
  }

  clearSession() {
    this._session = null;
    const sessFile = this.file + '.session';
    try {
      if (fs.existsSync(sessFile)) fs.unlinkSync(sessFile);
    } catch (e) {
      console.error('清除会话失败:', e.message);
    }
  }

  reset() {
    this.data.server = { url: '', username: '' };
    this.data.lastCategory = '';
    this.clearSession();
    this.save();
    return this.getPublicConfig();
  }
}

module.exports = Store;
