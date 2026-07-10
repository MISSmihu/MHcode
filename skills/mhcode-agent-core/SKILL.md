---
name: mhcode-agent-core
description: 统一管理 MHcode Agent 的推理强度、Skills 加载、MCP schema 快照、工具调用、上下文组装、DeepSeek 前缀缓存命中率、tokens 成本和模型路由。用于设计或修改 MHcode Agent 执行流程、实现“推理：低/中/高/超高”菜单、优化 prompt_cache_hit_tokens、压缩工具结果、降低 tokens 费用、规划 96% 以上缓存命中率、接入 DeepSeek 官方或其他 AI 协议时。
---

# MHcode Agent 核心

## 目标

把 MHcode 的 Agent 执行组织成“可路由、可缓存、可观测、可控成本”的统一流程。默认目标是：

- 推理强度支持 `低 / 中 / 高 / 超高`。
- DeepSeek 等前缀缓存模型的命中率达到 `96%+`。
- Skills、MCP 和工具调用不破坏稳定前缀。
- 工具结果默认摘要化，原文用本地引用保存。

## 推理强度

UI 菜单必须支持四档：

```text
推理
低
中
高
超高  ✓
```

使用稳定枚举：

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

默认值建议为 `ultra`。切换强度后只影响后续模型请求；如果任务正在运行，显示“下一轮生效”。

### 四档策略

`低`：简单问答、轻量改文案、格式化、小范围查询。限制工具调用，优先便宜模型。

`中`：普通代码修改、单文件排查、小型重构。加载必要 Skills，使用标准上下文摘要。

`高`：跨文件修改、架构判断、复杂 bug、测试修复。允许更多 MCP 工具调用和更强模型。

`超高`：协议设计、Agent 架构、缓存策略、发布级检查。允许规划器 + 主模型组合，必须开启成本与缓存命中观测。

执行预算参考：

```json
{
  "low": { "maxToolCalls": 3, "contextPolicy": "minimal", "cachePolicy": "reuse-prefix", "planner": false },
  "medium": { "maxToolCalls": 8, "contextPolicy": "task-summary", "cachePolicy": "reuse-prefix", "planner": false },
  "high": { "maxToolCalls": 16, "contextPolicy": "expanded", "cachePolicy": "stable-prefix", "planner": true },
  "ultra": { "maxToolCalls": 32, "contextPolicy": "full-relevant", "cachePolicy": "strict-stable-prefix", "planner": true }
}
```

## 上下文分层

把请求拆成稳定前缀和易变尾部。

稳定前缀必须保持顺序、文本和 schema 哈希稳定：

- 产品身份：MHcode、协议交换台、当前工作区策略。
- 系统提示：Agent 行为边界、权限规则、输出格式。
- 当前推理强度。
- Skills 索引：只放 skill 名称、版本、触发条件和能力摘要。
- MCP 工具目录：只放工具名、输入 schema 哈希、输出摘要格式。
- 项目摘要：仓库结构、关键模块、最近稳定结论。
- 路由规则：模型选择、预算墙、故障转移策略。

易变尾部必须尽量短：

- 用户本轮输入。
- 最近 diff 或文件片段。
- 工具调用参数。
- 工具结果摘要。
- 错误、重试和临时判断。

不要把用户输入、工具结果或临时判断插入稳定前缀中间。

## Skills 加载

默认只注入 Skills 索引，不注入所有 Skills 正文。

索引格式：

```text
skill: mhcode-agent-core
version: 1
trigger: 推理强度、缓存命中、MCP、工具调用、tokens 成本
summary: 统一规划推理预算、稳定前缀、MCP schema 快照和工具结果摘要
```

只有当任务真正触发某个 Skill 时，才加载完整 `SKILL.md`。加载后记录 `name + version + sha256`，同会话优先复用摘要，不重复注入大段正文。

## MCP 策略

MCP 工具不要每轮完整展开。生成稳定工具快照：

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

只有当工具列表或 schema 变化时，才刷新快照。普通工具调用只放参数和结构化结果摘要。

## 工具结果压缩

工具结果默认按以下顺序处理：

1. 提取结论：成功/失败、影响范围、下一步。
2. 保留索引：文件路径、行号、对象 ID、命令名。
3. 压缩明细：长日志、长 JSON、长 diff 只保留关键片段。
4. 存储原文：原始结果放本地引用，不重复进入模型上下文。

摘要格式：

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

## 缓存命中观测

每次模型响应后记录：

- `prompt_cache_hit_tokens`
- `prompt_cache_miss_tokens`
- `input_tokens`
- `output_tokens`
- `cache_hit_rate = hit / (hit + miss)`
- `effective_cost`

如果 `cache_hit_rate < 0.96`，优先检查：

- 是否每轮重复注入完整 Skills 正文。
- MCP schema 是否顺序不稳定。
- 工具结果是否直接塞入原文。
- 项目摘要是否频繁重写。
- 推理强度切换是否重排了稳定前缀。
- 用户尾部内容是否被插入到前缀中间。

达不到 96% 命中率时，先优化上下文结构，再讨论模型单价。

## 模型路由

推理强度、预算墙、缓存命中率和任务风险共同决定模型路由。

默认顺序：

1. DeepSeek 官方通道。
2. OpenAI 兼容通道。
3. 聚合路由。
4. 本地模型兜底。

`超高` 可以启用规划器 + 主模型组合，但不得破坏稳定前缀。

## 执行顺序

推荐请求组装顺序：

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

保持 1-7 项可复现，8-10 项才允许高频变化。
