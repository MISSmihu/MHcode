# MHcode Skills 开发文档

本文档定义 MHcode 的 Skills 编写规范、Agent 缓存命中策略、MCP 工具接入规则，以及“推理：低 / 中 / 高 / 超高”功能规格。目标是让 MHcode 成为支持万家 AI 协议的开发者工具，同时把 DeepSeek 等模型的前缀缓存命中率稳定推到 `96%+`，真正降低 tokens 成本。

技术栈、模块划分和开发命令见 [技术栈与开发工具链](technical-stack.zh-CN.md)。

## 目录结构

建议项目结构：

```text
C:\Users\Administrator\Desktop\MHcode
├── docs
│   └── skills-development.zh-CN.md
└── skills
    └── mhcode-agent-core
        ├── SKILL.md
        └── agents
            └── openai.yaml
```

每个 Skill 至少包含：

- `SKILL.md`：Agent 读取的核心中文指令。
- `agents/openai.yaml`：UI 展示名称、简介和默认提示词。

不要在 Skill 目录里堆 README、安装文档、变更日志。详细背景放到 `docs/`，真正给 Agent 执行用的内容放进 `SKILL.md`。

## SKILL.md 规范

每个 `SKILL.md` 必须包含 YAML frontmatter：

```markdown
---
name: mhcode-agent-core
description: 统一管理 MHcode Agent 的推理强度、Skills 加载、MCP schema 快照、工具调用、上下文组装、DeepSeek 前缀缓存命中率、tokens 成本和模型路由。用于设计或修改 MHcode Agent 执行流程、实现“推理：低/中/高/超高”菜单、优化 prompt_cache_hit_tokens、压缩工具结果、降低 tokens 费用、规划 96% 以上缓存命中率、接入 DeepSeek 官方或其他 AI 协议时。
---
```

要求：

- `name` 使用小写字母、数字和短横线。
- `description` 必须写清楚“做什么”和“什么时候触发”。
- 正文用中文写，使用命令式规则，减少解释性废话。
- 一项 Skill 只解决一个明确边界。MHcode 当前把推理强度、Skills、MCP、工具调用和缓存命中整合为同一个 Agent 核心 Skill，避免多个相近 Skill 互相抢触发。

## MHcode 内置 Skill

### mhcode-agent-core

用途：统一管理 MHcode Agent 的推理强度、Skills 加载、MCP schema 快照、工具调用、上下文组装、缓存命中、成本控制和模型路由。

核心能力：

- 实现 `推理：低 / 中 / 高 / 超高` 菜单。
- 将推理强度映射到工具调用上限、上下文策略、规划器和缓存策略。
- 拆分稳定前缀和易变尾部。
- 固定 Skills 索引、MCP schema、项目摘要。
- 压缩工具调用结果。
- 记录 `prompt_cache_hit_tokens` 和 `prompt_cache_miss_tokens`。
- 将缓存命中率目标设为 `96%+`。
- 选择 DeepSeek 官方、OpenAI 兼容、聚合路由或本地模型兜底。

必须支持四档：

```ts
type ReasoningLevel = "low" | "medium" | "high" | "ultra"
```

中文展示：

```ts
const reasoningLabels = {
  low: "低",
  medium: "中",
  high: "高",
  ultra: "超高",
} as const
```

默认值：

```ts
const defaultReasoningLevel: ReasoningLevel = "ultra"
```

截图里的目标状态是 `推理` 菜单展开，`超高` 被选中并显示对勾。

## 推理强度 UI 规格

菜单：

```text
推理
低
中
高
超高  ✓
```

行为规则：

- 当前选中项右侧显示对勾。
- 点击当前项只关闭菜单，不重复触发任务。
- 切换推理强度后影响下一轮模型请求。
- 如果任务正在运行，显示“下一轮生效”。
- 状态栏显示当前推理强度，例如 `推理 超高`。
- 企业策略可以限制最高档位，例如只允许到 `高`。

建议 UI 数据：

```ts
const reasoningOptions = [
  { id: "low", label: "低", description: "简单问答、轻量编辑、低成本优先" },
  { id: "medium", label: "中", description: "普通代码修改、单文件任务" },
  { id: "high", label: "高", description: "跨文件修改、复杂 bug、测试修复" },
  { id: "ultra", label: "超高", description: "协议设计、Agent 架构、发布级检查" },
] as const
```

## 推理强度执行预算

推理强度不要只影响模型参数，也要影响工具调用、上下文、缓存和预算。

```json
{
  "low": {
    "maxToolCalls": 3,
    "contextPolicy": "minimal",
    "cachePolicy": "reuse-prefix",
    "planner": false
  },
  "medium": {
    "maxToolCalls": 8,
    "contextPolicy": "task-summary",
    "cachePolicy": "reuse-prefix",
    "planner": false
  },
  "high": {
    "maxToolCalls": 16,
    "contextPolicy": "expanded",
    "cachePolicy": "stable-prefix",
    "planner": true
  },
  "ultra": {
    "maxToolCalls": 32,
    "contextPolicy": "full-relevant",
    "cachePolicy": "strict-stable-prefix",
    "planner": true
  }
}
```

## 缓存命中架构

MHcode 的成本优势来自稳定前缀，而不是只来自低模型单价。

稳定前缀：

- 产品身份和系统提示。
- Agent 规则。
- Skills 索引。
- MCP 工具 schema 快照。
- 项目摘要。
- 路由策略。
- 当前推理强度。

易变尾部：

- 用户本轮输入。
- 最近 diff。
- 工具调用参数。
- 工具结果摘要。
- 错误和重试信息。

禁止行为：

- 每轮完整注入所有 Skills。
- 每轮重排 MCP 工具 schema。
- 把长日志、长 JSON、长 diff 原样塞入上下文。
- 把用户输入插入到稳定前缀中间。
- 为了“超高推理”破坏前缀稳定性。

## MCP 工具调用规则

MCP 工具接入必须生成稳定快照：

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

规则：

- 工具列表或 schema 变化时才刷新快照。
- 工具调用参数放到易变尾部。
- 工具结果先摘要，再按需引用原文。
- 原始结果存本地，用 `raw_result_id` 引用。

工具结果摘要格式：

```json
{
  "tool": "shell_command",
  "status": "success",
  "summary": "typecheck 通过，但有 2 个 lint warning",
  "refs": [
    { "kind": "file", "path": "packages/app/src/main.ts", "line": 42 }
  ],
  "raw_result_id": "tool-result:2026-07-07-001"
}
```

## Agent 请求组装顺序

推荐顺序：

1. MHcode 系统身份。
2. 稳定 Agent 规则。
3. 当前推理强度。
4. Skills 索引。
5. MCP schema 快照。
6. 项目摘要。
7. 路由和预算策略。
8. 用户本轮输入。
9. 工具调用摘要。
10. 输出要求。

不要把第 8-10 项插入到第 1-7 项之间。

## DeepSeek 首发接入要求

MHcode 首发优先打透 DeepSeek 官方接入：

- API Key 本地加密保存。
- 官方端点连接探测。
- 模型探测。
- 流式输出。
- 工具调用兼容。
- JSON 输出兼容。
- 缓存命中观测。
- tokens 与费用统计。

缓存统计字段至少记录：

- `prompt_cache_hit_tokens`
- `prompt_cache_miss_tokens`
- `input_tokens`
- `output_tokens`
- `cache_hit_rate`
- `effective_cost`

命中率公式：

```ts
const cacheHitRate = promptCacheHitTokens / (promptCacheHitTokens + promptCacheMissTokens)
```

当 `cacheHitRate < 0.96` 时，优先优化上下文结构，再考虑切换模型。

## Skill 开发流程

1. 明确 Skill 触发场景。
2. 写 `name` 和 `description`。
3. 只把必要执行规则写入 `SKILL.md`。
4. 如果内容超过 500 行，拆到 `references/`。
5. 如需稳定自动化，放入 `scripts/`。
6. 用真实任务检查 Skill 是否会引导 Agent 少用 tokens。
7. 记录命中率变化。

## 验证命令

在项目根目录外也可以验证单个 Skill：

```powershell
python C:\Users\Administrator\.codex\skills\.system\skill-creator\scripts\quick_validate.py C:\Users\Administrator\Desktop\MHcode\skills\mhcode-agent-core
```

通过后再把 Skills 接入 MHcode 的 Skill 加载器。
