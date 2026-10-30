/*
 * content.js — 在页面上下文中执行 Readability 提取正文。
 *
 * 注意：content script 运行在隔离世界，无法直接读取页面的 window.getSelection()，
 * 因此选区文本由 popup 通过 chrome.scripting.executeScript 单独注入函数获取。
 */
(function () {
  'use strict';

  // 暴露一个供 popup 调用的提取函数（挂在 window 上以便可被 scripting API 访问）
  window.__crazyNoteExtract = function () {
    try {
      if (typeof Readability === 'undefined') {
        return { error: 'Readability 未加载' };
      }
      const clone = document.cloneNode(true);
      const article = new Readability(clone, {
        charThreshold: 100,
        keepClasses: false,
      }).parse();
      if (!article) {
        return { error: '未能提取正文' };
      }
      return {
        title: article.title || document.title,
        byline: article.byline || '',
        excerpt: article.excerpt || '',
        content: article.content || '',
        textContent: article.textContent || '',
        length: article.length || 0,
        url: location.href,
        siteName: article.siteName || location.hostname,
      };
    } catch (e) {
      return { error: '提取失败: ' + e.message };
    }
  };

  // 获取用户当前选中的文本
  window.__crazyNoteGetSelection = function () {
    try {
      const sel = window.getSelection();
      return sel ? String(sel).trim() : '';
    } catch (e) {
      return '';
    }
  };
})();
