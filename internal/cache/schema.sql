-- Telegram MCP 本地缓存 schema
-- 设计原则：字段与 DTO 投影对齐，避免存了用不上的数据

PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA foreign_keys = ON;

-- 聊天（会话）缓存
CREATE TABLE IF NOT EXISTS chats (
    id          INTEGER PRIMARY KEY,
    title       TEXT    NOT NULL,
    type        TEXT    NOT NULL,          -- user/group/supergroup/channel/secret
    username    TEXT,
    about       TEXT,
    member_count INTEGER DEFAULT 0,
    unread      INTEGER DEFAULT 0,
    muted       INTEGER DEFAULT 0,
    pinned      INTEGER DEFAULT 0,
    can_send    INTEGER DEFAULT 1,
    is_admin    INTEGER DEFAULT 0,
    is_creator  INTEGER DEFAULT 0,
    last_text   TEXT,
    last_at     INTEGER DEFAULT 0,
    access_hash INTEGER DEFAULT 0,         -- 用于重建 InputPeer
    updated_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_chats_last ON chats(last_at DESC);
CREATE INDEX IF NOT EXISTS idx_chats_unread ON chats(unread DESC);

-- 消息缓存（每聊天最多 200 条，由应用层淘汰）
CREATE TABLE IF NOT EXISTS messages (
    chat_id     INTEGER NOT NULL,
    msg_id      INTEGER NOT NULL,
    sender_id   INTEGER DEFAULT 0,
    sender_name TEXT,
    date        INTEGER NOT NULL,
    text        TEXT,
    media_kind  TEXT,
    media_meta  TEXT,                       -- JSON
    reply_to    INTEGER DEFAULT 0,
    out         INTEGER DEFAULT 0,
    edited_at   INTEGER DEFAULT 0,
    views       INTEGER DEFAULT 0,
    PRIMARY KEY (chat_id, msg_id)
);
CREATE INDEX IF NOT EXISTS idx_messages_date ON messages(chat_id, date DESC);
CREATE INDEX IF NOT EXISTS idx_messages_global ON messages(date DESC);

-- 用户资料缓存
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY,
    access_hash   INTEGER NOT NULL,
    first_name    TEXT,
    last_name     TEXT,
    username      TEXT,
    usernames     TEXT,                     -- JSON 数组
    phone         TEXT,
    bio           TEXT,
    status_text   TEXT,
    last_seen     INTEGER DEFAULT 0,
    is_bot        INTEGER DEFAULT 0,
    is_premium    INTEGER DEFAULT 0,
    is_verified   INTEGER DEFAULT 0,
    is_contact    INTEGER DEFAULT 0,
    is_mutual     INTEGER DEFAULT 0,
    birthday      TEXT,
    common_chats  INTEGER DEFAULT 0,
    photo_id      INTEGER DEFAULT 0,
    channel_id    INTEGER DEFAULT 0,
    updated_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);

-- 管理员白名单（★ 数字 UserID 主键，用户名仅显示）
CREATE TABLE IF NOT EXISTS admins (
    user_id           INTEGER PRIMARY KEY,
    username_snapshot TEXT,
    added_at          INTEGER NOT NULL,
    added_by          INTEGER DEFAULT 0
);

-- 自动回复规则
CREATE TABLE IF NOT EXISTS autoreply_rules (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    enabled    INTEGER NOT NULL DEFAULT 0,
    chat_scope TEXT,                        -- JSON 数组，null=全部
    keyword    TEXT,
    prompt     TEXT,
    created_at INTEGER NOT NULL
);

-- 自动回复日志
CREATE TABLE IF NOT EXISTS autoreply_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    rule_id    INTEGER,
    chat_id    INTEGER,
    in_msg_id  INTEGER,
    out_text   TEXT,
    error      TEXT,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_arlog_time ON autoreply_log(created_at DESC);

-- Web UI 凭据（单行，纯密码无用户名）
CREATE TABLE IF NOT EXISTS webui_credential (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    password_hash TEXT    NOT NULL,
    is_default    INTEGER NOT NULL DEFAULT 1,
    updated_at    INTEGER NOT NULL
);

-- Web UI 会话
CREATE TABLE IF NOT EXISTS webui_session (
    session_id TEXT PRIMARY KEY,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    last_seen  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_session_exp ON webui_session(expires_at);

-- 登录失败限流
CREATE TABLE IF NOT EXISTS login_attempt (
    ip           TEXT PRIMARY KEY,
    fail_count   INTEGER NOT NULL DEFAULT 0,
    locked_until INTEGER NOT NULL DEFAULT 0
);

-- 元数据（schema 版本等）
CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
