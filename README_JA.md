# Telegram MCP

[中文](README.md) | [English](README_EN.md) | [日本語](README_JA.md) | [Русский](README_RU.md)

[MCP](https://modelcontextprotocol.io) 経由で AI エージェントに実際の Telegram アカウントの完全な操作権限を与えます — メッセージ送信、グループ・チャンネル管理、ユーザープロフィール閲覧、ボット操作、音声文字起こし。

Bot API ではありません。**完全な MTProto クライアント**です：個人チャット履歴の閲覧、ボット管理、Bot API では不可能なことができます。

## 特徴

| 機能 | 説明 |
|---|---|
| **Pure Go、cgo 不要** | Windows / Linux / macOS / Termux 対応の単一バイナリ |
| **トークン効率** | 568 個の細粒度ツール（24k+ トークン）ではなく 12 個の集約ツール（8k トークン） |
| **レスポンス射影** | `chat list(50)` は 458 トークン（全フィールドなら 133k、99.7% 削減） |
| **引用自動展開** | 引用された画像・ファイル・音声が注釈され、ダウンロード可能 |
| **信頼レベル** | 管理者メッセージはそのまま、外部コンテンツは `<untrusted_content>` でラップ |
| **Web UI** | LAN アクセス可能、パスワードのみのログイン、依存関係ゼロで埋め込み |

## クイックスタート

### 1. Telegram API 認証情報を取得

https://my.telegram.org → API development tools → アプリを作成。`api_id` と `api_hash` を取得します。

### 2. ダウンロード

[Releases](../../releases) からお使いのプラットフォームを選択：

| プラットフォーム | ファイル |
|---|---|
| Windows x64 | `tgmcp-windows-x86_64.zip` |
| Windows ARM64 | `tgmcp-windows-arm64.zip` |
| Linux x64 | `tgmcp-linux-x86_64.tar.gz` |
| Linux ARM64 | `tgmcp-linux-arm64.tar.gz` |
| Linux ARMv6/v7 | `tgmcp-linux-armv6.tar.gz` / `armv7` |
| Linux RISC-V 64 | `tgmcp-linux-riscv64.tar.gz` |
| Linux LoongArch 64 | `tgmcp-linux-loong64.tar.gz` |
| macOS Apple Silicon | `tgmcp-darwin-arm64.tar.gz` |
| macOS Intel | `tgmcp-darwin-x86_64.tar.gz` |
| FreeBSD | `tgmcp-freebsd-x86_64.tar.gz` / `arm64` |

Debian/Ubuntu 用 `.deb`、RHEL/Fedora 用 `.rpm` パッケージも用意しています。

### 3. ログイン

```bash
# QR スキャン（推奨）
./tgmcp auth --account main --qr

# または電話番号
./tgmcp auth --account main --phone +819012345678
```

### 4. MCP クライアント設定

**Claude Code**（`~/.claude.json` またはプロジェクトの `.mcp.json`）：

```json
{
  "mcpServers": {
    "telegram": {
      "command": "/path/to/tgmcp",
      "args": ["serve"],
      "env": {
        "TGMCP_DATA_DIR": "/path/to/data"
      }
    }
  }
}
```

**Cursor**（`~/.cursor/mcp.json`）：同じ形式。

**VS Code**（`.vscode/mcp.json`）：

```json
{
  "servers": {
    "telegram": {
      "type": "stdio",
      "command": "/path/to/tgmcp",
      "args": ["serve"]
    }
  }
}
```

### 5. 使い方

AI に話しかけるだけ：

- 「すべてのグループと未読数を一覧表示して」
- 「グループ X の最新 20 件のメッセージを読んで」
- 「このメッセージを別のグループに転送して」
- 「@someone に『こんにちは』と送って」
- 「@Eddie のプロフィールを見せて」
- 「@some_bot で今日のチェックインを取得して」

## Web UI

サービス起動後 `http://<あなたのIP>:8080/ui` にアクセス。

| ページ | 機能 |
|---|---|
| アカウント | アカウント状態、キャッシュ統計 |
| チャット | 会話リスト + メッセージ + 手動送信 |
| 自動返信 | ルール設定 + トグル + ログ |
| 音声 | ASR バックエンドとパス設定 |
| 管理者 | 管理者の追加・削除（数値 ID で保存） |
| 設定 | パスワード変更、サービス情報 |

**デフォルトパスワード `Admin`** — 初回ログイン後すぐに変更してください。

## コマンド

```bash
tgmcp auth      ログイン（--qr スキャン / --phone 電話番号）
tgmcp serve     MCP サービス起動（--http で HTTP トランスポート）
tgmcp doctor    環境チェック
tgmcp asr       音声文字起こし設定（setup / status / config）
tgmcp version   バージョン
```

## 12 個のツール

| ツール | 機能 |
|---|---|
| `system` | サービス状態、アカウント一覧、環境チェック |
| `chat` | 会話一覧、詳細、未読、既読、フォルダ |
| `message` | 送信・編集・削除・転送・検索・履歴・リアクション・ピン + **引用展開** |
| `group` | メンバー、招待・キック、管理者、権限、承認、参加・退出、コメント |
| `contact` | ユーザープロフィールページ、連絡先、検索、追加、ブロック |
| `media` | 写真・ファイル・動画・音声・アルバム送信、ダウンロード、**オンデマンド画像表示** |
| `sticker` | ステッカーセット、ステッカー送信、カスタム絵文字、リアクション |
| `account` | 自分のプロフィール、名前・自己紹介・アバター・ユーザー名変更、セッション |
| `bot` | 自分のボット管理、インラインクエリ |
| `botuse` | 他人のボット使用（ボタン読み取り、コールバッククリック、WebApp） |
| `asr` | 音声テキスト化（4 バックエンド） |
| `stats` | グループ統計、チャット統計、キャッシュ統計 |

## ASR カスタムパス

既に sherpa-onnx がインストールされている場合、既存のパスを指定できます：

```bash
# 環境変数（最優先）
export TGMCP_ASR_MODEL_DIR=/opt/sherpa/models/paraformer-zh-small
export TGMCP_ASR_CLI=/usr/local/bin/sherpa-onnx-offline

# または設定経由
tgmcp asr config --cli /usr/local/bin/sherpa-onnx --model-dir /opt/sherpa/models/...

# 永続設定
tgmcp asr setup   # モデルが既にあればダウンロードをスキップ
tgmcp asr status  # 解決されたパスを表示
```

**優先順位**：環境変数 > `config.json` > デフォルト `{DataDir}/models/...`

認識されるモデルファイル：`model.onnx`、`model.int8.onnx`、`encoder.onnx`、`encoder.int8.onnx`、`paraformer.onnx`。

## 設定

データディレクトリはデフォルト `~/.tgmcp`、`TGMCP_DATA_DIR` で変更可能。

```
~/.tgmcp/
├── config.json          # 設定
├── cache.db             # SQLite キャッシュ（チャット/メッセージ/ユーザー/管理者）
├── sessions/
│   └── main/
│       └── session.json # Telegram セッション（★ auth key を含む、厳重管理）
├── models/              # ASR モデル
└── downloads/           # メディアダウンロード
```

## セキュリティ注意事項

- **セッションファイルは平文 JSON**（auth key を含む）。漏洩 = アカウント乗っ取り。
- **管理者は数値 UserID で識別** — ユーザー名は変更・占有される可能性があるため、主キーにしない。
- **Web UI のデフォルトパスワード `Admin`** は `0.0.0.0` にバインドされるため、LAN 内の誰でもアクセス可能。すぐに変更してください。
- **管理者メッセージはインジェクション防御をバイパス** — 管理者アカウントが侵害されるとモデルも侵害されます。

## 開発

```bash
go vet ./...
go test ./...
go build -o bin/tgmcp ./cmd/tgmcp
```

リリースバイナリはすべて GitHub Actions（GoReleaser + `CGO_ENABLED=0` クロスコンパイル）で生成されます。

## プラットフォーム対応

| OS | アーキテクチャ | 備考 |
|---|---|---|
| Linux | x86_64, i386, arm64, armv6, armv7, riscv64, loong64, mipsle, mips64, mips64le, ppc64, ppc64le, s390x | 完全対応 |
| Windows | x86_64, i386, arm64 | 完全対応 |
| macOS | x86_64, arm64 | 完全対応 |
| FreeBSD | x86_64, i386, arm64 | 完全対応 |
| NetBSD | x86_64, arm64 | 完全対応 |
| OpenBSD | x86_64 | 完全対応 |
| **linux/mips（ビッグエンディアン 32bit）** | — | ❌ **非対応**（`modernc.org/libc` に mips 実装なし） |

## ライセンス

**PolyForm Noncommercial 1.0.0** + 追加条項：

- ✅ 個人利用（自分のアカウントでの商用利用を含む、あらゆる目的）
- ✅ 非営利組織内での利用
- ✅ フォークと改変（自分の利用目的）
- ✅ 無償での未改変コピー配布
- ✅ 無償の非営利オープンソースプロジェクトへの組み込み
- ❌ **販売、レンタル、サービスとしての課金**
- ❌ **著作者表示の削除**（著作者名 + 元リポジトリ URL の保持が必要）

詳細は [LICENSE](LICENSE) を参照。商用ライセンスについては著者に連絡してください。

---

*Copyright 2026 Edwin (辰星) — https://github.com/midi-HL/telegram-mcp*
