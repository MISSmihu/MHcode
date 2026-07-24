---
name: mhcode-agent-core
description: 统一管理 MHcode Agent 的推理强度、权限审批、结构化文件工具、Plan、AI 团队、Skills、MCP、上下文压缩、模型路由、项目记忆、rewind、DeepSeek 前缀缓存和多协议工具循环。用于修改 MHcode Agent 执行流程、工具注册、上下文组装、供应商协议、团队审阅或成本观测时。
---

# MHcode Agent 核心

## 目标

把 MHcode Agent 组织成可路由、可缓存、可观测、可恢复、可审批的执行流程：

- 推理强度支持 `低 / 中 / 高 / 超高`。
- Skills、MCP schema 和工具注册不破坏稳定前缀。
- 文件操作默认使用结构化工具，不把 Shell 当文件 API。
- Plan、团队、记忆、rewind 和会话事件在重启与分支中保持一致。
- DeepSeek 等前缀缓存模型记录命中率、tokens 和费用。
- 权限边界先于工具调用，失败和取消必须进入明确终态。

## 执行顺序

保持以下顺序；第 1-7 项应尽量可复现，第 8-10 项才允许高频变化：

1. MHcode 系统身份。
2. 稳定 Agent 规则和权限边界。
3. 当前推理强度与预算。
4. Skills 索引；只按需加载完整 Skill。
5. MCP schema 快照。
6. 项目摘要和长期记忆摘要。
7. 模型路由、上下文预算和故障转移策略。
8. 当前用户输入。
9. 工具调用参数和摘要结果。
10. 输出要求、错误和重试信息。

不要把用户输入、工具结果或临时判断插入稳定前缀中间。

## 推理强度

使用稳定枚举：

```ts
type ReasoningLevel = "low" | "medium" | "high" | "ultra"
```

中文展示为：`低 / 中 / 高 / 超高`。四档同时影响工具调用上限、上下文策略、缓存策略和规划器：

```json
{
  "low": { "maxToolCalls": 3, "contextPolicy": "minimal", "cachePolicy": "reuse-prefix", "planner": false },
  "medium": { "maxToolCalls": 8, "contextPolicy": "task-summary", "cachePolicy": "reuse-prefix", "planner": false },
  "high": { "maxToolCalls": 16, "contextPolicy": "expanded", "cachePolicy": "stable-prefix", "planner": true },
  "ultra": { "maxToolCalls": 32, "contextPolicy": "full-relevant", "cachePolicy": "strict-stable-prefix", "planner": true }
}
```

运行中切换只影响下一轮请求，并显示“下一轮生效”。不要让菜单成为没有执行效果的装饰。

## 权限与结构化工具

所有工具先经过 `SandboxPolicy`、路径校验和 `ApprovalPolicy`：

- 检查工作区根目录和额外可写目录。
- 检查只读/工作区写入权限、网络、Shell 和破坏性操作开关。
- 应用超时、内存、CPU 和进程数限制。
- 处理用户审批、取消和失败回滚。
- Plan 阶段只使用只读工具；测试者和审阅者默认只读。

文件读取、搜索、写入、补丁、复制和删除必须使用：

```text
read_file, file_info, list_dir, search,
write_file, apply_patch, copy_file, delete_file
```

`run_command` 只用于构建、测试、编译器和确实需要执行的程序。不要通过 Shell 读取、枚举、搜索、写入、复制、移动或删除工作区文本文件。这样会绕过编码检测、路径策略、文件快照和 rewind。

Windows 文本规则：

- 保留 UTF-8/BOM、UTF-16LE/BE、GB18030 和 CRLF/LF。
- 新建 PowerShell 文件默认 UTF-8 BOM。
- 新建 CMD/BAT 文件默认 GB18030 + CRLF。
- 持久终端显式处理 UTF-8 输入输出，并检测 UTF-16/GB18030 输出。

## Skills 与 MCP

常规上下文只放 Skill 索引：

```text
skill: name
version: version
trigger: 触发条件
summary: 能力摘要
```

触发后才加载完整 `SKILL.md`，并记录 `name + version + sha256`。同会话复用摘要，不重复注入长正文。

MCP 只生成稳定 schema 快照：

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

工具列表或 schema 改变时才刷新快照。工具结果先给结论、影响范围和下一步；保留文件路径/行号/对象 ID 等引用，长原文放本地引用。

## Plan 模式

Plan 是显式能力，不默认给每轮请求增加一次规划调用。

当用户开启 Plan、当前档位允许规划、且工作区已配置时：

1. 用只读工具探索工作区。
2. 生成结构化计划。
3. 请求用户批准。
4. 批准后执行，按步骤更新状态。
5. 拒绝、取消、失败和成功都写入明确终态。

`task_progress` 是运行事件，必须在输入框上方作为独立状态渲染，不能混入助手正文或历史消息文本。

## AI 团队模式

团队模式按角色协作，不把多个角色的长原文全部塞进主上下文：

```text
planner → implementer → tester / reviewer → synthesizer
```

- Planner 只读探索和拆分任务。
- Implementer 执行批准后的修改。
- Tester 运行检查并报告可复现结果。
- Reviewer 默认只读，指出风险和缺失测试。
- Synthesizer 汇总结果，不伪造未执行的检查。
- 审阅反馈触发有限轮次修订。
- 任一角色取消、超时或失败都必须更新团队和计划终态。
- 每个角色可以使用独立供应商/模型，但共享结构化 artifact 摘要，不重复注入完整对话。

## 上下文压缩

根据当前模型真实上下文窗口扣除输出、工具和安全预留，得到输入预算。达到推理策略阈值时自动压缩：

- 先发 `context_compression/running`。
- 保留稳定 system 前缀、当前用户请求、最近完整对话组和未拆分的 tool call/result 对。
- 把旧历史合并为一条压缩记忆。
- 完成事件包含 before/after tokens、移除消息数和目标预算。
- 失败时回滚本轮，不能静默丢消息。
- 压缩只影响发给模型的上下文；事件日志、checkpoint、rewind 和分支保留完整原始历史。

模型上下文来源优先级是：上游返回、手动设置、精确模型目录、协议默认、供应商默认、安全兜底。不要把未知模型直接显示为虚假的大窗口。

## 缓存命中与路由

请求稳定前缀包含系统身份、Agent 规则、推理强度、Skill 索引、MCP schema、项目摘要和路由策略。易变尾部包含用户输入、diff、工具参数、工具摘要、错误和重试。

每次模型响应记录：

- `prompt_cache_hit_tokens`
- `prompt_cache_miss_tokens`
- `input_tokens`
- `output_tokens`
- `cache_hit_rate`
- `effective_cost`

命中率低于目标时先检查上下文顺序、schema 稳定性、Skill 重复注入和工具结果长度，再考虑换模型或改价格策略。

## 协议与工具循环

协议差异必须留在 `internal/protocol`。新增协议至少测试：

- 认证、模型列表和上下文窗口。
- 流式文本、结束事件和空响应。
- 原生工具调用参数的累积、归一化和错误。
- usage/cache 字段映射。
- EOF、超时、限流、余额不足和部分工具成功。
- 失败转移不重复追加用户消息、不破坏会话状态。

Agent 工具循环应返回结构化 `Summary`、`Parts`、变更快照和必要附件；模型失败但工具已经成功时，保留可用结果并明确告知用户，不伪造最终答案。

## 记忆、事件和 rewind

项目长期记忆是项目级摘要，不是把所有历史重新注入每轮上下文。遵守：

- 按设置限制会话数和字符数。
- 会话内冻结的摘要在当前分支复用。
- Rewind 移动当前 head，不删除旧事件。
- 从旧消息继续时自然形成新分支。
- 文件写入立即记录前后快照、编码和行尾。
- 删除项目不删除用户源工作区；必要时迁移到临时 `MHcodeProject`。

## 修改与验证

修改 Agent 流程时同步检查：

1. Go 类型、权限和持久化迁移。
2. 工具 schema 与注册顺序。
3. 协议 mock 和流式错误测试。
4. 前端事件类型、加载态、失败态和空态。
5. 稳定前缀和上下文预算。
6. Plan/团队取消、失败和恢复。
7. 事件日志、rewind 和重启恢复。

至少运行：

```powershell
cd frontend
bun.cmd run check
cd ..
go test ./... -count=1
go vet ./...
```

涉及 Wails API、浏览器原生表面、Windows 进程控制或打包时追加 `wails build -clean`。
