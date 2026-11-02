// ASR 配置与模型安装
(function () {
  'use strict';

  const form = document.getElementById('asr-form');
  if (!form) return;

  const backendSel = document.getElementById('asr-form-backend');

  // --- 载入当前配置 ---
  fetch('/api/asr')
    .then(function (r) { return r.json(); })
    .then(function (d) {
      if (d.error) return;
      backendSel.value = d.backend || 'none';
      document.getElementById('asr-form-model-dir').value = d.model_dir || '';
      document.getElementById('asr-form-cli').value = d.cli_path || '';
      document.getElementById('asr-form-endpoint').value = d.endpoint || '';
      document.getElementById('asr-form-language').value = d.language || '';
      if (d.api_key_set) {
        document.getElementById('asr-form-api-key').placeholder = '已设置（留空不修改）';
      }
    })
    .catch(function () {});

  // --- 环境检测 ---
  fetch('/api/asr/environment')
    .then(function (r) { return r.json(); })
    .then(function (env) {
      const box = document.getElementById('env-check');
      const notes = document.getElementById('env-notes');
      const items = [
        (env.ffmpeg ? '✅' : '❌') + ' ffmpeg' + (env.ffmpeg_path ? '（' + env.ffmpeg_path + '）' : ''),
        (env.sherpa_cli ? '✅' : '❌') + ' sherpa CLI' + (env.sherpa_path ? '（' + env.sherpa_path + '）' : ''),
        (env.model_found ? '✅' : '❌') + ' 模型' + (env.model_path ? '（' + env.model_path + '）' : '')
      ];
      box.innerHTML = items.join('<br>');
      if (env.notes && env.notes.length) {
        notes.innerHTML = '⚠ ' + env.notes.join('<br>⚠ ');
      }
    })
    .catch(function () {
      document.getElementById('env-check').textContent = '检测失败';
    });

  // --- 模型列表 ---
  let pollTimer = null;

  fetch('/api/asr/models')
    .then(function (r) { return r.json(); })
    .then(function (d) {
      const list = document.getElementById('model-list');
      if (!d.models || !d.models.length) {
        list.textContent = '无可用模型';
        return;
      }
      list.innerHTML = d.models.map(function (m) {
        return '<div class="model-item">' +
          '<div class="model-info">' +
          '<div class="model-name">' + m.name + ' <span class="model-size">' + m.size + '</span></div>' +
          '<div class="model-desc">' + m.desc + '</div>' +
          '</div>' +
          '<button class="btn-primary model-install" data-model="' + m.id + '">下载</button>' +
          '</div>';
      }).join('');

      list.querySelectorAll('.model-install').forEach(function (btn) {
        btn.addEventListener('click', function () {
          startInstall(btn.dataset.model);
        });
      });
    })
    .catch(function () {
      document.getElementById('model-list').textContent = '加载失败';
    });

  async function startInstall(modelID) {
    const errBox = document.getElementById('install-error');
    errBox.hidden = true;
    document.getElementById('install-progress').hidden = false;
    document.getElementById('progress-text').textContent = '准备下载…';
    document.getElementById('progress-fill').style.width = '0%';

    try {
      const res = await fetch('/api/asr/install', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ model_id: modelID })
      });
      const d = await res.json();
      if (!res.ok) {
        errBox.textContent = d.error || '启动失败';
        errBox.hidden = false;
        document.getElementById('install-progress').hidden = true;
        return;
      }
      pollInstall();
    } catch (e) {
      errBox.textContent = '网络错误：' + e.message;
      errBox.hidden = false;
      document.getElementById('install-progress').hidden = true;
    }
  }

  async function pollInstall() {
    try {
      const res = await fetch('/api/asr/install/status');
      const st = await res.json();

      const fill = document.getElementById('progress-fill');
      const text = document.getElementById('progress-text');

      if (st.phase === 'downloading') {
        const pct = st.percent || 0;
        fill.style.width = pct.toFixed(1) + '%';
        const dl = (st.downloaded / 1024 / 1024).toFixed(1);
        const total = st.total > 0 ? (st.total / 1024 / 1024).toFixed(1) : '?';
        text.textContent = '下载中 ' + dl + ' / ' + total + ' MB（' + pct.toFixed(1) + '%）';
        pollTimer = setTimeout(pollInstall, 1000);
        return;
      }
      if (st.phase === 'extracting') {
        fill.style.width = '100%';
        text.textContent = '解压中…';
        pollTimer = setTimeout(pollInstall, 1000);
        return;
      }
      if (st.phase === 'done') {
        fill.style.width = '100%';
        text.textContent = '✅ 安装完成：' + (st.target_dir || '');
        setTimeout(function () { window.location.reload(); }, 2000);
        return;
      }
      if (st.phase === 'error') {
        const errBox = document.getElementById('install-error');
        errBox.textContent = st.error || '安装失败';
        errBox.hidden = false;
        text.textContent = '安装失败';
        return;
      }
      // 未在运行
      document.getElementById('install-progress').hidden = true;
    } catch (e) {
      pollTimer = setTimeout(pollInstall, 2000);
    }
  }

  const cancelBtn = document.getElementById('install-cancel');
  if (cancelBtn) {
    cancelBtn.addEventListener('click', function () {
      if (pollTimer) clearTimeout(pollTimer);
      document.getElementById('install-progress').hidden = true;
    });
  }

  // --- 保存配置 ---
  form.addEventListener('submit', async function (e) {
    e.preventDefault();
    const errBox = document.getElementById('asr-error');
    const okBox = document.getElementById('asr-success');
    errBox.hidden = true;
    okBox.hidden = true;

    const payload = {
      backend: backendSel.value,
      model_dir: document.getElementById('asr-form-model-dir').value.trim(),
      cli_path: document.getElementById('asr-form-cli').value.trim(),
      ffmpeg_path: document.getElementById('asr-form-ffmpeg').value.trim(),
      num_threads: parseInt(document.getElementById('asr-form-threads').value, 10) || 0,
      api_key: document.getElementById('asr-form-api-key').value.trim(),
      endpoint: document.getElementById('asr-form-endpoint').value.trim(),
      language: document.getElementById('asr-form-language').value.trim()
    };

    try {
      const res = await fetch('/api/asr', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const d = await res.json();
      if (res.ok) {
        okBox.textContent = '配置已保存';
        okBox.hidden = false;
        setTimeout(function () { window.location.reload(); }, 800);
      } else {
        errBox.textContent = d.error || '保存失败';
        errBox.hidden = false;
      }
    } catch (err) {
      errBox.textContent = '网络错误：' + err.message;
      errBox.hidden = false;
    }
  });
})();
