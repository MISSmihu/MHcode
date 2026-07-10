# MHcode 技术栈与开发工具链

本文档记录 MHcode 的技术选型、项目结构、开发命令和第一阶段落地路线。结论：使用 **Go 核心引擎 + Wails 桌面壳 + SolidJS 前端 + SQLite 本地存储**。

## 选型结论

### Go

Go 负责 MHcode 的核心能力：

- Agent 调度。
- Skills 加载。
- MCP 工具目录和工具调用。
- AI 协议适配器。
- DeepSeek 官方接入。
- 流式响应处理。
- 稳定前缀和易变尾部组装。
- 缓存命中率和费用统计。
- SQLite 本地存储。
- 本地密钥和配置管理。

Go 的优势是单文件分发、并发稳定、跨平台简单、适合长驻本地核心进程。

### Wails

桌面壳首版使用 Wails v2 稳定线。Wails 的价值是把 Go 后端和 Web 前端打包成桌面应用，比 Electron 更轻，同时保留前端开发效率。

Wails v3 官方文档目前仍标注 Alpha，可持续观察。等 v3 稳定或 MHcode 需要多窗口、系统托盘等能力时再评估升级。

### SolidJS

前端使用 SolidJS + TypeScript + Vite。

适合 MHcode 的原因：

- 响应式细粒度，适合高频状态面板。
- 体积小。
- 与 Vite 组合开发速度快。
- UI 可以从当前 HTML 原型平滑迁移。

### SQLite

本地存储使用 SQLite。

建议存储：

- 模型供应商配置。
- API Key 引用和密钥元数据。
- 会话记录。
- tokens 用量。
- `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens`。
- 工具调用摘要。
- MCP schema 快照。
- Skills 索引和版本。

Go SQLite 驱动优先选择免 CGO 方案，方便 Windows/macOS/Linux 分发。

## 推荐项目结构

```text
MHcode
├── cmd
│   └── mhcode
│       └── main.go
├── internal
│   ├── agent
│   │   ├── runner.go
│   │   ├── context_builder.go
│   │   └── reasoning.go
│   ├── cache
│   │   ├── prefix.go
│   │   └── metrics.go
│   ├── config
│   │   └── config.go
│   ├── mcp
│   │   ├── registry.go
│   │   ├── schema_snapshot.go
│   │   └── tool_result.go
│   ├── protocol
│   │   ├── provider.go
│   │   ├── deepseek.go
│   │   ├── openai_compatible.go
│   │   └── local.go
│   ├── skills
│   │   ├── loader.go
│   │   └── index.go
│   ├── storage
│   │   ├── db.go
│   │   └── migrations.go
│   └── vault
│       └── keyring.go
├── frontend
│   ├── src
│   │   ├── app.tsx
│   │   ├── components
│   │   ├── pages
│   │   └── state
│   ├── package.json
│   └── vite.config.ts
├── skills
├── docs
└── wails.json
```

## 核心模块边界

### `internal/protocol`

定义统一模型协议接口。

```go
type Provider interface {
  Name() string
  ListModels(ctx context.Context) ([]Model, error)
  Stream(ctx context.Context, request ChatRequest) (<-chan StreamEvent, error)
}
```

首发实现：

- `DeepSeekProvider`
- `OpenAICompatibleProvider`
- `LocalProvider`

后续扩展：

- Anthropic
- Gemini
- Azure OpenAI
- Ollama
- LM Studio
- vLLM
- 国内模型平台
- 自定义 HTTP / SSE / WebSocket

### `internal/agent`

负责 Agent 执行流程。

职责：

- 接收用户任务。
- 选择推理强度。
- 调用 Skills。
- 查询 MCP 工具。
- 构建上下文。
- 发起模型请求。
- 消费工具调用。
- 写入会话和成本记录。

### `internal/cache`

负责缓存命中策略。

职责：

- 稳定前缀组装。
- 易变尾部组装。
- 前缀 hash。
- tokens 命中率统计。
- 费用估算。
- 命中率低于 96% 时给出诊断。

### `internal/skills`

负责 Skills 发现和加载。

职责：

- 扫描 `skills/*/SKILL.md`。
- 解析 frontmatter。
- 生成稳定 Skills 索引。
- 只在触发时加载完整 Skill。
- 记录 Skill 版本和 hash。

### `internal/mcp`

负责 MCP 工具接入。

职责：

- 管理 MCP server。
- 生成工具 schema 快照。
- 计算 schema hash。
- 规范化工具调用参数。
- 压缩工具结果。
- 保存原始工具结果引用。

### `internal/vault`

负责密钥。

职责：

- API Key 不直接写入普通配置文件。
- Windows 使用 Credential Manager。
- macOS 使用 Keychain。
- Linux 使用 Secret Service 或加密文件兜底。

## 推理强度

MHcode 必须支持四档推理强度：

```ts
type ReasoningLevel = "low" | "medium" | "high" | "ultra"
```

中文菜单：

```text
推理
低
中
高
超高  ✓
```

默认值：

```ts
const defaultReasoningLevel = "ultra"
```

后端映射：

```go
type ReasoningLevel string

const (
  ReasoningLow    ReasoningLevel = "low"
  ReasoningMedium ReasoningLevel = "medium"
  ReasoningHigh   ReasoningLevel = "high"
  ReasoningUltra  ReasoningLevel = "ultra"
)
```

四档不只影响模型，也影响工具调用上限、上下文大小、是否启用规划器和缓存策略。

## 缓存命中率

缓存命中率是 MHcode 的核心指标。

目标：

```text
cache_hit_rate >= 96%
```

公式：

```go
cacheHitRate := promptCacheHitTokens / (promptCacheHitTokens + promptCacheMissTokens)
```

请求上下文必须分成：

- 稳定前缀：系统提示、Agent 规则、推理强度、Skills 索引、MCP schema 快照、项目摘要、路由策略。
- 易变尾部：用户输入、工具参数、工具结果摘要、最近 diff、错误和重试信息。

禁止为了“超高推理”破坏稳定前缀。

## DeepSeek 首发接入

第一阶段只把 DeepSeek 做深，不急着铺满所有供应商。

必须完成：

- API Key 保存和连接测试。
- 官方模型探测。
- 流式请求。
- JSON 输出。
- 工具调用兼容。
- tokens 统计。
- `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens` 统计。
- 费用估算。
- 错误翻译。

## 前端页面

首批页面：

- 协议总览。
- DeepSeek 官方接入。
- 模型路由。
- 推理强度菜单。
- Skills 管理。
- MCP 工具管理。
- 上下文窗口。
- 缓存命中面板。
- tokens 与费用面板。
- 设置和密钥库。

## 开发命令

建议命令：

```powershell
# 安装前端依赖
cd C:\Users\Administrator\Desktop\MHcode\frontend
bun install

# 前端开发
bun run dev

# Go 测试
cd C:\Users\Administrator\Desktop\MHcode
go test ./...

# Wails 开发
wails dev

# Wails 打包
wails build
```

Windows 上调用 Bun 时，优先使用 `bun.cmd`。

## 第一阶段路线

1. 初始化 Wails + SolidJS 项目。
2. 建立 Go 模块和目录结构。
3. 实现 `ReasoningLevel` 前后端类型。
4. 实现推理菜单 UI。
5. 实现 Skills 扫描和索引。
6. 实现 MCP schema 快照。
7. 实现 DeepSeek provider。
8. 实现流式输出。
9. 实现缓存命中率统计。
10. 实现费用面板。

## 取舍

不建议一开始全量重写复杂 Agent 系统。先打穿最小闭环：

```text
DeepSeek API Key → 模型探测 → 推理强度 → 上下文组装 → 流式输出 → 缓存命中统计 → 费用展示
```

闭环稳定后，再扩展更多供应商和插件市场。

