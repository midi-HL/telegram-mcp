# 转发给另一个会话的 AI — 部署信息采集提示词

你好。我在开发一个 Telegram MCP 服务（Go 语言），需要适配你之前部署的 sherpa-onnx 语音识别环境。请帮我调查并**如实汇报**以下信息。

## 背景

我的项目需要调用 sherpa-onnx 做离线语音转写。我打算支持「复用已有部署」而不是让用户重新下载 74MB 模型。为此需要知道你那边的实际部署情况。

## 请调查并回答以下问题

### 一、sherpa-onnx CLI 可执行文件

1. 可执行文件的**完整绝对路径**是什么？
   - 例如 `/usr/local/bin/sherpa-onnx-offline` 或 `C:\sherpa\sherpa-onnx-non-streaming-asr-x64.exe`
2. 它是哪个版本？（运行 `<路径> --version` 或看文件名里的版本号）
3. 它是**官方发布版**还是自己编译的？
4. 如果是自己编译的，编译时的参数是什么？

### 二、模型文件

1. 模型存放在**哪个目录**？（完整绝对路径）
2. 该目录下的**文件列表**（`ls -la` 输出）
3. 具体是哪个模型？
   - `paraformer-zh-small` / `paraformer-zh` / `zipformer` / `whisper` / 其他
4. 模型的**总大小**是多少？
5. 模型目录结构是怎样的？例如：
   ```
   /opt/models/paraformer-zh-small/
   ├── model.onnx
   ├── tokens.txt
   └── ...
   ```

### 三、调用方式

1. 你之前是怎么调用它做转写的？**完整的命令行**是什么？
   ```
   例如：sherpa-onnx-offline --tokens=/path/tokens.txt --paraformer=/path/model.onnx --model-type=paraformer input.wav
   ```
2. 输入音频要求什么格式？（wav / ogg / mp3？采样率？单声道？）
3. 输出格式是什么样的？例如：
   ```
   0: 你好世界
   1: 第二句话
   ```
   还是直接输出纯文本？

### 四、运行环境

1. 操作系统和架构（`uname -a`）
2. 是原生系统还是容器（Docker/proot/Termux）？
3. 如果是 Docker，容器的挂载配置是什么？
4. 运行用户是谁？有权限读模型目录吗？

### 五、其他相关信息

1. 有没有**配置文件**（如 `.env`、`config.yaml`）记录了这些路径？
2. 有没有**启动脚本**或 systemd service？
3. 你之前那个项目叫什么名字？在哪个路径？

## 输出格式要求

请按以下格式回答，方便我直接读取：

```
CLI_PATH=/绝对路径/到/sherpa-onnx
CLI_VERSION=版本号
MODEL_DIR=/绝对路径/到/模型目录
MODEL_TYPE=paraformer-zh-small
MODEL_FILES=model.onnx,tokens.txt,...
MODEL_SIZE=74MB
INVOKE_CMD=完整命令行
INPUT_FORMAT=wav 16kHz mono
OUTPUT_FORMAT=0: 文本 这种格式
OS=Linux x86_64
ENV=原生/Docker/proot/Termux
PROJECT_NAME=项目名
PROJECT_PATH=项目路径
NOTES=其他需要说明的
```

**如果某项查不到，写 `UNKNOWN`，不要猜测。**

## 特别说明

- 我需要的是**事实**，不是建议。
- 如果路径不存在或命令报错，如实告诉我错误信息。
- 如果你不确定某个值，说「不确定」比编一个数字好。
