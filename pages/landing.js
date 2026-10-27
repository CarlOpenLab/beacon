/**
 * term-fleet · landing v2
 * Everything here is progressive enhancement: the page reads fine without it.
 */
(function () {
  'use strict';

  var reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
  var isEN = false;
  var langHooks = [];

  function onLang(fn) { langHooks.push(fn); }

  /* ========================================================================
     Language
     ======================================================================== */

  var langNodes = Array.prototype.slice.call(document.querySelectorAll('[data-en]'));
  langNodes.forEach(function (node) {
    if (node.dataset.zh === undefined) node.dataset.zh = node.innerHTML;
  });

  // aria-labels can't ride the innerHTML swap, so they get their own pair.
  var ariaNodes = Array.prototype.slice.call(document.querySelectorAll('[data-en-aria]'));
  ariaNodes.forEach(function (node) {
    if (node.dataset.zhAria === undefined) node.dataset.zhAria = node.getAttribute('aria-label') || '';
  });

  var titles = { zh: document.title, en: 'Beacon — one terminal fleet, controlled from anywhere' };
  var langBtn = document.getElementById('lang-toggle');

  function applyLang(lang) {
    isEN = lang === 'en';
    langNodes.forEach(function (node) {
      node.innerHTML = isEN ? node.dataset.en : node.dataset.zh;
    });
    ariaNodes.forEach(function (node) {
      node.setAttribute('aria-label', isEN ? node.dataset.enAria : node.dataset.zhAria);
    });
    document.title = isEN ? titles.en : titles.zh;
    document.documentElement.lang = isEN ? 'en' : 'zh-CN';
    if (langBtn) {
      langBtn.textContent = isEN ? '中' : 'EN';
      langBtn.setAttribute('aria-label', isEN ? '切换到中文' : 'Switch to English');
    }
    try { localStorage.setItem('tf-lang-v2', lang); } catch (err) { /* private mode */ }
    langHooks.forEach(function (fn) { fn(); });
  }

  if (langBtn) {
    langBtn.addEventListener('click', function () {
      applyLang(isEN ? 'zh' : 'en');
    });
  }

  var storedLang = null;
  try { storedLang = localStorage.getItem('tf-lang-v2'); } catch (err) { /* noop */ }
  if (storedLang === 'en') applyLang('en');

  /* ========================================================================
     Product stage — three clients, one session
     ======================================================================== */

  var canvas = document.getElementById('stage-canvas');
  var cap = document.getElementById('stage-cap');
  var tabs = Array.prototype.slice.call(document.querySelectorAll('.sw-btn'));
  var mocks = Array.prototype.slice.call(document.querySelectorAll('.mock'));

  var currentPanel = 'panel-desktop';

  function fitStage() {
    if (!canvas) return;
    var pad = window.innerWidth < 760 ? 16 : 36;
    var available = canvas.clientWidth - pad * 2;
    if (available <= 0) return;

    var active = null;

    mocks.forEach(function (mock) {
      var naturalW = Number(mock.dataset.w) || 1000;
      var scale = Math.min(1, available / naturalW);
      mock.style.transform = 'translate(-50%, -50%) scale(' + scale + ')';
      if (mock.classList.contains('is-active')) {
        active = { mock: mock, height: Number(mock.dataset.h) || 540, scale: scale };
      }
    });

    if (active) {
      canvas.style.height = Math.round(active.height * active.scale + pad * 2) + 'px';
    }
  }

  function selectPanel(id, focusTab) {
    if (!id) return;
    currentPanel = id;

    mocks.forEach(function (mock) {
      var on = mock.id === id;
      mock.classList.toggle('is-active', on);
      mock.removeAttribute('hidden');
      mock.setAttribute('aria-hidden', on ? 'false' : 'true');
    });

    tabs.forEach(function (tab) {
      var on = tab.dataset.target === id;
      tab.setAttribute('aria-selected', on ? 'true' : 'false');
      tab.tabIndex = on ? 0 : -1;
      if (on && focusTab) tab.focus();
    });

    if (canvas) canvas.dataset.active = id;
    if (cap) cap.dataset.active = id;

    fitStage();
  }

  tabs.forEach(function (tab, index) {
    tab.addEventListener('click', function () {
      selectPanel(tab.dataset.target, false);
    });

    tab.addEventListener('keydown', function (event) {
      var next = null;
      if (event.key === 'ArrowRight') next = (index + 1) % tabs.length;
      if (event.key === 'ArrowLeft') next = (index - 1 + tabs.length) % tabs.length;
      if (event.key === 'Home') next = 0;
      if (event.key === 'End') next = tabs.length - 1;
      if (next === null) return;
      event.preventDefault();
      selectPanel(tabs[next].dataset.target, true);
    });
  });

  // A phone at phone width; the desktop on everything else. Visitors can switch.
  var initialPanel = window.innerWidth < 760 ? 'panel-phone' : 'panel-desktop';
  selectPanel(initialPanel, false);
  fitStage();

  var resizeTimer = null;
  window.addEventListener('resize', function () {
    window.clearTimeout(resizeTimer);
    resizeTimer = window.setTimeout(fitStage, 120);
  });

  if (document.fonts && document.fonts.ready) {
    document.fonts.ready.then(fitStage).catch(function () { /* noop */ });
  }
  window.addEventListener('load', fitStage);

  /* ========================================================================
     Demo A — detach, then resume at a byte offset
     ======================================================================== */

  (function () {
    var term = document.getElementById('drop-term');
    var btn = document.getElementById('drop-btn');
    var status = document.getElementById('drop-status');
    var meta = document.getElementById('drop-meta');
    var dot = document.getElementById('drop-dot');
    var body = document.getElementById('drop-body');
    var note = document.getElementById('drop-note');
    if (!term || !btn) return;

    var timers = [];
    var phase = 'live';

    var copy = {
      live: {
        zh: ['已连接 · s10', '拔掉网线', 'byte 41,208', '客户端离开 · shell 留下'],
        en: ['connected · s10', 'Pull the plug', 'byte 41,208', 'client detaches · shell stays']
      },
      off: {
        zh: ['已断开 · 客户端 detach', '重连中…', 'byte 41,208 · 已暂停', '客户端离开 · shell 留下'],
        en: ['detached · client left', 'reconnecting…', 'byte 41,208 · paused', 'the client left · the shell stays']
      },
      wait: {
        zh: ['重连中… 从 byte 41,208', '重连中…', 'byte 41,208 → 41,245', '客户端离开 · shell 留下'],
        en: ['reconnecting… from byte 41,208', 'reconnecting…', 'byte 41,208 → 41,245', 'the client left · the shell stays']
      },
      resumed: {
        zh: ['已续接 · 只补了 37 字节', '再来一次', 'byte 41,245', '只补了断线期间的那 37 个字节'],
        en: ['resumed · 37 bytes only', 'Run it again', 'byte 41,245', 'only the 37 missed bytes were delivered']
      }
    };

    function render() {
      var c = copy[phase][isEN ? 'en' : 'zh'];
      status.textContent = c[0];
      btn.textContent = c[1];
      meta.textContent = c[2];
      if (note) note.textContent = c[3];

      var busy = phase === 'off' || phase === 'wait';
      btn.disabled = busy;
      term.classList.toggle('is-off', busy);
      dot.className = 'dot ' + (phase === 'off' ? 'is-off' : phase === 'wait' ? 'is-wait' : 'mint');
      if (busy) term.setAttribute('aria-busy', 'true');
      else term.removeAttribute('aria-busy');
    }

    function clearTimers() {
      timers.forEach(window.clearTimeout);
      timers = [];
    }

    function later(fn, ms) {
      timers.push(window.setTimeout(fn, reduceMotion.matches ? Math.min(ms, 140) : ms));
    }

    btn.addEventListener('click', function () {
      if (phase === 'off' || phase === 'wait') return;

      clearTimers();
      body.querySelectorAll('.is-new').forEach(function (line) { line.remove(); });
      phase = 'off';
      render();

      later(function () {
        phase = 'wait';
        render();
      }, 700);

      later(function () {
        ['14:05:02  build    ✓  38s', '14:06:02  deploy   ✓  200'].forEach(function (text) {
          var line = document.createElement('div');
          line.className = 'tline out is-new';
          line.textContent = text;
          body.appendChild(line);
        });
        phase = 'resumed';
        render();
      }, 1700);
    });

    render();
    onLang(render);
  })();

  /* ========================================================================
     Demo B — the lease is explicit
     ======================================================================== */

  (function () {
    var btn = document.getElementById('lease-btn');
    var banner = document.getElementById('lease-banner');
    var mac = document.getElementById('lease-mac');
    var phone = document.getElementById('lease-phone');
    var roleMac = document.getElementById('role-mac');
    var rolePhone = document.getElementById('role-phone');
    var typed = document.getElementById('lease-typed');
    var note = document.getElementById('lease-note');
    if (!btn || !mac || !phone) return;

    var epoch = 6;
    var owner = 'mac';

    function render() {
      var macOwns = owner === 'mac';
      var host = macOwns ? 'studio-mac' : 'iPhone';
      var canType = isEN ? 'controlling · can type' : '控制中 · 可输入';
      var mirror = isEN ? 'read-only · mirroring' : '只读 · 实时镜像';

      mac.classList.toggle('is-controller', macOwns);
      phone.classList.toggle('is-controller', !macOwns);
      roleMac.textContent = macOwns ? canType : mirror;
      rolePhone.textContent = macOwns ? mirror : canType;
      banner.textContent = (isEN ? 'controlling' : '控制中') + ' (Epoch #' + epoch + ') · ' + host;
      typed.textContent = macOwns ? 'echo "lease is explicit"' : 'echo "typed on iPhone"';
      btn.textContent = macOwns
        ? (isEN ? 'Take over on iPhone' : '在手机上接管')
        : (isEN ? 'Hand back to desktop' : '交还给桌面');
      if (note) {
        note.textContent = isEN
          ? 'Takeover bumps the epoch; the previous controller turns read-only.'
          : '接管会递增纪元，旧控制端转为只读。';
      }

      phone.setAttribute('aria-label', isEN
        ? (macOwns ? 'iPhone, read-only' : 'iPhone, controlling')
        : (macOwns ? 'iPhone，只读' : 'iPhone，控制中'));
      mac.setAttribute('aria-label', isEN
        ? (macOwns ? 'studio-mac, controlling' : 'studio-mac, read-only')
        : (macOwns ? 'studio-mac，控制中' : 'studio-mac，只读'));
    }

    btn.addEventListener('click', function () {
      owner = owner === 'mac' ? 'phone' : 'mac';
      epoch += 1;
      render();
    });

    render();
    onLang(render);
  })();

  /* ========================================================================
     Quickstart tabs + copy
     ======================================================================== */

  (function () {
    var codeTabs = Array.prototype.slice.call(document.querySelectorAll('.code-tabs button'));
    var panes = {
      local: document.getElementById('code-local'),
      relay: document.getElementById('code-relay')
    };
    var copyBtn = document.getElementById('copy-btn');
    var copyTimer = null;

    function selectPane(name, focus) {
      codeTabs.forEach(function (tab) {
        var on = tab.dataset.code === name;
        tab.setAttribute('aria-selected', on ? 'true' : 'false');
        tab.tabIndex = on ? 0 : -1;
        if (on && focus) tab.focus();
      });
      Object.keys(panes).forEach(function (key) {
        if (panes[key]) panes[key].hidden = key !== name;
      });
    }

    codeTabs.forEach(function (tab, index) {
      tab.addEventListener('click', function () { selectPane(tab.dataset.code, false); });
      tab.addEventListener('keydown', function (event) {
        if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') return;
        event.preventDefault();
        var next = event.key === 'ArrowRight'
          ? (index + 1) % codeTabs.length
          : (index - 1 + codeTabs.length) % codeTabs.length;
        selectPane(codeTabs[next].dataset.code, true);
      });
    });

    if (!copyBtn) return;
    var idleLabel = function () { return isEN ? 'Copy' : '复制'; };

    copyBtn.addEventListener('click', function () {
      var visible = codeTabs.filter(function (t) { return t.getAttribute('aria-selected') === 'true'; })[0];
      var pane = visible ? panes[visible.dataset.code] : panes.local;
      if (!pane) return;

      var text = pane.textContent;

      function done() {
        copyBtn.textContent = isEN ? 'Copied' : '已复制';
        window.clearTimeout(copyTimer);
        copyTimer = window.setTimeout(function () {
          copyBtn.textContent = idleLabel();
        }, 1600);
      }

      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done, fallback);
      } else {
        fallback();
      }

      function fallback() {
        var area = document.createElement('textarea');
        area.value = text;
        area.setAttribute('readonly', '');
        area.style.position = 'fixed';
        area.style.opacity = '0';
        document.body.appendChild(area);
        area.select();
        try { document.execCommand('copy'); done(); } catch (err) { /* noop */ }
        document.body.removeChild(area);
      }
    });

    // Keep the idle label in sync when the language flips.
    onLang(function () {
      copyBtn.textContent = idleLabel();
    });
  })();

  /* ========================================================================
     Section spy — the rail and the header nav agree on where the reader is
     ======================================================================== */

  (function () {
    var rail = Array.prototype.slice.call(document.querySelectorAll('.side-link'));
    var top = Array.prototype.slice.call(document.querySelectorAll('.nav a'));
    if (!rail.length || !('IntersectionObserver' in window)) return;

    var sections = rail.map(function (l) { return document.querySelector(l.getAttribute('href')); });
    if (sections.some(function (s) { return !s; })) return;
    var current = -1;

    function mark(index) {
      if (index === current) return;
      current = index;
      rail.forEach(function (l, i) { l.classList.toggle('is-active', i === index); });
      top.forEach(function (l, i) { l.classList.toggle('is-active', i === index); });
    }

    // The page's upper third is where a reader is judged to be: the section
    // crossing that band is the one the rail names, and the band's narrowness
    // keeps two chapters from claiming the frame at once.
    var spy = new IntersectionObserver(function (entries) {
      entries.forEach(function (entry) {
        var i = sections.indexOf(entry.target);
        if (entry.isIntersecting) mark(i);
        else if (i === current) mark(-1);
      });
    }, { rootMargin: '-30% 0px -58% 0px' });
    sections.forEach(function (s) { spy.observe(s); });
  })();

})();
