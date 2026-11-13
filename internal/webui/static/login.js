// 账号登录（凭据配置 + 扫码 + 手机号）
(function () {
  'use strict';

  const credForm = document.getElementById('cred-form');
  if (!credForm) return;

  // --- Tab 切换 ---
  document.querySelectorAll('.tab').forEach(function (tab) {
    tab.addEventListener('click', function () {
      document.querySelectorAll('.tab').forEach(function (t) { t.classList.remove('active'); });
      document.querySelectorAll('.pane').forEach(function (p) { p.hidden = true; });
      tab.classList.add('active');
      const pane = document.getElementById('pane-' + tab.dataset.tab);
      if (pane) pane.hidden = false;
    });
  });

  // --- 凭据保存 ---
  credForm.addEventListener('submit', async function (e) {
    e.preventDefault();
    const errBox = document.getElementById('cred-error');
    const okBox = document.getElementById('cred-success');
    errBox.hidden = true;
    okBox.hidden = true;

    const payload = {
      account: document.getElementById('cred-account').value.trim(),
      api_id: parseInt(document.getElementById('cred-api-id').value, 10) || 0,
      api_hash: document.getElementById('cred-api-hash').value.trim()
    };

    try {
      const res = await fetch('/api/credentials', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const d = await res.json();
      if (res.ok) {
        const source = d.env_override ? '环境变量覆盖' : (d.is_builtin ? '内置官方凭据' : '自定义凭据');
        okBox.textContent = '已保存（账号：' + d.account + '，当前使用' + source + '，api_id=' + d.api_id + '）';
        okBox.hidden = false;
        loadCredentials();
      } else {
        errBox.textContent = d.error || '保存失败';
        errBox.hidden = false;
      }
    } catch (err) {
      errBox.textContent = '网络错误：' + err.message;
      errBox.hidden = false;
    }
  });

  // --- 载入已保存的凭据 ---
  async function loadCredentials() {
    try {
      const res = await fetch('/api/credentials');
      const d = await res.json();
      const status = document.getElementById('cred-status');
      const prefix = d.env_override
        ? '✅ 当前由环境变量覆盖（api_id=' + d.effective_api_id + '）；'
        : '✅ 已公开内置默认凭据（api_id=' + d.builtin_api_id + '），无需申请；';
      if (!res.ok || !d.accounts || !d.accounts.length) {
        status.textContent = prefix + '可直接进行第 2 步登录';
        return;
      }
      const lines = d.accounts.map(function (a) {
        return a.name + ' — api_id=' + a.api_id + (a.is_builtin ? '（内置）' : (a.api_hash_set ? '（自定义）' : ' ✗'));
      });
      status.textContent = prefix + '账号：' + lines.join('；');
      if (!document.getElementById('cred-api-id').value) {
        document.getElementById('cred-account').value = d.accounts[0].name;
      }
    } catch (e) {
      // 忽略
    }
  }
  loadCredentials();

  // 恢复内置凭据
  const credReset = document.getElementById('cred-reset');
  if (credReset) {
    credReset.addEventListener('click', async function () {
      document.getElementById('cred-api-id').value = '';
      document.getElementById('cred-api-hash').value = '';
      const errBox = document.getElementById('cred-error');
      const okBox = document.getElementById('cred-success');
      errBox.hidden = true;
      try {
        const res = await fetch('/api/credentials', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            account: document.getElementById('cred-account').value.trim(),
            api_id: 0,
            api_hash: ''
          })
        });
        const d = await res.json();
        if (res.ok) {
          okBox.textContent = d.env_override
            ? '已写入内置值；当前仍由环境变量覆盖，删除环境变量后生效'
            : '已恢复内置官方凭据，可直接登录';
          okBox.hidden = false;
          loadCredentials();
        } else {
          errBox.textContent = d.error || '恢复失败';
          errBox.hidden = false;
        }
      } catch (err) {
        errBox.textContent = '网络错误：' + err.message;
        errBox.hidden = false;
      }
    });
  }

  // --- 扫码登录 ---
  let qrSessionId = '';
  let qrPollTimer = null;

  const qrStartBtn = document.getElementById('qr-start');
  if (qrStartBtn) {
    qrStartBtn.addEventListener('click', async function () {
      const errBox = document.getElementById('qr-error');
      errBox.hidden = true;
      const btn = this;
      btn.disabled = true;
      try {
        const res = await fetch('/api/login/qr', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            account: document.getElementById('cred-account').value.trim(),
            api_id: parseInt(document.getElementById('cred-api-id').value, 10) || 0,
            api_hash: document.getElementById('cred-api-hash').value.trim()
          })
        });
        const d = await res.json();
        if (!res.ok) {
          errBox.textContent = d.error || '启动失败';
          errBox.hidden = false;
          return;
        }
        qrSessionId = d.id;
        document.getElementById('qr-box').hidden = false;
        pollQR();
      } catch (err) {
        errBox.textContent = '网络错误：' + err.message;
        errBox.hidden = false;
      } finally {
        btn.disabled = false;
      }
    });
  }

  async function pollQR() {
    if (!qrSessionId) return;
    try {
      const res = await fetch('/api/login/status?id=' + encodeURIComponent(qrSessionId));
      const d = await res.json();
      if (!res.ok) {
        document.getElementById('qr-hint').textContent = d.error || '会话已过期';
        return;
      }
      if (d.qr_url) {
        renderQR(d.qr_url);
        const left = d.qr_expires ? Math.max(0, d.qr_expires - Math.floor(Date.now() / 1000)) : 0;
        document.getElementById('qr-hint').textContent =
          left > 0 ? '等待扫码… 二维码 ' + left + ' 秒后自动刷新，请用手机 Telegram 立即扫码' : '等待扫码… 二维码即将刷新';
      }
      if (d.state === 'success') {
        document.getElementById('qr-hint').textContent =
          '✅ 登录成功：@' + (d.username || '') + ' (ID ' + d.user_id + ')';
        setTimeout(function () { window.location.reload(); }, 1500);
        return;
      }
      if (d.state === 'failed') {
        const eb = document.getElementById('qr-error');
        eb.textContent = d.error || '登录失败';
        eb.hidden = false;
        return;
      }
      qrPollTimer = setTimeout(pollQR, 2000);
    } catch (e) {
      qrPollTimer = setTimeout(pollQR, 3000);
    }
  }

  // 渲染二维码（用外部服务，失败则显示可复制链接）
  function renderQR(url) {
    const box = document.getElementById('qr-image');
    box.innerHTML = '';
    const img = document.createElement('img');
    img.src = 'https://api.qrserver.com/v1/create-qr-code/?size=240x240&data=' + encodeURIComponent(url);
    img.alt = 'Telegram 登录二维码';
    img.width = 240;
    img.height = 240;
    img.onerror = function () {
      box.innerHTML = '<p class="hint">无法加载二维码图片。请在 Telegram 中打开以下链接：</p>';
      const code = document.createElement('code');
      code.style.wordBreak = 'break-all';
      code.textContent = url;
      box.appendChild(code);
    };
    box.appendChild(img);

    const link = document.createElement('p');
    link.className = 'hint';
    link.style.wordBreak = 'break-all';
    link.style.fontSize = '11px';
    link.textContent = url;
    box.appendChild(link);
  }

  const qrCancelBtn = document.getElementById('qr-cancel');
  if (qrCancelBtn) {
    qrCancelBtn.addEventListener('click', async function () {
      if (qrPollTimer) clearTimeout(qrPollTimer);
      if (qrSessionId) {
        try {
          await fetch('/api/login/cancel', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ id: qrSessionId })
          });
        } catch (e) { /* 忽略 */ }
      }
      qrSessionId = '';
      document.getElementById('qr-box').hidden = true;
    });
  }

  // --- 手机号登录 ---
  let phoneSessionId = '';
  let phonePollTimer = null;

  const phoneForm = document.getElementById('phone-form');
  if (phoneForm) {
    phoneForm.addEventListener('submit', async function (e) {
      e.preventDefault();
      const errBox = document.getElementById('phone-error');
      errBox.hidden = true;
      try {
        const res = await fetch('/api/login/phone', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            account: document.getElementById('cred-account').value.trim(),
            api_id: parseInt(document.getElementById('cred-api-id').value, 10) || 0,
            api_hash: document.getElementById('cred-api-hash').value.trim(),
            phone: document.getElementById('phone-input').value.trim()
          })
        });
        const d = await res.json();
        if (!res.ok) {
          errBox.textContent = d.error || '发送失败';
          errBox.hidden = false;
          return;
        }
        phoneSessionId = d.id;
        document.getElementById('phone-code-box').hidden = false;
        pollPhone();
      } catch (err) {
        errBox.textContent = '网络错误：' + err.message;
        errBox.hidden = false;
      }
    });
  }

  async function pollPhone() {
    if (!phoneSessionId) return;
    try {
      const res = await fetch('/api/login/status?id=' + encodeURIComponent(phoneSessionId));
      const d = await res.json();
      if (!res.ok) {
        const eb = document.getElementById('phone-error');
        eb.textContent = d.error || '会话已过期';
        eb.hidden = false;
        return;
      }
      if (d.hint) {
        document.getElementById('phone-hint').textContent = '验证码已发送：' + d.hint;
      }
      if (d.state === 'waiting_password') {
        document.getElementById('phone-pwd-box').hidden = false;
        document.getElementById('phone-code-box').hidden = true;
      }
      if (d.state === 'success') {
        const sb = document.getElementById('phone-success');
        sb.textContent = '✅ 登录成功：@' + (d.username || '') + ' (ID ' + d.user_id + ')';
        sb.hidden = false;
        setTimeout(function () { window.location.reload(); }, 1500);
        return;
      }
      if (d.state === 'failed') {
        const eb = document.getElementById('phone-error');
        eb.textContent = d.error || '登录失败';
        eb.hidden = false;
        return;
      }
      phonePollTimer = setTimeout(pollPhone, 2000);
    } catch (e) {
      phonePollTimer = setTimeout(pollPhone, 3000);
    }
  }

  const codeForm = document.getElementById('code-form');
  if (codeForm) {
    codeForm.addEventListener('submit', async function (e) {
      e.preventDefault();
      const errBox = document.getElementById('phone-error');
      errBox.hidden = true;
      try {
        const res = await fetch('/api/login/code', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            id: phoneSessionId,
            code: document.getElementById('code-input').value.trim()
          })
        });
        const d = await res.json();
        if (!res.ok) {
          errBox.textContent = d.error || '提交失败';
          errBox.hidden = false;
        }
      } catch (err) {
        errBox.textContent = '网络错误：' + err.message;
        errBox.hidden = false;
      }
    });
  }

  const pwdForm = document.getElementById('pwd-form');
  if (pwdForm) {
    pwdForm.addEventListener('submit', async function (e) {
      e.preventDefault();
      const errBox = document.getElementById('phone-error');
      errBox.hidden = true;
      try {
        const res = await fetch('/api/login/password', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            id: phoneSessionId,
            password: document.getElementById('pwd-input').value
          })
        });
        const d = await res.json();
        if (!res.ok) {
          errBox.textContent = d.error || '提交失败';
          errBox.hidden = false;
        }
      } catch (err) {
        errBox.textContent = '网络错误：' + err.message;
        errBox.hidden = false;
      }
    });
  }

  // --- 缓存统计 ---
  const cacheStats = document.getElementById('cache-stats');
  if (cacheStats) {
    fetch('/api/status')
      .then(function (r) { return r.json(); })
      .then(function (d) {
        if (d.cache) {
          cacheStats.textContent = '聊天 ' + d.cache.chats + ' · 消息 ' + d.cache.messages +
            ' · 用户 ' + d.cache.users + ' · 数据库 ' +
            (d.cache.db_size_mb || 0).toFixed(2) + ' MB';
        } else {
          cacheStats.textContent = '无数据';
        }
      })
      .catch(function (e) { cacheStats.textContent = '加载失败：' + e.message; });
  }
})();
