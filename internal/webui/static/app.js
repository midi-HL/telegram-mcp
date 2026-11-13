// Telegram MCP Web UI 前端脚本
(function () {
  'use strict';

  // 登出
  const logoutBtn = document.getElementById('logout-btn');
  if (logoutBtn) {
    logoutBtn.addEventListener('click', async () => {
      try {
        await fetch('/api/logout', {method: 'POST'});
      } catch (e) { /* ignore */ }
      window.location.href = '/ui/login';
    });
  }

  // 添加管理员
  const addAdminForm = document.getElementById('add-admin-form');
  if (addAdminForm) {
    addAdminForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      const errBox = document.getElementById('admin-error');
      const okBox = document.getElementById('admin-success');
      errBox.hidden = true;
      okBox.hidden = true;

      const username = document.getElementById('admin-username').value.trim();
      if (!username) return;

      const btn = e.target.querySelector('button');
      btn.disabled = true;
      try {
        const res = await fetch('/api/admins', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({username})
        });
        const data = await res.json();
        if (res.ok) {
          okBox.textContent = `已添加管理员 @${data.username}（ID ${data.user_id}）`;
          okBox.hidden = false;
          setTimeout(() => window.location.reload(), 1200);
        } else {
          errBox.textContent = data.error || '添加失败';
          errBox.hidden = false;
        }
      } catch (err) {
        errBox.textContent = '网络错误：' + err.message;
        errBox.hidden = false;
      } finally {
        btn.disabled = false;
      }
    });
  }

  // 删除管理员
  document.querySelectorAll('.btn-danger[data-user-id]').forEach(btn => {
    btn.addEventListener('click', async () => {
      const id = btn.dataset.userId;
      if (!confirm(`确认删除管理员（ID ${id}）？`)) return;
      try {
        const res = await fetch('/api/admins', {
          method: 'DELETE',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({user_id: parseInt(id, 10)})
        });
        if (res.ok) {
          window.location.reload();
        } else {
          const data = await res.json();
          alert(data.error || '删除失败');
        }
      } catch (e) {
        alert('网络错误：' + e.message);
      }
    });
  });

  // ===== Agent 唤醒 =====
  (function () {
    const form = document.getElementById('wake-form');
    if (!form) return;
    const errBox = document.getElementById('wake-error');
    const okBox = document.getElementById('wake-success');
    const statusBox = document.getElementById('wake-status');
    const output = document.getElementById('wake-output');

    function showError(msg) {
      errBox.textContent = msg;
      errBox.hidden = false;
      okBox.hidden = true;
    }
    function showOK(msg) {
      okBox.textContent = msg;
      okBox.hidden = false;
      errBox.hidden = true;
    }
    function showOutput(data) {
      output.textContent = JSON.stringify(data, null, 2);
      output.hidden = false;
    }

    function readConfig() {
      return {
        enabled: document.getElementById('wake-enabled').checked,
        mode: document.getElementById('wake-mode').value,
        include_out: document.getElementById('wake-include-out').checked,
        include_text: document.getElementById('wake-include-text').checked,
        max_text_runes: parseInt(document.getElementById('wake-max-text').value, 10) || 280,
        debounce_seconds: parseInt(document.getElementById('wake-debounce').value, 10) || 0,
        webhook_url: document.getElementById('wake-webhook-url').value.trim(),
        webhook_token: document.getElementById('wake-webhook-token').value.trim(),
        prompt: document.getElementById('wake-prompt').value.trim()
      };
    }

    function fillConfig(c) {
      document.getElementById('wake-enabled').checked = !!c.enabled;
      document.getElementById('wake-mode').value = c.mode || 'notify';
      document.getElementById('wake-include-out').checked = !!c.include_out;
      document.getElementById('wake-include-text').checked = c.include_text !== false;
      document.getElementById('wake-max-text').value = c.max_text_runes || 280;
      document.getElementById('wake-debounce').value = c.debounce_seconds || 0;
      document.getElementById('wake-webhook-url').value = c.webhook_url || '';
      document.getElementById('wake-webhook-token').value = c.webhook_token || '';
      document.getElementById('wake-prompt').value = c.prompt || '';
    }

    async function loadStatus() {
      try {
        const res = await fetch('/api/wake');
        const d = await res.json();
        if (!res.ok) { statusBox.textContent = d.error || '加载失败'; return; }
        fillConfig(d.config || {});
        const s = d.stats || {};
        statusBox.textContent = '当前：' + (s.enabled ? '已开启' : '已关闭') +
          ' · 模式 ' + (s.mode || 'notify') + ' · 待读 ' + (s.pending || 0) +
          ' · 已投递 ' + (s.delivered || 0) + ' · 传输 ' + (s.transport || '-');
      } catch (e) {
        statusBox.textContent = '加载失败：' + e.message;
      }
    }

    async function postJSON(url, payload) {
      const res = await fetch(url, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(payload || {})
      });
      const d = await res.json();
      if (!res.ok) throw new Error(d.error || ('HTTP ' + res.status));
      return d;
    }

    form.addEventListener('submit', async function (e) {
      e.preventDefault();
      try {
        const d = await postJSON('/api/wake', readConfig());
        showOK('唤醒配置已保存并立即生效');
        fillConfig(d.config || readConfig());
        await loadStatus();
      } catch (err) { showError(err.message); }
    });

    document.getElementById('wake-enabled').addEventListener('change', async function () {
      try {
        await postJSON('/api/wake/toggle', {enabled: this.checked});
        showOK(this.checked ? '已开启 Agent 唤醒' : '已关闭 Agent 唤醒');
        await loadStatus();
      } catch (err) { showError(err.message); }
    });

    document.getElementById('wake-test').addEventListener('click', async function () {
      try {
        const d = await postJSON('/api/wake/test', {text: '这是一条来自 Web UI 的唤醒测试消息'});
        showOK('测试已投递：' + ((d.notification && d.notification.channels || []).join(', ') || '已加入待读队列'));
        showOutput(d.notification);
        await loadStatus();
      } catch (err) { showError(err.message); }
    });

    document.getElementById('wake-pending').addEventListener('click', async function () {
      try {
        const res = await fetch('/api/wake/pending');
        const d = await res.json();
        if (!res.ok) throw new Error(d.error || '读取失败');
        const list = d.notifications || [];
        showOK('待读共 ' + list.length + ' 条');
        showOutput(list.length ? list : {message: '队列为空'});
      } catch (err) { showError(err.message); }
    });

    document.getElementById('wake-ack').addEventListener('click', async function () {
      try {
        const d = await postJSON('/api/wake/ack', {});
        showOK('已清空 ' + (d.cleared || 0) + ' 条待读');
        output.hidden = true;
        await loadStatus();
      } catch (err) { showError(err.message); }
    });

    loadStatus();
  })();

  // 修改密码
  const pwdForm = document.getElementById('password-form');
  if (pwdForm) {
    pwdForm.addEventListener('submit', async (e) => {
      e.preventDefault();
      const errBox = document.getElementById('pwd-error');
      const okBox = document.getElementById('pwd-success');
      errBox.hidden = true;
      okBox.hidden = true;

      const oldPwd = document.getElementById('old-password').value;
      const newPwd = document.getElementById('new-password').value;
      const confirmPwd = document.getElementById('confirm-password').value;

      if (newPwd !== confirmPwd) {
        errBox.textContent = '两次输入的新密码不一致';
        errBox.hidden = false;
        return;
      }
      if (newPwd.length < 8) {
        errBox.textContent = '新密码至少 8 位';
        errBox.hidden = false;
        return;
      }

      const btn = e.target.querySelector('button');
      btn.disabled = true;
      try {
        const res = await fetch('/api/password', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({
            old_password: oldPwd,
            new_password: newPwd,
            confirm: confirmPwd
          })
        });
        const data = await res.json();
        if (res.ok) {
          okBox.textContent = '密码已修改，请重新登录…';
          okBox.hidden = false;
          setTimeout(() => window.location.href = '/ui/login', 1500);
        } else {
          errBox.textContent = data.error || '修改失败';
          errBox.hidden = false;
        }
      } catch (err) {
        errBox.textContent = '网络错误：' + err.message;
        errBox.hidden = false;
      } finally {
        btn.disabled = false;
      }
    });
  }
})();

// ===== 聊天面板 =====
(function () {
  const chatList = document.getElementById('chat-list');
  if (!chatList) return;

  let currentChat = 0;

  function fmtTime(ts) {
    if (!ts) return '';
    const d = new Date(ts * 1000);
    return d.toLocaleString('zh-CN', {month:'2-digit', day:'2-digit', hour:'2-digit', minute:'2-digit'});
  }

  function esc(s) {
    const div = document.createElement('div');
    div.textContent = s == null ? '' : String(s);
    return div.innerHTML;
  }

  async function loadChats() {
    try {
      const res = await fetch('/api/chats');
      const d = await res.json();
      if (!res.ok) { chatList.textContent = d.error || '加载失败'; return; }
      const chats = d.chats || [];
      if (!chats.length) { chatList.textContent = '暂无会话'; return; }
      chatList.innerHTML = chats.map(function(c) {
        const badge = c.unread ? '<span class="unread-badge">' + c.unread + '</span>' : '';
        return '<div class="chat-item" data-id="' + c.id + '" data-title="' + esc(c.title) + '">' +
               '<div class="chat-item-title">' + esc(c.title) + badge + '</div>' +
               '<div class="chat-item-meta">' + esc(c.type) + '</div></div>';
      }).join('');
      chatList.querySelectorAll('.chat-item').forEach(function(el) {
        el.addEventListener('click', function() {
          chatList.querySelectorAll('.chat-item').forEach(function(x) { x.classList.remove('active'); });
          el.classList.add('active');
          currentChat = parseInt(el.dataset.id, 10);
          document.getElementById('chat-title').textContent = el.dataset.title;
          loadMessages();
        });
      });
    } catch (e) {
      chatList.textContent = '网络错误：' + e.message;
    }
  }

  async function loadMessages() {
    const box = document.getElementById('message-list');
    if (!currentChat) return;
    box.innerHTML = '加载中…';
    try {
      const res = await fetch('/api/messages?chat_id=' + currentChat + '&limit=40');
      const d = await res.json();
      if (!res.ok) { box.textContent = d.error || '加载失败'; return; }
      const msgs = (d.messages || []).slice().reverse();
      if (!msgs.length) { box.textContent = '暂无消息'; return; }
      box.innerHTML = msgs.map(function(m) {
        const cls = m.out ? 'msg msg-out' : 'msg msg-in';
        const trust = m.trust === 'external' ? '<span class="msg-trust">外部</span>' : '';
        const sender = (!m.out && m.sender_name) ? '<div class="msg-sender">' + esc(m.sender_name) + trust + '</div>' : '';
        return '<div class="' + cls + '">' + sender + esc(m.text) +
               '<div class="msg-time">' + fmtTime(m.date) + '</div></div>';
      }).join('');
      box.scrollTop = box.scrollHeight;
    } catch (e) {
      box.textContent = '网络错误：' + e.message;
    }
  }

  async function sendMessage() {
    const ta = document.getElementById('compose-text');
    const errBox = document.getElementById('chat-error');
    errBox.hidden = true;
    const text = ta.value.trim();
    if (!text || !currentChat) return;
    try {
      const res = await fetch('/api/send', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({chat_id: currentChat, text: text})
      });
      const d = await res.json();
      if (!res.ok) { errBox.textContent = d.error || '发送失败'; errBox.hidden = false; return; }
      ta.value = '';
      loadMessages();
    } catch (e) {
      errBox.textContent = '网络错误：' + e.message;
      errBox.hidden = false;
    }
  }

  document.getElementById('send-btn').addEventListener('click', sendMessage);
  document.getElementById('compose-text').addEventListener('keydown', function(e) {
    if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) { e.preventDefault(); sendMessage(); }
  });

  loadChats();
})();

// ===== 自动回复 =====
(function () {
  const toggle = document.getElementById('ar-toggle');
  if (!toggle) return;

  toggle.addEventListener('change', async function() {
    try {
      const res = await fetch('/api/autoreply/toggle', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({enabled: toggle.checked})
      });
      if (res.ok) {
        window.location.reload();
      } else {
        const d = await res.json();
        alert(d.error || '切换失败');
        toggle.checked = !toggle.checked;
      }
    } catch (e) {
      alert('网络错误：' + e.message);
      toggle.checked = !toggle.checked;
    }
  });

  const form = document.getElementById('rule-form');
  form.addEventListener('submit', async function(e) {
    e.preventDefault();
    const errBox = document.getElementById('rule-error');
    const okBox = document.getElementById('rule-success');
    errBox.hidden = true; okBox.hidden = true;

    const scopeRaw = document.getElementById('rule-scope').value.trim();
    const scope = scopeRaw ? scopeRaw.split(',').map(function(s) { return s.trim(); }).filter(Boolean) : null;

    const rule = {
      enabled: document.getElementById('rule-enabled').checked,
      chat_scope: scope,
      keyword: document.getElementById('rule-keyword').value.trim(),
      prompt: document.getElementById('rule-prompt').value.trim()
    };

    try {
      const res = await fetch('/api/autoreply', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(rule)
      });
      const d = await res.json();
      if (res.ok) {
        okBox.textContent = '规则已添加';
        okBox.hidden = false;
        setTimeout(function() { window.location.reload(); }, 900);
      } else {
        errBox.textContent = d.error || '添加失败';
        errBox.hidden = false;
      }
    } catch (err) {
      errBox.textContent = '网络错误：' + err.message;
      errBox.hidden = false;
    }
  });

  document.querySelectorAll('.btn-danger[data-rule-id]').forEach(function(btn) {
    btn.addEventListener('click', async function() {
      if (!confirm('确认删除该规则？')) return;
      try {
        const res = await fetch('/api/autoreply', {
          method: 'DELETE',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({id: parseInt(btn.dataset.ruleId, 10)})
        });
        if (res.ok) window.location.reload();
      } catch (e) { alert('网络错误：' + e.message); }
    });
  });
})();
