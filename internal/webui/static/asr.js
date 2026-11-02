// ASR 页面：环境检测 / 依赖安装 / 模型下载 / 配置
(function () {
  'use strict';

  const form = document.getElementById('asr-form');
  if (!form) return;

  const backendSel = document.getElementById('asr-backend');
  const backendBadge = document.getElementById('cfg-backend-badge');

  function $(id) { return document.getElementById(id); }

  function fmtBytes(n) {
    if (!n) return '0 B';
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
    if (n < 1024 * 1024 * 1024) return (n / 1024 / 1024).toFixed(1) + ' MB';
    return (n / 1024 / 1024 / 1024).toFixed(2) + ' GB';
  }

  function escapeHTML(s) {
    const div = document.createElement('div');
    div.textContent = s == null ? '' : String(s);
    return div.innerHTML;
  }

  // ===== 1. 环境检测 =====
  async function checkEnv() {
    try {
      const res = await fetch('/api/asr/environment');
      const env = await res.json();

      setStatus('ffmpeg', env.ffmpeg, env.ffmpeg_path, env.ffmpeg ? null : 'btn-install-ffmpeg');
      setStatus('sherpa', env.sherpa_cli, env.sherpa_path, env.sherpa_cli ? null : 'btn-install-sherpa');
      setStatus('model', env.model_found, env.model_path, null);
    } catch (e) {
      const el = $('st-ffmpeg');
      if (el) el.textContent = '检测失败';
    }
  }

  function setStatus(key, ok, path, showBtnID) {
    const card = document.querySelector('.status-card[data-dep="' + key + '"]');
    const val = $('st-' + key);
    if (!card || !val) return;

    card.classList.toggle('ok', !!ok);
    card.classList.toggle('missing', !ok);

    if (ok) {
      let short = path || '已安装';
      if (path) {
        const parts = path.split(/[\\/]/);
        short = parts[parts.length - 1];
        val.title = path;
      }
      val.textContent = '✓ ' + short;
    } else {
      val.textContent = '未找到';
      val.title = '';
    }

    if (showBtnID) {
      const btn = $(showBtnID);
      if (btn) btn.hidden = false;
    }
  }

  // ===== 2. 依赖安装 =====
  let pollTimer = null;

  function bindInstallBtn(id, kind) {
    const btn = $(id);
    if (!btn) return;
    btn.addEventListener('click', async function () {
      btn.disabled = true;
      try {
        const res = await fetch('/api/asr/deps/install', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ kind: kind })
        });
        const d = await res.json();
        if (!res.ok) {
          alert(d.error || '启动安装失败');
          btn.disabled = false;
          return;
        }
        showInstallPanel(kind === 'ffmpeg' ? '安装 FFmpeg' : '安装 Sherpa CLI');
        pollDeps();
      } catch (e) {
        alert('网络错误：' + e.message);
        btn.disabled = false;
      }
    });
  }

  bindInstallBtn('btn-install-ffmpeg', 'ffmpeg');
  bindInstallBtn('btn-install-sherpa', 'sherpa');

  function showInstallPanel(title) {
    const panel = $('install-panel');
    if (!panel) return;
    panel.hidden = false;
    panel.classList.remove('install-error');
    $('install-title').textContent = title;
    $('install-pct').textContent = '0%';
    $('install-fill').style.width = '0%';
    $('install-steps').innerHTML = '';
  }

  async function pollDeps() {
    try {
      const res = await fetch('/api/asr/deps/status');
      const st = await res.json();

      const pct = st.percent || 0;
      $('install-fill').style.width = pct.toFixed(1) + '%';
      $('install-pct').textContent = pct.toFixed(1) + '%';

      if (st.phase === 'downloading') {
        $('install-title').textContent =
          '下载中 ' + fmtBytes(st.downloaded) + ' / ' + (st.total > 0 ? fmtBytes(st.total) : '?');
      } else if (st.phase === 'extracting') {
        $('install-title').textContent = '解压安装中…';
      } else if (st.phase === 'checking') {
        $('install-title').textContent = '检查中…';
      }

      if (st.steps && st.steps.length) {
        const box = $('install-steps');
        if (box.childElementCount !== st.steps.length) {
          box.innerHTML = st.steps.map(function (s) {
            return '<div>› ' + escapeHTML(s) + '</div>';
          }).join('');
          box.scrollTop = box.scrollHeight;
        }
      }

      if (st.phase === 'done') {
        $('install-title').textContent = '✅ ' + (st.message || '安装完成');
        $('install-fill').style.width = '100%';
        $('install-pct').textContent = '100%';
        setTimeout(function () {
          $('install-panel').hidden = true;
          resetButtons();
          checkEnv();
        }, 1800);
        return;
      }

      if (st.phase === 'error' || st.phase === 'unsupported') {
        $('install-title').textContent = '❌ ' + (st.error || '安装失败');
        $('install-panel').classList.add('install-error');
        resetButtons();
        return;
      }

      pollTimer = setTimeout(pollDeps, 800);
    } catch (e) {
      pollTimer = setTimeout(pollDeps, 2000);
    }
  }

  function resetButtons() {
    document.querySelectorAll('.btn-sm').forEach(function (b) { b.disabled = false; });
  }

  const closeBtn = $('install-close');
  if (closeBtn) {
    closeBtn.addEventListener('click', function () {
      if (pollTimer) clearTimeout(pollTimer);
      $('install-panel').hidden = true;
      resetButtons();
    });
  }

  // ===== 3. 模型列表 =====
  async function loadModels() {
    try {
      const res = await fetch('/api/asr/models');
      const d = await res.json();
      const list = $('model-list');
      if (!list) return;
      if (!d.models || !d.models.length) {
        list.textContent = '暂无可用模型';
        return;
      }
      list.innerHTML = d.models.map(function (m, i) {
        const tag = i === 0 ? '<span class="model-tag">推荐</span>' : '';
        return '<div class="model-card">' +
          '<div class="model-main">' +
            '<div class="model-title">' + escapeHTML(m.name) + tag + '</div>' +
            '<div class="model-meta">' + escapeHTML(m.desc) + ' · ' + escapeHTML(m.size) + '</div>' +
          '</div>' +
          '<button class="btn-primary btn-sm model-dl" data-model="' + escapeHTML(m.id) + '">下载</button>' +
          '</div>';
      }).join('');

      list.querySelectorAll('.model-dl').forEach(function (btn) {
        btn.addEventListener('click', function () {
          startModelDownload(btn.dataset.model, btn);
        });
      });
    } catch (e) {
      const list = $('model-list');
      if (list) list.textContent = '加载失败：' + e.message;
    }
  }

  async function startModelDownload(modelID, btn) {
    btn.disabled = true;
    try {
      const res = await fetch('/api/asr/install', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ model_id: modelID })
      });
      const d = await res.json();
      if (!res.ok) {
        alert(d.error || '启动下载失败');
        btn.disabled = false;
        return;
      }
      showInstallPanel('下载模型 ' + modelID);
      pollModelInstall();
    } catch (e) {
      alert('网络错误：' + e.message);
      btn.disabled = false;
    }
  }

  async function pollModelInstall() {
    try {
      const res = await fetch('/api/asr/install/status');
      const st = await res.json();
      const pct = st.percent || 0;

      $('install-fill').style.width = pct.toFixed(1) + '%';
      $('install-pct').textContent = pct.toFixed(1) + '%';

      if (st.phase === 'downloading') {
        $('install-title').textContent =
          '下载模型 ' + fmtBytes(st.downloaded) + ' / ' + (st.total > 0 ? fmtBytes(st.total) : '?');
      } else if (st.phase === 'extracting') {
        $('install-title').textContent = '解压模型…';
      }

      if (st.phase === 'done') {
        $('install-title').textContent = '✅ 模型安装完成';
        $('install-fill').style.width = '100%';
        setTimeout(function () {
          $('install-panel').hidden = true;
          window.location.reload();
        }, 1500);
        return;
      }
      if (st.phase === 'error') {
        $('install-title').textContent = '❌ ' + (st.error || '下载失败');
        document.querySelectorAll('.model-dl').forEach(function (b) { b.disabled = false; });
        return;
      }
      setTimeout(pollModelInstall, 800);
    } catch (e) {
      setTimeout(pollModelInstall, 2000);
    }
  }

  // ===== 4. 配置表单 =====
  function applyBackendVisibility() {
    const v = backendSel.value;
    backendBadge.textContent = v;
    document.querySelectorAll('[data-show-when]').forEach(function (el) {
      el.hidden = el.dataset.showWhen !== v;
    });
  }

  backendSel.addEventListener('change', applyBackendVisibility);

  async function loadConfig() {
    try {
      const res = await fetch('/api/asr');
      const d = await res.json();
      if (d.error) return;

      backendSel.value = d.backend || 'none';
      $('asr-model-dir').value = d.model_dir || '';
      $('asr-cli').value = d.cli_path || '';
      $('asr-ffmpeg').value = d.ffmpeg_path || '';
      $('asr-threads').value = d.num_threads || '';
      $('asr-endpoint').value = d.endpoint || '';
      $('asr-language').value = d.language || '';
      if (d.api_key_set) {
        $('asr-api-key').placeholder = '已设置（留空不修改）';
      }
      applyBackendVisibility();
    } catch (e) { /* 忽略 */ }
  }

  form.addEventListener('submit', async function (e) {
    e.preventDefault();
    const msg = $('form-msg');
    msg.className = 'form-msg';
    msg.textContent = '保存中…';

    const payload = {
      backend: backendSel.value,
      model_dir: $('asr-model-dir').value.trim(),
      cli_path: $('asr-cli').value.trim(),
      ffmpeg_path: $('asr-ffmpeg').value.trim(),
      num_threads: parseInt($('asr-threads').value, 10) || 0,
      api_key: $('asr-api-key').value.trim(),
      endpoint: $('asr-endpoint').value.trim(),
      language: $('asr-language').value.trim()
    };

    try {
      const res = await fetch('/api/asr', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const d = await res.json();
      if (res.ok) {
        msg.className = 'form-msg ok';
        msg.textContent = '✓ 已保存';
        checkEnv();
      } else {
        msg.className = 'form-msg err';
        msg.textContent = '✗ ' + (d.error || '保存失败');
      }
    } catch (err) {
      msg.className = 'form-msg err';
      msg.textContent = '✗ 网络错误：' + err.message;
    }
  });

  // ===== 启动 =====
  checkEnv();
  loadModels();
  loadConfig();
})();
