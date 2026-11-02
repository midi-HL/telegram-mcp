# sherpa-onnx 部署事实清单

> 采集时间：2026-11-02
> 采集方式：在目标机器上实地执行命令，输出原样记录
> 目标机器：`192.168.0.7:8022`（root）

---

## 速览（可直接复制）

```
CLI_PATH=/root/.picoclaw-asr/bin/sherpa-onnx-offline
CLI_VERSION=v2.0.1（官方预编译发布版，非自编译）
MODEL_DIR=/root/.picoclaw-asr/models/sherpa-onnx-paraformer-zh-small-2024-03-09
MODEL_TYPE=paraformer-zh-small（FunASR Paraformer，int8 量化）
MODEL_FILES=model.int8.onnx,tokens.txt,am.mvn,config.yaml,README.md,add-model-metadata.py,generate-tokens.py,test_wavs/
MODEL_SIZE=80MB（目录合计；model.int8.onnx 单文件 81828675 字节）
INVOKE_CMD=LD_LIBRARY_PATH=/root/.picoclaw-asr/bin /root/.picoclaw-asr/bin/sherpa-onnx-offline --tokens=<MODEL_DIR>/tokens.txt --paraformer=<MODEL_DIR>/model.int8.onnx --num-threads=2 --decoding-method=greedy_search <INPUT.wav>
INPUT_FORMAT=WAV / RIFF，PCM 16-bit 小端，16000 Hz，单声道
OUTPUT_FORMAT=stdout 上一行 JSON，取 .text 字段
OS=Linux aarch64（Android 13 内核 5.15.148）
ENV=proot 容器内的 Debian GNU/Linux trixie/sid，root 用户
PROJECT_NAME=PicoClaw（picoclaw-edition fork）
PROJECT_PATH=/root/picoclaw-edition
```

---

## 一、sherpa-onnx CLI 可执行文件

| 项目 | 事实 |
|---|---|
| 完整路径 | `/root/.picoclaw-asr/bin/sherpa-onnx-offline` |
| 文件大小 | 1,775,160 字节 |
| 权限 | `-rwxr-xr-x`（已 `chmod +x`） |
| 版本 | **v2.0.1** |
| 来源 | **官方预编译发布版**，非自编译 |
| 来源资产 | `sherpa-onnx-v2.0.1-linux-aarch64-shared-cpu.tar.bz2` |
| 下载地址 | `https://github.com/k2-fsa/sherpa-onnx/releases/download/v2.0.1/sherpa-onnx-v2.0.1-linux-aarch64-shared-cpu.tar.bz2` |

### 版本是怎么确定的

`--version` **不打印版本号**（它直接输出 usage 文本），所以版本来自下载时使用的 release tag，不是运行时探测。

### 同目录下的共享库（必须一起存在）

```
/root/.picoclaw-asr/bin/
├── sherpa-onnx-offline              ← 主程序
├── libonnxruntime.so                ← 34,606,912 字节
├── libsherpa-onnx-c-api.so          ← 4,443,416 字节
├── libsherpa-onnx-cxx-api.so        ← 282,000 字节
└── （其余为 alsa / microphone / tts 等未使用的可执行文件）
```

**关键**：这是 `shared` 构建，`.so` 必须和可执行文件放在同一目录，并通过 `LD_LIBRARY_PATH` 指向该目录，否则启动即报找不到库。

---

## 二、模型文件

| 项目 | 事实 |
|---|---|
| 目录 | `/root/.picoclaw-asr/models/sherpa-onnx-paraformer-zh-small-2024-03-09/` |
| 模型 | `paraformer-zh-small`（FunASR Paraformer，**int8 量化**） |
| 目录总大小 | 80 MB |
| 主模型文件 | `model.int8.onnx` — 81,828,675 字节 |
| 词表 | `tokens.txt` — 75,352 字节 |

### 目录结构（`ls -la` 原样）

```
drwx------ 3 root root     4096  .
drwx------ 4 root root     4096  ..
-rwx------ 1 root root      152  README.md
-rwx------ 1 root root     2542  add-model-metadata.py
-rwx------ 1 root root    11203  am.mvn
-rwx------ 1 root root    56290  config.yaml
-rwx------ 1 root root      491  generate-tokens.py
-rwx------ 1 root root 81828675  model.int8.onnx
drwx------ 2 root root     4096  test_wavs
-rwx------ 1 root root    75352  tokens.txt
```

> 注意：`am.mvn`、`config.yaml`、`*.py` 是模型包自带的辅助文件，**转写时不需要**。实际只用 `model.int8.onnx` 和 `tokens.txt`。

### 模型来源

- 资产名：`sherpa-onnx-paraformer-zh-small-2024-03-09.tar.bz2`
- 地址：`https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-paraformer-zh-small-2024-03-09.tar.bz2`

---

## 三、调用方式

### 完整命令行

```bash
LD_LIBRARY_PATH=/root/.picoclaw-asr/bin \
  /root/.picoclaw-asr/bin/sherpa-onnx-offline \
  --tokens=/root/.picoclaw-asr/models/sherpa-onnx-paraformer-zh-small-2024-03-09/tokens.txt \
  --paraformer=/root/.picoclaw-asr/models/sherpa-onnx-paraformer-zh-small-2024-03-09/model.int8.onnx \
  --num-threads=2 \
  --decoding-method=greedy_search \
  /path/to/input.wav
```

### 参数说明

| 参数 | 说明 |
|---|---|
| `--tokens=` | 词表路径，**必填** |
| `--paraformer=` | Paraformer 模型路径，**必填** |
| `--num-threads=` | CPU 线程数。该机器实测用 2 |
| `--decoding-method=` | `greedy_search` |
| 最后一个位置参数 | 输入音频路径（可以传多个） |

### 输入音频格式

- **必须是 WAV（RIFF 容器）**，PCM 16-bit 小端
- 采样率 **16000 Hz**，**单声道**
- 实测 `test_wavs/0.wav`：`codec_name=pcm_s16le, sample_rate=16000, channels=1`

> **重要**：sherpa-onnx 的读取器只认 RIFF。传入 Ogg/Opus（Telegram 语音的 `.oga`）会直接失败，报错为：
> `Expected chunk_id RIFF. Given: 0x5367674f`
> （`0x5367674f` 是 ASCII 的 `OggS`）
>
> 解决办法：先用 ffmpeg 转码
> ```bash
> ffmpeg -y -i input.oga -ar 16000 -ac 1 -c:a pcm_s16le output.wav
> ```
> 该机器已安装 ffmpeg 7.1.5（`/usr/bin/ffmpeg`）。

### 输出格式

**不是** `0: 文本` 那种逐行格式。实际是 **stdout 上一行 JSON**：

```json
{"lang": "", "emotion": "", "event": "", "text": "对我做了介绍啊那么我想说的是呢大家如果对我的研究感兴趣呢嗯", "timestamps": [], "durations": [], "tokens":["对","我","做",...], "ys_log_probs": [], "words": []}
```

- 取 **`.text`** 字段即可
- 模型加载日志、进度信息走 **stderr**，不会污染 stdout
- 一次调用传多个 wav 时会输出多行 JSON，需要逐行解析

### 实测性能（该机器）

| 指标 | 数值 |
|---|---|
| 5.6 秒音频 → 转写耗时 | 约 10 秒（两实例并发时 RTF ≈ 1.7） |
| 单实例空闲时 | 约 1.0 秒（RTF ≈ 0.18） |
| 峰值内存（RSS） | 约 179 MB |
| 冷启动（含模型加载） | 约 10 秒 |

> 每次调用都会重新加载模型，进程退出即释放。这是命令行模式的固有开销。

---

## 四、运行环境

| 项目 | 事实 |
|---|---|
| `uname -a` | `Linux localhost 4.9.227-perf+ #1 SMP PREEMPT Fri Jul 11 12:18:43 CST 2025 aarch64 GNU/Linux` |
| `/proc/version` | `Linux version 5.15.148-android13-8-... aarch64 Android` |
| 架构 | **aarch64（ARM64）** |
| 发行版 | `Debian GNU/Linux trixie/sid`（`/etc/os-release`） |
| 容器 | **proot** 容器内（宿主为 Android 13，Termux 启动） |
| 用户 | `uid=0(root) gid=0(root)` |
| 模型目录权限 | `drwx------ root root`，root 可读 |

> 注意：`uname -a` 报 4.9.227，`/proc/version` 报 5.15.148-android13 —— 这是 proot 的常见表现（`uname` 被 proot 拦截返回伪装值，`/proc/version` 是真实内核）。**真实宿主内核是 5.15.148-android13**。

### 关于「是否 Docker」

不是 Docker。是 Termux 里的 proot 容器跑 Debian。没有容器编排、没有挂载配置，就是普通文件系统。

---

## 五、其他信息

### 配置文件位置

**没有** `.env` 或 `config.yaml` 记录这些路径。路径硬编码在程序里：

- 代码位置：`web/backend/api/asr_local.go`
- 常量：`const sherpaRelease = "v2.0.1"`
- 目录解析：`GlobalASRDir()` → 优先 `$PICOCLAW_ASR_DIR`，否则 `~/.picoclaw-asr`

### 启动方式

**没有 systemd**。由 PicoClaw 自己的进程管理：

- `/opt/tg-bot-api/picoclaw-watchdog.sh` —— 每 20 秒探测 18790，挂了就用 `PICOCLAW_HOME=/root/.picoclaw` 拉起
- 另一个实例（`.picoclaw-upstream`）手动 `setsid` 拉起，无 watchdog

### 项目信息

| 项目 | 值 |
|---|---|
| 名称 | PicoClaw（`picoclaw-edition` fork） |
| 路径 | `/root/picoclaw-edition` |
| 二进制 | `/root/picoclaw-edition/picoclaw`（v0.7.2-edition） |
| 仓库 | `https://github.com/midi-HL/picoclaw-edition` |

### 已有实例（供参考，勿干扰）

| 实例 | 配置目录 | gateway 端口 | launcher 端口 |
|---|---|---|---|
| A（正式） | `/root/.picoclaw` | 18790 | 18800 |
| B（上游） | `/root/.picoclaw-upstream` | — | 18801 |

两个实例**共用同一份** ASR 运行时与模型（`/root/.picoclaw-asr/`，154MB）。

---

## 六、如果你要复用这套部署

### 最小依赖清单

复用需要**三样东西**，缺一不可：

1. **可执行文件**：`sherpa-onnx-offline`（ARM64 Linux）
2. **共享库**：`libonnxruntime.so` + `libsherpa-onnx-c-api.so` + `libsherpa-onnx-cxx-api.so`（必须同目录）
3. **模型**：`model.int8.onnx` + `tokens.txt`

### 两种复用方式

**方式 A：直接指向已有目录**

如果你的程序能读到 `/root/.picoclaw-asr/`，直接配置：

```go
binaryPath := "/root/.picoclaw-asr/bin/sherpa-onnx-offline"
modelDir   := "/root/.picoclaw-asr/models/sherpa-onnx-paraformer-zh-small-2024-03-09"
```

调用时**必须**设置 `LD_LIBRARY_PATH`：

```go
cmd := exec.CommandContext(ctx, binaryPath, args...)
cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH="+filepath.Dir(binaryPath))
```

**方式 B：独立安装一份**

按上面的 URL 下载 `linux-aarch64-shared-cpu` 包和模型包，解压到自己的目录。约 100MB 磁盘。

> 注意：两个程序**可以共用同一份**文件（只读），只要都设好 `LD_LIBRARY_PATH`。没有端口冲突，没有锁文件。

### 容易踩的三个坑

1. **忘了 `LD_LIBRARY_PATH`** → `error while loading shared libraries: libsherpa-onnx-c-api.so`
2. **传 Ogg/Opus 给 CLI** → `Expected chunk_id RIFF. Given: 0x5367674f`（需先 ffmpeg 转 WAV）
3. **从 stdout 里正则抠文本** → 应该按行 JSON 解析，取 `.text`

---

## 附：命令原始输出

### `ls -la /root/.picoclaw-asr/bin/`（节选）

```
-rwxr-xr-x 1 root root 34606912  libonnxruntime.so
-rwxr-xr-x 1 root root  4443416  libsherpa-onnx-c-api.so
-rwxr-xr-x 1 root root   282000  libsherpa-onnx-cxx-api.so
-rwxr-xr-x 1 root root  1775160  sherpa-onnx-offline
```

### 真实转写输出（`test_wavs/0.wav`）

```
{"lang": "", "emotion": "", "event": "", "text": "对我做了介绍啊那么我想说的是呢大家如果对我的研究感兴趣呢嗯", "timestamps": [], "durations": [], "tokens":["对","我","做","了","介","绍","啊","那","么","我","想","说","的","是","呢","大","家","如","果","对","我","的","研","究","感","兴","趣","呢","嗯"], "ys_log_probs": [], "words": []}
```

### `ffprobe` 音频信息

```
codec_name=pcm_s16le
sample_rate=16000
channels=1
```
