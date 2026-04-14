# Nozomi (希)

[English Version](./README.md)

Nozomi 是一个基于 **Go** 语言开发的高性能、工程化 AI 机器人框架。它深度整合了 **Matrix** 通讯协议与 **Google Gemini** 大语言模型，旨在提供稳定、长期的智能交互体验。Nozomi 具备先进的记忆管理、多模态感知以及工业级的监控能力。

## 🌟 核心特性

- **🧠 记忆回传算法 (Memory Retrospection)**: 突破 LLM 的上下文限制。当对话历史达到设定阈值时，机器人会异步生成摘要并将其回传至上下文，从而实现“无限”的长期记忆。
- **🖼️ 多模态感知**: 支持图片与文字混合的复杂上下文理解，具备私聊图片暂存与配文合并的特殊处理逻辑。
- **⚙️ 思考模式 (Thinking Mode)**: 完整支持 Gemini "Flash Thinking" 模型，允许机器人在输出最终答案前进行复杂的推理过程。
- **🌐 联网搜索增强 (Grounding)**: 集成 Google 搜索工具。根据搜索配额，在必要时自动获取实时信息。
- **💰 精准账单与限流**: 
  - 支持按 日/月/年 维度统计 Token 消耗（细分为 输入、输出、思考 Token）。
  - 具备每月联网搜索次数配额管理。
  - 基于令牌桶算法的限流机制，有效防止 API 滥用。
- **🛡️ 生产级稳定性**:
  - 全异步 I/O 设计，确保 Matrix 消息同步永不阻塞。
  - 核心逻辑线程安全，采用细粒度的互斥锁控制。
  - 实时错误报告，关键异常自动推送至指定的 Matrix 日志房间。

## 🏗️ 技术架构

项目采用解耦的领域驱动设计：

- `internal/matrix`: 管理协议事件、状态同步及 Markdown 渲染。
- `internal/llm`: 处理 SDK 交互、思考过程提取及工具配置。
- `internal/memory`: 调度滑动窗口记忆与异步总结逻辑。
- `internal/billing`: 负责持久化的使用量统计与报警触发。
- `internal/handler`: 中心路由器，协调各领域逻辑与事件流。

## 🚀 快速开始

1. **配置**: 在 `configs/config.yaml` 中填入你的凭据。
2. **编译**: 
   ```bash
   make build
   ```
3. **运行**:
   ```bash
   ./dist/nozomi
   ```

## 🔧 配置指南

### `CLIENT` 部分 (客户端配置)
| 配置项 | 说明 |
|:---|:---|
| `homeserverURL` | Matrix 家园服务器地址 (例如 `https://matrix.org`)。 |
| `userID` | 机器人的完整 Matrix ID (例如 `@nozomi:example.com`)。 |
| `accessToken` | 机器人的 Matrix 访问令牌，用于身份验证。 |
| `deviceID` | 当前会话的设备标识符。 |
| `logRoom` | 日志房间 ID 列表，机器人会将错误日志和状态更新发送至此。 |
| `maxMemoryLength` | 滑动窗口中的最大消息“槽位”数，超过此值将触发记忆总结。 |
| `whenRetroRemainMemLen` | 触发记忆总结后，保留在上下文中的最近消息槽位数。 |
| `avatarURL` | 机器人头像的 MXC 链接。 |
| `displayName` | 机器人在 Matrix 房间中显示的昵称。 |
| `databasePassword` | 状态数据库加密密码（如适用）。 |

### `MODEL` 部分 (模型配置)
| 配置项 | 说明 |
|:---|:---|
| `API_KEY` | Google AI Studio (Gemini) 的 API 密钥。 |
| `model` | 使用的具体模型名称 (例如 `gemini-2.0-flash-thinking-exp`)。 |
| `prefixToCall` | 在群聊中触发机器人的关键词前缀 (例如 `!c`)。 |
| `maxOutputToken` | 模型单次回复允许生成的最大 Token 数量。 |
| `alargmTokenCount` | “用量报警”阈值。单次请求消耗超过此值将被记录为高消耗。 |
| `useInternet` | `true/false`。是否启用 Google 搜索工具。 |
| `secureCheck` | `true/false`。若为 false，安全过滤器将设为 `BLOCK_NONE`，允许输出不受限。 |
| `maxMonthlySearch` | 每月允许进行联网搜索的次数上限。 |
| `timeOutWhen` | LLM API 调用的严格超时时间 (例如 `40s`)。 |
| `includeThoughts` | `true/false`。是否请求并处理模型的“思考过程”。 |
| `thinkingBudget` | 思考过程的 Token 预算（设为 0 则为自动）。 |
| `thinkingLevel` | 特定模型支持的推理深度等级。 |
| `rate` | 每秒允许单个用户发送的持续请求数（限流）。 |
| `rateBurst` | 允许用户发送的突发请求最大数量。 |

---
*致敬《漂流少年》(Sonny Boy) 中的“希”。*
