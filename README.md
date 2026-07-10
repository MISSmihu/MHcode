# MHcode

MHcode 是一个面向开发者的 AI 协议交换台。首发目标是打透 DeepSeek 官方接入，并把 Skills、MCP、工具调用、缓存命中、模型路由和费用观测做成核心能力。

## 技术路线

- 桌面壳：Wails v2 稳定线，后续观察 Wails v3。
- 核心引擎：Go。
- 前端界面：SolidJS + TypeScript + Vite。
- 本地存储：SQLite。
- 本地密钥：系统 Keychain / Credential Manager。
- 进程通信：Wails Go binding，必要时补本地 WebSocket / JSON-RPC。
- 首发模型协议：DeepSeek 官方 API。
- 扩展协议：OpenAI Compatible、Anthropic、Gemini、Azure OpenAI、Ollama、LM Studio、vLLM、国内模型平台和自定义 HTTP/SSE/WebSocket。

## 第一阶段目标

1. 建立 Wails + Go + SolidJS 桌面骨架。
2. 实现 DeepSeek 官方接入：密钥、模型探测、流式输出、错误翻译。
3. 实现推理强度菜单：低 / 中 / 高 / 超高。
4. 实现 Skills 加载和 MCP schema 快照。
5. 实现稳定前缀与易变尾部，缓存命中率目标 `96%+`。
6. 实现 tokens、缓存命中率、单轮费用和会话费用统计。

## 文档

- [开发计划](docs/development-plan.zh-CN.md)
- [技术栈与开发工具链](docs/technical-stack.zh-CN.md)
- [Skills 开发文档](docs/skills-development.zh-CN.md)
