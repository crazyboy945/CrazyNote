'use strict';

/**
 * CrazyNote 服务端 API 客户端。
 *
 * 全部请求在主进程发起，渲染层通过 IPC 调用：
 *  - 规避浏览器同源策略（CORS）限制
 *  - Cookie 会话由主进程维护，渲染层不接触凭据
 */
class ApiClient {
  constructor() {
    /** @type {Map<string, string>} 每个服务地址对应一个 Cookie 串 */
    this.cookies = new Map();
  }

  _cookieHeader(baseUrl) {
    return this.cookies.get(baseUrl) || '';
  }

  _storeCookies(baseUrl, setCookie) {
    if (!setCookie) return;
    const values = Array.isArray(setCookie) ? setCookie : [setCookie];
    const jar = new Map();
    const existing = this.cookies.get(baseUrl);
    if (existing) {
      for (const part of existing.split('; ')) {
        const idx = part.indexOf('=');
        if (idx > 0) jar.set(part.slice(0, idx), part.slice(idx + 1));
      }
    }
    for (const v of values) {
      const first = v.split(';')[0];
      const idx = first.indexOf('=');
      if (idx <= 0) continue;
      const name = first.slice(0, idx).trim();
      const val = first.slice(idx + 1).trim();
      if (val === '' || /expires=Thu, 01 Jan 1970/i.test(v)) jar.delete(name);
      else jar.set(name, val);
    }
    if (jar.size === 0) this.cookies.delete(baseUrl);
    else this.cookies.set(baseUrl, [...jar].map(([k, v]) => `${k}=${v}`).join('; '));
  }

  /**
   * 发起请求。
   * @param {string} baseUrl 服务端地址
   * @param {string} method HTTP 方法
   * @param {string} apiPath 接口路径，如 /api/notes
   * @param {{body?:any, query?:object, timeout?:number}} opts
   */
  async request(baseUrl, method, apiPath, opts = {}) {
    const { body, query, timeout = 20000 } = opts;
    const base = (baseUrl || '').replace(/\/+$/, '');
    if (!base) return { ok: false, error: '未配置服务器地址' };

    let url = base + apiPath;
    if (query && Object.keys(query).length) {
      const qs = new URLSearchParams();
      for (const [k, v] of Object.entries(query)) {
        if (v !== undefined && v !== null && v !== '') qs.append(k, String(v));
      }
      const s = qs.toString();
      if (s) url += (url.includes('?') ? '&' : '?') + s;
    }

    const headers = { Accept: 'application/json' };
    const cookie = this._cookieHeader(base);
    if (cookie) headers.Cookie = cookie;

    // GET/HEAD 不允许带 body，Node 的 fetch 会直接报错
    const upper = String(method || 'GET').toUpperCase();
    const canHaveBody = upper !== 'GET' && upper !== 'HEAD';
    if (canHaveBody && body !== undefined) {
      headers['Content-Type'] = 'application/json';
    }

    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeout);

    try {
      const res = await fetch(url, {
        method: upper,
        headers,
        body: canHaveBody && body !== undefined ? JSON.stringify(body) : undefined,
        signal: controller.signal,
        redirect: 'follow',
      });

      this._storeCookies(base, res.headers.getSetCookie ? res.headers.getSetCookie() : null);

      const text = await res.text();
      let data = null;
      if (text) {
        try {
          data = JSON.parse(text);
        } catch {
          data = { raw: text };
        }
      }

      if (!res.ok) {
        return {
          ok: false,
          status: res.status,
          error: (data && (data.error || data.message)) || `HTTP ${res.status}`,
          data,
        };
      }
      return { ok: true, status: res.status, data };
    } catch (e) {
      let msg = e.message || '连接失败';
      if (e.name === 'AbortError') msg = '请求超时';
      else if (/fetch failed/i.test(msg)) msg = '无法连接到服务器，请检查地址与网络';
      return { ok: false, status: 0, error: msg };
    } finally {
      clearTimeout(timer);
    }
  }

  /** 登录并保存会话 Cookie */
  async login(url, password) {
    const res = await this.request(url, 'POST', '/api/login', { body: { password } });
    if (res.ok) {
      // 登录成功后再确认一次身份
      const sess = await this.request(url, 'GET', '/api/session');
      if (sess.ok && sess.data) {
        return { ok: true, user: sess.data.user || 'admin', isAdmin: !!sess.data.is_admin, mode: sess.data.mode };
      }
      return { ok: true, user: 'admin', isAdmin: true };
    }
    return { ok: false, error: res.error || '登录失败' };
  }

  async logout() {
    for (const base of [...this.cookies.keys()]) {
      await this.request(base, 'POST', '/api/logout').catch(() => {});
    }
    this.cookies.clear();
    return { ok: true };
  }

  async getSession(url) {
    const res = await this.request(url, 'GET', '/api/session');
    if (!res.ok) return { ok: false, error: res.error, status: res.status };
    return { ok: true, ...(res.data || {}) };
  }
}

module.exports = ApiClient;
