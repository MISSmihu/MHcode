package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunCommandShellDisabled(t *testing.T) {
	policy := SandboxPolicy{WorkspaceRoot: t.TempDir(), ShellAccess: false}
	tool := RunCommandTool{Policy: policy}
	args, _ := json.Marshal(map[string]string{"command": "echo hi"})
	res, _ := tool.Execute(context.Background(), args)
	if !res.IsError {
		t.Fatal("ShellAccess=false 时应拒绝执行")
	}
}

func TestSandboxPolicyBuildsProcessLimits(t *testing.T) {
	policy := SandboxPolicy{MaxCommandMemoryMB: 2048, MaxCommandCPUPercent: 65, MaxCommandProcesses: 24}
	limits := policy.ProcessLimits()
	if limits.MemoryBytes != 2048*1024*1024 || limits.CPUPercent != 65 || limits.MaxProcesses != 24 || !limits.RestrictPrivileges {
		t.Fatalf("limits = %#v", limits)
	}
	policy.SandboxMode = "danger-full-access"
	if policy.ProcessLimits().RestrictPrivileges {
		t.Fatal("danger-full-access unexpectedly uses a restricted token")
	}
}

func TestRunCommandEcho(t *testing.T) {
	policy := SandboxPolicy{WorkspaceRoot: t.TempDir(), FilesystemAccess: "workspace-write", NetworkAccess: true, ShellAccess: true, MaxCommandSeconds: 30}
	tool := RunCommandTool{Policy: policy}

	command := "echo mhcode-ok"
	if runtime.GOOS == "windows" {
		command = "echo mhcode-ok"
	}
	args, _ := json.Marshal(map[string]string{"command": command})
	res, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if res.IsError {
		t.Fatalf("不应报错: %s", res.Summary)
	}
	if len(res.Parts) != 1 || res.Parts[0].Kind != PartToolCall {
		t.Fatalf("应产出 tool_call part: %+v", res.Parts)
	}
	if !strings.Contains(res.Parts[0].Output, "mhcode-ok") {
		t.Fatalf("输出应含 mhcode-ok: %q", res.Parts[0].Output)
	}
}

func TestRunCommandRejectsDestructiveOperationByDefault(t *testing.T) {
	policy := SandboxPolicy{WorkspaceRoot: t.TempDir(), FilesystemAccess: "workspace-write", NetworkAccess: true, ShellAccess: true}
	tool := RunCommandTool{Policy: policy}
	args, _ := json.Marshal(map[string]string{"command": "del definitely-not-a-real-file.txt"})
	res, _ := tool.Execute(context.Background(), args)
	if !res.IsError || (!strings.Contains(res.Summary, "destructive") && !strings.Contains(res.Summary, "delete_file")) {
		t.Fatalf("destructive command was not rejected: %#v", res)
	}
}

func TestRunCommandRejectsNetworkWhenDisabled(t *testing.T) {
	policy := SandboxPolicy{WorkspaceRoot: t.TempDir(), FilesystemAccess: "workspace-write", NetworkAccess: false, ShellAccess: true}
	tool := RunCommandTool{Policy: policy}
	args, _ := json.Marshal(map[string]string{"command": "curl https://example.com"})
	res, _ := tool.Execute(context.Background(), args)
	if !res.IsError || !strings.Contains(res.Summary, "network") {
		t.Fatalf("network command was not rejected: %#v", res)
	}
}

func TestRunCommandRejectsForeignAbsolutePath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("uses a Windows absolute path")
	}
	policy := SandboxPolicy{WorkspaceRoot: t.TempDir(), FilesystemAccess: "workspace-write", NetworkAccess: true, ShellAccess: true}
	tool := RunCommandTool{Policy: policy}
	args, _ := json.Marshal(map[string]string{"command": `type C:\Windows\win.ini`})
	res, _ := tool.Execute(context.Background(), args)
	if !res.IsError || (!strings.Contains(res.Summary, "outside") && !strings.Contains(res.Summary, "read_file")) {
		t.Fatalf("foreign path was not rejected: %#v", res)
	}
}

func TestRunCommandRedirectsFileOperationsToStructuredTools(t *testing.T) {
	policy := SandboxPolicy{WorkspaceRoot: t.TempDir(), FilesystemAccess: "workspace-write", NetworkAccess: true, ShellAccess: true, AllowDestructiveOps: true}
	tool := RunCommandTool{Policy: policy}
	for command, expected := range map[string]string{
		"Get-Content README.md":                       "read_file",
		`powershell -Command "Get-Content README.md"`: "read_file",
		"Get-ChildItem -Recurse":                      "list_dir",
		"rg TODO .":                                   "search",
		"Set-Content a.txt hello":                     "write_file",
		"Copy-Item a.txt b.txt":                       "copy_file",
		"Remove-Item obsolete.txt":                    "delete_file",
	} {
		args, _ := json.Marshal(map[string]string{"command": command})
		res, _ := tool.Execute(context.Background(), args)
		if !res.IsError || !strings.Contains(res.Summary, expected) {
			t.Fatalf("command %q was not redirected to %s: %#v", command, expected, res)
		}
	}
}

func TestPowerShellScriptWrittenByFileToolRunsWithUnicode(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows PowerShell encoding contract")
	}
	root := t.TempDir()
	policy := SandboxPolicy{WorkspaceRoot: root, FilesystemAccess: "workspace-write", NetworkAccess: true, ShellAccess: true, MaxCommandSeconds: 30}
	write := WriteFileTool{Policy: policy}
	args, _ := json.Marshal(map[string]string{
		"path":    "unicode.ps1",
		"content": "Write-Output '脚本编码正常'\n",
	})
	result, err := write.Execute(context.Background(), args)
	if err != nil || result.IsError {
		t.Fatalf("write result=%#v err=%v", result, err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "unicode.ps1"))
	if err != nil || !strings.HasPrefix(string(raw), string(utf8BOM)) {
		t.Fatalf("PowerShell script must carry UTF-8 BOM: %x err=%v", raw, err)
	}
	run := RunCommandTool{Policy: policy}
	runArgs, _ := json.Marshal(map[string]string{"command": "powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File unicode.ps1"})
	runResult, err := run.Execute(context.Background(), runArgs)
	if err != nil || runResult.IsError || !strings.Contains(runResult.Parts[0].Output, "脚本编码正常") {
		t.Fatalf("run result=%#v err=%v", runResult, err)
	}
}
