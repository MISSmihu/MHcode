# Changelog

本项目的显著变化记录在此文件中。

## v0.3.7 - 2026-07-25

### Reliable stream finalization

- Treat a provider `finish_reason` as a semantic completion fallback when a broken OpenAI-compatible relay omits `[DONE]` and leaves the HTTP connection open.
- Keep the normal `[DONE]` and EOF paths immediate, preserve a short grace period for trailing usage, and cancel the stalled transport afterward.
- Preserve typed provider errors after semantic finish while tolerating transport errors caused by closing an already completed stream.
- Honor Gemini `finishReason` and merge fragmented usage fields without losing prompt, completion, or cache token counts.

### Live request usage

- Refresh token, cache-health, DeepSeek-session, diagnostics, and usage-ledger state after every model request in normal Agent, Plan, tool-loop, and AI-team execution.
- Count every dynamic-subagent model request instead of retaining only its final usage sample.
- Patch only usage-related frontend state so background task events cannot replace the visible conversation state.

### Verification

- Add real HTTP/SSE regressions for a finished stream that never sends `[DONE]`, trailing fragmented usage, immediate normal completion, and typed post-finish errors.
- All Go package tests, `go vet`, frontend type checking, 73 frontend tests, and the production Vite build pass.

## v0.3.6 - 2026-07-24

### True parent-child parallelism

- Make `delegate_task` start dynamic subagents in the background and return task IDs immediately instead of blocking the primary Agent.
- Add `await_subagents` for explicit status queries and final collection, with automatic collection before synthesis when the model omits it.
- Let the primary Agent continue independent reads, analysis, and tool work while up to three subagents run concurrently.
- Keep fixed cumulative tool-call limits removed; the three-worker cap only bounds concurrent model fan-out.

### Lifecycle and live status safety

- Join every child before parent commit or rollback, cancel children with the parent, and preserve independent per-child cancellation.
- Serialize workspace-mutating tools while leaving read-only work and model inference concurrent.
- Stream subagent progress as dedicated events so a completed `delegate_task` cannot regress to a long-running tool card.
- Preserve terminal subagent and tool states against late progress events, while returning child artifacts and usage exactly once.

### Verification

- Add regressions for immediate delegation, simultaneous workers, parent work during a blocked child, fan-out limits, released slots, cancellation, automatic collection, event ordering, and artifact deduplication.
- All Go package tests, `go vet`, frontend type checking, 72 frontend tests, and the production Vite build pass.

## v0.3.5 - 2026-07-24

### Progress-aware loop safety

- Keep the fixed cumulative tool-call limit removed for the main Agent, Plan mode, AI team, and dynamic subagents.
- Detect repeated no-progress tool rounds using normalized call arguments and stable result fingerprints rather than a task-length budget.
- Stop identical rounds and repeating two- or three-round cycles after three repetitions, while allowing long polling tasks whose results continue to change.
- Disable tools and request a final summary when a cycle is detected; preserve all partial output, file changes, and task activity.
- Stop safely if an upstream provider ignores `tool_choice: none` instead of executing another tool call.

### Verification

- Add regressions for identical loops, alternating loops, changing polling results, and providers that ignore disabled tools.
- Retain the 40-call long-task and 20-update plan regressions; all Go package tests and `go vet` pass.

## v0.3.4 - 2026-07-24

### Parallel subagents

- Start explore, review, and implement subagents concurrently within one delegation, while requiring non-overlapping file ownership for parallel implementation work.
- Add independent child cancellation so one subagent can be stopped without interrupting siblings or the coordinating parent task; cancelling the parent still stops every child.
- Stream and persist each subagent's output, reasoning, tool activity, status, timing, and file-change statistics across conversation switches and reloads.
- Show running subagents above the composer and provide a dedicated read-only output tab in the right-side workspace panel.

### Open-ended Agent execution

- Remove fixed tool-call counts from the main Agent, Plan mode, AI team roles, dynamic subagents, and managed remote credential tasks.
- Keep reasoning levels focused on model reasoning, context, cache, and planner policy instead of silently shortening long tasks.
- Continue to enforce cancellation, per-tool timeouts, context compression, approvals, sandbox policy, and duplicate-call guards.

### Verification

- All Go package tests and `go vet` pass, including concurrent child execution, independent cancellation, parent cancellation, event-log round trips, and a 40-call long-task regression.
- Frontend type checking, 72 tests, production Vite build, and Wails Windows/amd64 production build pass.

## v0.3.3 - 2026-07-24

### Dynamic subagents and Agent execution

- Add true dynamic subagents through `delegate_task`, with independent context, model routing, cancellation, persistence, and bounded summaries returned to the primary Agent.
- Run read-only exploration and review workers concurrently while serializing implementation workers to avoid workspace write conflicts.
- Render live subagent status, current actions, steps, model information, duration, and changed-file statistics independently from the fixed AI team workflow.
- Preserve expanded command and artifact disclosures while new streamed activity remains collapsed by default.

### Authorized remote deployment

- Prefer direct managed SSH operations for authorized deployment tasks instead of wasting turns on unrelated local discovery.
- Add host-managed sensitive-result capture so requested remote account values can be revealed or copied by the user without exposing plaintext to the model, prompts, or event logs.
- Improve SSH command classification, progress reporting, cancellation, deployment guidance, and regression coverage.

### Skills and interface

- Add a full Skill detail viewer with document and source modes, safe file actions, and persistent sliding enable controls.
- Improve code and diff disclosure behavior, structured execution cards, and conversation timeline readability.

### Verification

- All Go package tests and `go vet` pass.
- Frontend type checking, regression tests, and the production Vite build pass.

## v0.3.2 - 2026-07-24

### Secure password-based SSH

- Support direct password-based SSH login from host, username, and password input without requiring an SSH key, `ssh-agent`, or an external provider authorization entry.
- Store the password in Windows Credential Manager and pass only an opaque host-managed reference to the Agent and SSH tool.
- Restore valid credential references across compressed history and short follow-up messages while keeping the password out of prompts, logs, plans, and conversation history.
- Accept compact input such as `P:host username:user password:secret` and redact the secret in the composer preview.

### Verification

- Go tests and vet, frontend type checking and tests, production frontend build, and Wails Windows build pass.

## v0.3.1 - 2026-07-24

### Agent execution and remote operations

- Add host-managed SSH connection testing, remote command execution, and file/directory upload tools. Passwords are stored in Windows Credential Manager and are never exposed to the model or command line.
- Make task cancellation immediate, with forced cleanup after a short grace period so stalled provider or tool calls cannot leave a conversation permanently stopping.
- Preserve structured progress, tool results, provider events, and terminal details in the conversation timeline.

### Conversation safety and recovery

- Redact SSH credentials from optimistic messages, queued messages, session history, and Agent context while retaining an opaque credential reference for authorized operations.
- Correct failed and interrupted turn handling so uncommitted messages return to the composer only when the model produced no usable output.
- Keep ordinary provider failures distinct from upstream security-policy errors and preserve the correct terminal state across conversation switches.

### Verification

- Go tests and vet, frontend type checking and tests, production frontend build, and Wails Windows build pass.

## v0.3.0 - 2026-07-22

### Agent 与模型协议

- 推理强度扩展为关闭、轻度、中、高、很高和极高，并按供应商协议与模型名称动态映射；OpenAI Responses、DeepSeek、Anthropic、Gemini 和 xAI 分别发送受支持的原生字段。
- 完善 Anthropic/Gemini 原生工具循环、自动上下文压缩、AI 团队任务恢复和中断后继续执行。
- 工具调用增加持久化耗时、工作目录、退出码和完整 Shell 输出；对话中默认折叠详细执行记录。
- 新增自动化任务调度、手动运行/停止，以及可持久化的项目、会话、供应商和模型路由。

### 多项目与会话

- 对话任务、历史、草稿、消息队列、团队状态和右侧面板统一按 `projectID + sessionID` 隔离，可在后台生成时切换并继续其他会话。
- 修复不同项目存在同名或重复 ID 会话时无法打开、历史清空或操作错会话的问题。
- 会话新增右键菜单，支持重命名、归档、打开项目目录、复制工作目录/会话 ID 和永久删除。
- 发送失败会回滚乐观消息并恢复输入内容；停止、编辑重发、rewind 和分叉流程补齐状态恢复。

### 工作区与界面

- 新增右侧项目文件树、CodeMirror 代码查看器、对话内代码/差异预览和文件修改摘要。
- 计划与 AI 团队状态改为输入框上方独立居中展示，并完善滚动到底、主题化确认框、推理菜单定位和动态 DPI/字体设置。
- 设置中心新增自动化任务和关于页面，支持版本检查、自动下载与应用更新。
- 内置浏览器改用可交互的原生 WebView2 表面，改进标签管理、输入法定位、缩放、页面清晰度和设置页遮挡行为。

### 验证

- 全部 Go 包测试通过。
- 前端类型检查、61 项测试和生产构建通过。
- Wails Windows/amd64 正式构建及隔离配置冷启动冒烟通过。

## v0.2.1 - 2026-07-20

### 改进

- 新增基于 CodeMirror 的只读代码查看器，非 Git 工作区也可从对话和工具结果直接查看文件及定位行号。
- 浏览器与文件查看器合并为统一右侧标签面板，切换文件时保留网页、浏览器标签和页面状态。
- 生成文件改为可折叠条目，HTML 文件支持内置预览或外部浏览器打开。
- 优化工作区文件路径识别、结构化文件读取及 Windows 文本编码处理。
- 修复 Windows Git 操作弹出控制台窗口的问题，并补充相关回归测试。

### 验证

- 前端类型检查、42 项测试和生产构建通过。
- 全部 Go 包测试通过，并完成浏览器/文件标签切换、宽度拖拽及独立关闭的运行态验证。

## v0.1.0 Preview - 2026-07-20

MHcode 首个公开 Windows 预发行版。

### 主要能力

- DeepSeek、OpenAI Compatible/Responses、Anthropic 和 Gemini 多协议模型接入。
- 自定义供应商、密钥库、上游模型发现和模型上下文窗口管理。
- 结构化文件工具、Git 工作区、持久终端、网络搜索和远程仓库读取。
- Plan 模式、AI 团队、任务进度、运行中引导和消息排队。
- 项目长期记忆、checkpoint、rewind、对话分支和自动上下文压缩。
- Chromium/CDP 内置浏览器、文件预览、图片粘贴与 Windows 窗口控制。
- Windows Job Object 进程树管理、资源限制和权限审批。

### 已知限制

- 这是 Preview 版本，尚未完成安装包签名、自动升级和真实多供应商长期压测。
- 当前主要验证 Windows；其他平台的原生浏览器表面和电脑操控能力有限。
- OS 级沙箱尚不提供文件系统或网络隔离，这两项由路径策略和用户审批约束。
- 可执行文件暂未进行代码签名，Windows 可能显示未知发布者提示。
