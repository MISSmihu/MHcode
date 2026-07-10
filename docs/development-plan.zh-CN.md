# MHcode 开发计划

本文档是 MHcode 的阶段性开发计划。当前产品定位是：**面向开发者的 AI 协议交换台**。第一目标不是铺满所有模型，而是先把 DeepSeek 官方接入、`mhcode-agent-core`、Skills/MCP、工具调用、缓存命中和费用观测打成闭环。

## 北极星目标

MHcode 要解决三件事：

1. 集成多家 AI 协议，让开发者不被单一供应商锁死。
2. 用 Skills、MCP、工具调用和稳定前缀把缓存命中率推到 `96%+`。
3. 让 tokens、缓存命中、费用、模型路由和推理强度可见、可控、可解释。

## MVP 验收标准

MVP 完成时必须能做到：

- 桌面应用可启动。
- DeepSeek API Key 可保存、测试、删除。
- 可探测 DeepSeek 模型。
- 可发起一次流式对话。
- 可选择推理强度：`低 / 中 / 高 / 超高`。
- 可加载 `mhcode-agent-core`。
- 可生成 Skills 稳定索引。
- 可生成 MCP schema 快照。
- 可把上下文拆成稳定前缀和易变尾部。
- 可记录 `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens`。
- 可显示缓存命中率、tokens 和费用。

## 阶段 0：项目基线

目标：让工程能稳定开发、构建和验证。

任务：

- 确认 Wails v2 + Go + SolidJS + Vite 项目骨架。
- 固定目录结构：`internal/*`、`frontend/src/*`、`skills/*`、`docs/*`。
- 建立基础 CI 脚本或本地验证脚本。
- 确认 Windows 下 `bun.cmd`、`go test ./...`、`wails dev` 可运行。

验收：

- `go test ./...` 通过。
- `cd frontend && bun.cmd run build` 通过。
- `mhcode-agent-core` 通过 `quick_validate.py`。

## 阶段 1：DeepSeek 官方接入

目标：先把一个官方协议做深。

任务：

- 实现 `internal/protocol.Provider` 接口。
- 实现 `DeepSeekProvider`。
- 支持 API Key 配置。
- 支持连接测试。
- 支持模型探测。
- 支持流式响应。
- 支持 JSON 输出能力标记。
- 支持工具调用能力标记。
- 支持错误翻译。

建议接口：

```go
type Provider interface {
  Name() string
  ListModels(ctx context.Context) ([]Model, error)
  Stream(ctx context.Context, request ChatRequest) (<-chan StreamEvent, error)
}
```

验收：

- 输入 DeepSeek API Key 后可以测试连接。
- 能看到可用模型。
- 能发起一次流式请求。
- 断网、Key 错误、余额不足、限流时有中文错误提示。

## 阶段 2：推理强度与模型路由

目标：实现截图中的“推理：低 / 中 / 高 / 超高”，并让它真正影响 Agent 执行。

任务：

- 前端实现推理菜单。
- 后端定义 `ReasoningLevel`。
- 将四档映射到工具调用上限、上下文策略、缓存策略和规划器开关。
- 状态栏显示当前推理强度。
- 任务运行中切换显示“下一轮生效”。

验收：

- 菜单包含 `低 / 中 / 高 / 超高`。
- 当前项右侧显示对勾。
- 默认可设为 `超高`。
- 切换后下一轮请求使用新的预算策略。

## 阶段 3：mhcode-agent-core

目标：把 Agent 逻辑统一到一个核心 Skill，避免多个相似 Skill 抢触发。

任务：

- 只保留 `skills/mhcode-agent-core`。
- 加载 Skill frontmatter。
- 生成稳定 Skills 索引。
- 按需加载完整 `SKILL.md`。
- 记录 Skill hash。
- 前端展示当前 Skill 索引。

验收：

- Codex 和 MHcode 都只看到 `mhcode-agent-core`。
- 旧 Skill 名称 `mhcode-cache-optimizer`、`mhcode-reasoning-control` 不再出现。
- Skill 索引稳定、可复现。

## 阶段 4：MCP 工具系统

目标：把 MCP 工具做成缓存友好的稳定快照。

任务：

- 定义 MCP server 配置结构。
- 扫描工具列表。
- 生成 `tools_hash`。
- 为每个工具生成 `input_schema_hash`。
- 定义工具输出策略：`summary-first`。
- 原始工具结果保存为本地引用。

快照示例：

```json
{
  "server": "filesystem",
  "tools_hash": "sha256:...",
  "tools": [
    {
      "name": "read_file",
      "input_schema_hash": "sha256:...",
      "output_policy": "summary-first"
    }
  ]
}
```

验收：

- 工具 schema 不变时 hash 不变。
- 工具调用结果默认摘要化。
- 长日志、长 JSON、长 diff 不直接进入模型上下文。

## 阶段 5：缓存命中引擎

目标：把 96%+ 命中率做成架构约束。

任务：

- 实现稳定前缀构建器。
- 实现易变尾部构建器。
- 记录 prompt hash。
- 记录 DeepSeek usage 字段。
- 计算 `cache_hit_rate`。
- 命中率低于 96% 时给出诊断。

稳定前缀：

- MHcode 系统身份。
- Agent 规则。
- 当前推理强度。
- Skills 索引。
- MCP schema 快照。
- 项目摘要。
- 路由和预算策略。

易变尾部：

- 用户本轮输入。
- 工具调用参数。
- 工具结果摘要。
- 最近 diff。
- 输出要求。

验收：

- UI 能显示命中率。
- UI 能区分 hit tokens 和 miss tokens。
- 命中率低于 96% 时能提示原因。
- 切换推理强度不重排稳定前缀。

## 阶段 6：费用与用量观测

目标：让用户知道每一分钱花在哪里。

任务：

- 记录单轮 tokens。
- 记录会话 tokens。
- 记录缓存命中 tokens。
- 记录缓存未命中 tokens。
- 估算单轮费用。
- 估算会话费用。
- 增加项目级预算墙。

验收：

- 底部状态栏显示 tokens、命中率、费用。
- 右侧面板显示会话汇总。
- 预算超限时阻止或提示用户确认。

## 阶段 7：协议扩展

目标：DeepSeek 跑通后扩展更多供应商。

优先级：

1. OpenAI Compatible。
2. Anthropic。
3. Gemini。
4. Azure OpenAI。
5. Ollama / LM Studio / vLLM。
6. 国内平台：通义、智谱、Kimi、火山、千帆、混元、星火。
7. 聚合平台：OpenRouter、硅基流动、Together、Groq。
8. 自定义 HTTP / SSE / WebSocket。

验收：

- 每个 provider 都实现统一接口。
- UI 可以切换供应商。
- 失败时可以走故障转移。

## 阶段 8：桌面体验

目标：形成真正可用的桌面产品。

任务：

- 密钥库页面。
- 模型路由页面。
- Skills 页面。
- MCP 页面。
- 缓存命中面板。
- 费用面板。
- 日志导出。
- 设置导入/导出。

验收：

- Windows 可日常使用。
- 设置持久化。
- 错误可诊断。
- 不需要用户看命令行才能完成基础配置。

## 风险与对策

风险：过早接太多模型。

对策：DeepSeek 先做深，一个 provider 跑通后再复制。

风险：缓存命中率只是 UI 指标。

对策：把稳定前缀顺序、MCP schema hash、工具结果摘要写进核心执行流程。

风险：工具调用输出吞掉 tokens。

对策：默认 summary-first，原文只保存引用。

风险：推理强度只是菜单。

对策：四档必须影响工具调用上限、上下文策略、规划器和缓存策略。

## 当前下一步

立即做：

1. 确认 `mhcode-agent-core` 是唯一内置 Skill。
2. 实现 DeepSeek API Key 配置和连接测试。
3. 实现 DeepSeek 流式响应。
4. 把推理强度接到请求预算。
5. 记录 `prompt_cache_hit_tokens` / `prompt_cache_miss_tokens`。

完成这五项后，MHcode 就有第一条真正的产品主线。

