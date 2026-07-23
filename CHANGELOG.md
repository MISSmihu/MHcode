# Changelog

本项目的显著变化记录在此文件中。

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
