package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/MISSmihu/MHcode/internal/sandboxexec"
)

const maxCommandOutputBytes = 4 * 1024 * 1024

// RunCommandTool 是唯一的 shell 通道：只负责执行命令，绝不用于读写文本文件。
// 文件读写走编码安全层（read_file/write_file/apply_patch），二者严格分离，
// 避免 shell 重定向破坏编码。命令输出单独用 DecodeCommandOutput 解码（Windows 常为 GBK）。
type RunCommandTool struct{ Policy SandboxPolicy }

func (t RunCommandTool) Name() string { return "run_command" }
func (t RunCommandTool) Description() string {
	return "在工作区内执行 build、test、编译器或程序命令。文本读取、目录列表、搜索、写入、复制和删除会被拒绝，必须使用结构化文件工具。Windows 控制台使用 UTF-8，输出仍会自动识别 UTF-16/GB18030。"
}
func (t RunCommandTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string", "description": "要执行的命令行"},
		},
		"required": []string{"command"},
	}
}

func (t RunCommandTool) Execute(ctx context.Context, rawArgs json.RawMessage) (Result, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(rawArgs, &args); err != nil {
		return errorResult("参数解析失败: " + err.Error()), nil
	}
	if strings.TrimSpace(args.Command) == "" {
		return errorResult("command 不能为空"), nil
	}
	if err := t.Policy.CheckShell(); err != nil {
		return errorResult(err.Error()), nil
	}
	if err := t.Policy.ValidateCommand(args.Command); err != nil {
		return errorResult(err.Error()), nil
	}

	// 工作目录固定为工作区根，避免命令逃逸到区外。
	workDir := strings.TrimSpace(t.Policy.WorkspaceRoot)
	if workDir == "" {
		return errorResult(ErrNoWorkspaceRoot.Error()), nil
	}
	workDir, err := filepath.Abs(workDir)
	if err != nil {
		return errorResult("invalid workspace root: " + err.Error()), nil
	}
	info, err := os.Stat(workDir)
	if err != nil || !info.IsDir() {
		return errorResult("workspace root is not an accessible directory"), nil
	}

	timeout := time.Duration(t.Policy.MaxCommandSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := buildShellCommand(runCtx, args.Command)
	cmd.Dir = workDir
	cmd.Env = safeCommandEnvironment()

	var stdout, stderr cappedCommandOutput
	stdout.limit = maxCommandOutputBytes / 2
	stderr.limit = maxCommandOutputBytes / 2
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	startedAt := time.Now()
	process, startErr := sandboxexec.Start(cmd, t.Policy.ProcessLimits())
	runErr := startErr
	if startErr == nil {
		runErr = process.Wait()
	}
	completedAt := time.Now()
	output := DecodeCommandOutput(stdout.Bytes())
	stderrOutput := DecodeCommandOutput(stderr.Bytes())
	if strings.TrimSpace(stderrOutput) != "" {
		if strings.TrimSpace(output) != "" {
			output = strings.TrimRight(output, "\r\n") + "\n[stderr]\n" + stderrOutput
		} else {
			output = stderrOutput
		}
	}
	if dropped := stdout.Dropped() + stderr.Dropped(); dropped > 0 {
		output += fmt.Sprintf("\n... [command output truncated: %d bytes]", dropped)
	}

	exitCode := 0
	status := "ok"
	if runErr != nil {
		status = "error"
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			if strings.TrimSpace(output) == "" && startErr != nil {
				output = startErr.Error()
			}
			if runCtx.Err() == context.DeadlineExceeded {
				output = output + "\n（命令超时被终止）"
			}
		}
	}

	summary := fmt.Sprintf("$ %s\n退出码 %d", args.Command, exitCode)
	return Result{
		Summary: summary,
		IsError: status == "error",
		Parts: []ResultPart{
			{
				Kind:             PartToolCall,
				Name:             t.Name(),
				Status:           status,
				Input:            args.Command,
				Output:           output,
				WorkingDirectory: workDir,
				ExitCode:         intPointer(exitCode),
				StartedAt:        startedAt.Format(time.RFC3339Nano),
				CompletedAt:      completedAt.Format(time.RFC3339Nano),
				DurationMs:       elapsedMilliseconds(startedAt, completedAt),
			},
		},
	}, nil
}

func intPointer(value int) *int {
	return &value
}

func elapsedMilliseconds(startedAt, completedAt time.Time) int64 {
	duration := completedAt.Sub(startedAt).Milliseconds()
	if duration < 1 {
		return 1
	}
	return duration
}

type cappedCommandOutput struct {
	mu      sync.Mutex
	buffer  bytes.Buffer
	limit   int
	written int
}

func (w *cappedCommandOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.written += len(p)
	remaining := w.limit - w.buffer.Len()
	if remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		_, _ = w.buffer.Write(p[:remaining])
	}
	return len(p), nil
}

func (w *cappedCommandOutput) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buffer.Bytes()...)
}

func (w *cappedCommandOutput) Dropped() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.written <= w.buffer.Len() {
		return 0
	}
	return w.written - w.buffer.Len()
}

func safeCommandEnvironment() []string {
	filtered := make([]string, 0, len(os.Environ()))
	for _, item := range os.Environ() {
		name, _, ok := strings.Cut(item, "=")
		if !ok || commandEnvironmentIsSensitive(name) || commandEnvironmentIsOverridden(name) {
			continue
		}
		filtered = append(filtered, item)
	}
	filtered = append(filtered,
		"PYTHONUTF8=1",
		"PYTHONIOENCODING=utf-8",
		"GIT_PAGER=cat",
		"PAGER=cat",
		"NO_COLOR=1",
	)
	return filtered
}

func commandEnvironmentIsOverridden(name string) bool {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "PYTHONUTF8", "PYTHONIOENCODING", "GIT_PAGER", "PAGER", "NO_COLOR":
		return true
	default:
		return false
	}
}

// SafeCommandEnvironment returns the environment exposed to agent-controlled
// processes. Interactive terminals started directly by the user may choose a
// different policy.
func SafeCommandEnvironment() []string {
	return safeCommandEnvironment()
}

func commandEnvironmentIsSensitive(name string) bool {
	name = strings.ToUpper(strings.TrimSpace(name))
	for _, marker := range []string{"API_KEY", "TOKEN", "SECRET", "PASSWORD", "CREDENTIAL", "AUTHORIZATION"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// buildShellCommand 按平台选择 shell 解释器。
func buildShellCommand(ctx context.Context, command string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// UTF-8 code page covers most modern CLI programs. DecodeCommandOutput
		// still handles UTF-16 and legacy GB18030 emitters.
		cmd = exec.CommandContext(ctx, "cmd.exe", "/d", "/c", "chcp 65001>nul & "+command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", command)
	}
	hideConsoleWindow(cmd) // Windows 上隐藏弹出的 cmd 黑框
	return cmd
}
