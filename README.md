# Nozomi (希)

[中文文档 (Chinese Version)](./README_ZH.md)

Nozomi is a high-performance, engineering-focused AI Bot framework built with **Go**. It bridges the **Matrix** communication protocol with **Google Gemini** LLMs. Designed for stability and long-term interaction, Nozomi features advanced memory management, multi-modal perception, and industrial-grade monitoring.

## 🌟 Key Features

- **🧠 Memory Retrospection**: Bypasses LLM context limits. When conversation history reaches a threshold, the bot asynchronously generates a summary and folds it back into the context, enabling "infinite" long-term memory.
- **🖼️ Multi-modal Perception**: Supports mixed image and text contexts. Includes specialized logic for private chat image caching and description merging.
- **⚙️ Thinking Mode**: Full support for Gemini "Flash Thinking" models, allowing the bot to process complex reasoning before delivering the final response.
- **🌐 Grounding (Search)**: Integrated Google Search tool. Automatically fetches real-time information when necessary based on search quotas.
- **💰 Precise Billing & Rate Limiting**: 
  - Tracks Token usage across Day/Month/Year dimensions (categorized by Input, Output, and Thinking tokens).
  - Monthly quota management for internet searches.
  - Token-bucket-based rate limiting to prevent API abuse.
- **🛡️ Production Ready**:
  - Fully asynchronous I/O; Matrix sync is never blocked.
  - Thread-safe core logic with granular mutex locking.
  - Real-time error reporting to dedicated Matrix log rooms.

## 🏗️ Architecture

The project follows a decoupled domain-driven design:

- `internal/matrix`: Manages protocol events, state synchronization, and Markdown rendering.
- `internal/llm`: Handles SDK interactions, thinking process extraction, and tool configuration.
- `internal/memory`: Orchestrates the sliding window history and asynchronous summarization.
- `internal/billing`: Manages persistent usage statistics and alarm triggers.
- `internal/handler`: The central router coordinating domain logic and event flow.

## 🚀 Quick Start

1. **Configure**: Fill in your credentials in `configs/config.yaml`.
2. **Build**: 
   ```bash
   make build
   ```
3. **Run**:
   ```bash
   ./dist/nozomi
   ```

## 🔧 Configuration Guide

### `CLIENT` Section
| Key | Description |
|:---|:---|
| `homeserverURL` | The URL of your Matrix homeserver (e.g., `https://matrix.org`). |
| `userID` | The full Matrix ID of the bot (e.g., `@nozomi:example.com`). |
| `accessToken` | The bot's Matrix access token for authentication. |
| `deviceID` | An identifier for the session device. |
| `logRoom` | An array of room IDs where the bot will send error logs and status updates. |
| `maxMemoryLength` | The maximum number of message "slots" in the sliding window before triggering summarization. |
| `whenRetroRemainMemLen` | How many recent message slots to keep intact after a summarization occurs. |
| `avatarURL` | The MXC URI for the bot's avatar. |
| `displayName` | The display name shown in Matrix rooms. |
| `databasePassword` | Password for the SQLite/State database encryption (if applicable). |

### `MODEL` Section
| Key | Description |
|:---|:---|
| `API_KEY` | Your Google AI Studio (Gemini) API Key. |
| `model` | The specific model string (e.g., `gemini-2.0-flash-thinking-exp`). |
| `prefixToCall` | The keyword prefix required to trigger the bot in group chats (e.g., `!c`). |
| `maxOutputToken` | Maximum number of tokens allowed in a single model response. |
| `alargmTokenCount` | Threshold for a "Dosage Alert". Single requests exceeding this will be logged as high-consumption. |
| `useInternet` | `true/false`. Enables or disables the Google Search tool. |
| `secureCheck` | `true/false`. If false, safety filters are set to `BLOCK_NONE` for unrestricted output. |
| `maxMonthlySearch` | Monthly limit for internet search tool calls. |
| `timeOutWhen` | Strict timeout for the LLM API call (e.g., `40s`). |
| `includeThoughts` | `true/false`. Whether to request and process the "Thinking" process from the model. |
| `thinkingBudget` | Token budget for the thinking process (0 for auto). |
| `thinkingLevel` | Specific reasoning depth level if supported by the model. |
| `rate` | The sustained request rate allowed per user (requests per second). |
| `rateBurst` | The maximum number of requests a user can send in a sudden burst. |

---
*Inspired by Nozomi from "Sonny Boy".*
