package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MISSmihu/MHcode/internal/sandboxexec"
)

// SandboxPolicy 是工具执行的安全边界。由 agent 层从 RuntimeSettings 填充，
// tools 包不反向依赖 agent，避免 import 环。
type SandboxPolicy struct {
	SandboxMode          string   // read-only | workspace-write | danger-full-access
	WorkspaceRoot        string   // 主工作区根，所有路径必须落在这里或 ExtraWritableRoots 下
	ExtraWritableRoots   []string // 额外可写根
	FilesystemAccess     string   // read-only | workspace-write | unrestricted
	NetworkAccess        bool     // 是否允许 web_search 等联网工具
	ShellAccess          bool     // 是否允许 run_command
	AllowDestructiveOps  bool     // Whether commands classified as destructive may run.
	MaxCommandSeconds    int      // 命令超时上限
	MaxCommandMemoryMB   int      // 单个命令进程树的内存上限；0 表示不限制
	MaxCommandCPUPercent int      // 单个命令进程树的 CPU 上限；100 表示不限制
	MaxCommandProcesses  int      // 单个命令进程树的活动进程数上限；0 表示不限制
}

func (p SandboxPolicy) ProcessLimits() sandboxexec.Limits {
	memoryBytes := uint64(0)
	if p.MaxCommandMemoryMB > 0 {
		memoryBytes = uint64(p.MaxCommandMemoryMB) * 1024 * 1024
	}
	cpuPercent := uint32(0)
	if p.MaxCommandCPUPercent > 0 && p.MaxCommandCPUPercent < 100 {
		cpuPercent = uint32(p.MaxCommandCPUPercent)
	}
	maxProcesses := uint32(0)
	if p.MaxCommandProcesses > 0 {
		maxProcesses = uint32(p.MaxCommandProcesses)
	}
	return sandboxexec.Limits{
		MemoryBytes:        memoryBytes,
		CPUPercent:         cpuPercent,
		MaxProcesses:       maxProcesses,
		RestrictPrivileges: !strings.EqualFold(strings.TrimSpace(p.SandboxMode), "danger-full-access"),
	}
}

var (
	ErrPathOutsideWorkspace   = errors.New("路径超出工作区允许范围")
	ErrForeignAbsolutePath    = errors.New("检测到其他操作系统格式的绝对路径")
	ErrReadOnlyFilesystem     = errors.New("文件系统为只读模式，禁止写入")
	ErrNetworkDisabled        = errors.New("网络访问已禁用（NetworkAccess=false）")
	ErrShellDisabled          = errors.New("命令执行已禁用（ShellAccess=false）")
	ErrShellRequiresWritable  = errors.New("shell execution requires a writable filesystem mode")
	ErrDestructiveDisabled    = errors.New("destructive shell operation is disabled")
	ErrCommandNetworkDisabled = errors.New("network access is disabled for shell commands")
	ErrCommandOutsideRoots    = errors.New("shell command references a path outside the allowed roots")
	ErrShellFileOperation     = errors.New("shell file operations are disabled; use the structured workspace tools")
	ErrSymlinkMutation        = errors.New("refusing to replace a symbolic link with a regular file")
	ErrNoWorkspaceRoot        = errors.New("未配置工作区根目录")
)

// ResolveReadPath 校验并规范化一个用于「读取」的路径：必须落在允许的根内。
func (p SandboxPolicy) ResolveReadPath(input string) (string, error) {
	return p.resolveWithinRoots(input)
}

// ResolveWritePath 校验用于「写入」的路径：既要在允许的根内，还要求文件系统非只读。
func (p SandboxPolicy) ResolveWritePath(input string) (string, error) {
	if strings.EqualFold(p.FilesystemAccess, "read-only") {
		return "", ErrReadOnlyFilesystem
	}
	abs, err := p.resolveWithinRoots(input)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Lstat(abs); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: %s", ErrSymlinkMutation, abs)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}
	return abs, nil
}

// resolveWithinRoots 把输入路径（可为相对，基于 WorkspaceRoot）转成绝对干净路径，
// 并确保它位于某个允许根之内，防止 ../ 目录穿越。
// FilesystemAccess=unrestricted 时跳过根约束（仍返回绝对路径）。
func (p SandboxPolicy) resolveWithinRoots(input string) (string, error) {
	root := strings.TrimSpace(p.WorkspaceRoot)
	if root == "" {
		return "", ErrNoWorkspaceRoot
	}

	input = strings.TrimSpace(input)
	if looksLikeForeignAbsolutePath(input) {
		return "", fmt.Errorf("%w: %s；请使用相对于当前工作区的路径，工作区根目录写作 .", ErrForeignAbsolutePath, input)
	}

	abs := input
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	abs = filepath.Clean(abs)

	if strings.EqualFold(p.FilesystemAccess, "unrestricted") {
		return abs, nil
	}

	roots := append([]string{root}, p.ExtraWritableRoots...)
	for _, candidate := range roots {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if pathWithinRoot(abs, filepath.Clean(candidate)) {
			if err := ensureResolvedPathWithinRoots(abs, roots); err != nil {
				return "", err
			}
			return abs, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrPathOutsideWorkspace, abs)
}

func ensureResolvedPathWithinRoots(target string, roots []string) error {
	resolvedRoots := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		resolved, err := filepath.EvalSymlinks(filepath.Clean(root))
		if err != nil {
			resolved = filepath.Clean(root)
		}
		resolvedRoots = append(resolvedRoots, resolved)
	}

	probe := filepath.Clean(target)
	for {
		resolved, err := filepath.EvalSymlinks(probe)
		if err == nil {
			for _, root := range resolvedRoots {
				if pathWithinRoot(resolved, root) {
					return nil
				}
			}
			return fmt.Errorf("%w: symbolic link resolves outside allowed roots: %s", ErrPathOutsideWorkspace, resolved)
		}
		if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return err
		}
		probe = parent
	}
}

func looksLikeForeignAbsolutePath(input string) bool {
	if input == "" || filepath.IsAbs(input) {
		return false
	}
	normalized := strings.ReplaceAll(input, `\`, "/")
	if normalized == "~" || strings.HasPrefix(normalized, "~/") {
		return true
	}
	if os.PathSeparator == '\\' {
		return strings.HasPrefix(normalized, "/")
	}
	return len(normalized) >= 3 &&
		((normalized[0] >= 'A' && normalized[0] <= 'Z') || (normalized[0] >= 'a' && normalized[0] <= 'z')) &&
		normalized[1] == ':' && normalized[2] == '/'
}

// pathWithinRoot 判断 target 是否等于 root 或位于 root 之下。
// 用 filepath.Rel 后检查是否以 ".." 开头，跨平台且不区分大小写差异由 OS 决定。
func pathWithinRoot(target, root string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".."
}

// CheckShell 校验是否允许执行命令。
func (p SandboxPolicy) CheckShell() error {
	if !p.ShellAccess {
		return ErrShellDisabled
	}
	if strings.EqualFold(strings.TrimSpace(p.FilesystemAccess), "read-only") {
		return ErrShellRequiresWritable
	}
	return nil
}

// ValidateCommand is a conservative command broker. It is intentionally not
// presented as an OS sandbox: arbitrary shell code cannot be made safe by
// setting cmd.Dir alone. The broker rejects common escape, network, and
// destructive forms before a process is started.
func (p SandboxPolicy) ValidateCommand(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return errors.New("command cannot be empty")
	}
	normalized := strings.ToLower(strings.ReplaceAll(command, `\`, "/"))
	if tool := structuredFileToolForCommand(normalized); tool != "" {
		return fmt.Errorf("%w: use %s", ErrShellFileOperation, tool)
	}

	if !p.AllowDestructiveOps && commandLooksDestructive(normalized) {
		return ErrDestructiveDisabled
	}
	if !p.NetworkAccess && commandLooksNetworked(normalized) {
		return ErrCommandNetworkDisabled
	}
	if !strings.EqualFold(strings.TrimSpace(p.FilesystemAccess), "unrestricted") && commandReferencesForeignPath(command, p) {
		return ErrCommandOutsideRoots
	}
	return nil
}

func structuredFileToolForCommand(command string) string {
	canonical := strings.NewReplacer(
		`"`, " ", `'`, " ", "(", " ", ")", " ", "{", " ", "}", " ",
		";", " ; ", "&&", " && ", "||", " || ", "|", " | ",
	).Replace(command)
	padded := " " + strings.Join(strings.Fields(canonical), " ") + " "
	containsCommand := func(names ...string) bool {
		for _, name := range names {
			if strings.Contains(padded, " "+name+" ") {
				return true
			}
		}
		return false
	}
	if containsCommand("get-content", "gc", "type", "cat", "head", "tail", "more") {
		return "read_file"
	}
	if containsCommand("get-childitem", "gci", "dir", "ls", "tree") || strings.Contains(padded, " rg --files ") {
		return "list_dir"
	}
	if containsCommand("select-string", "sls", "findstr", "rg", "grep") {
		return "search"
	}
	if containsCommand("set-content", "sc", "out-file", "add-content", "tee") ||
		strings.Contains(padded, " > ") || strings.Contains(padded, " >> ") {
		return "write_file or apply_patch"
	}
	if containsCommand("copy", "copy-item", "cp") {
		return "copy_file"
	}
	if containsCommand("move", "move-item", "mv", "rename-item", "rename", "ren") {
		return "copy_file followed by delete_file"
	}
	if containsCommand("del", "erase", "remove-item", "rm", "unlink") {
		return "delete_file"
	}
	return ""
}

func commandLooksDestructive(command string) bool {
	for _, marker := range []string{
		"git reset --hard", "git clean -f", "git checkout --", "git restore --source",
		"format ", "diskpart", "del ", "erase ", "rmdir ", "rd ", "remove-item",
		" rm ", "rm -", " mv ", "move ", "rename ", "ren ", "chmod ",
		"set-content", "out-file", "> ", ">>", "| tee ", "truncate ",
	} {
		if strings.Contains(" "+command, " "+marker) {
			return true
		}
	}
	return strings.HasPrefix(command, "del ") || strings.HasPrefix(command, "rm ") || strings.HasPrefix(command, "mv ")
}

func commandLooksNetworked(command string) bool {
	for _, marker := range []string{
		"curl ", "wget ", "invoke-webrequest", "invoke-restmethod",
		"bitsadmin", "certutil -urlcache", "ftp ", "ssh ", "scp ",
		"sftp ", "telnet ", "nslookup ", "http://", "https://",
	} {
		if strings.Contains(command, marker) {
			return true
		}
	}
	return false
}

func commandReferencesForeignPath(command string, policy SandboxPolicy) bool {
	for _, raw := range strings.FieldsFunc(command, func(r rune) bool {
		switch r {
		case ' ', '\t', '\r', '\n', '"', '\'', ';', '|', '&', '(', ')', ',':
			return true
		default:
			return false
		}
	}) {
		token := strings.Trim(raw, "[]{}")
		if token == "" || strings.HasPrefix(token, "-") || strings.Contains(token, "=") {
			continue
		}
		lower := strings.ToLower(token)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			continue
		}
		if !looksLikeAbsolutePath(token) {
			continue
		}
		allowed := false
		for _, root := range append([]string{policy.WorkspaceRoot}, policy.ExtraWritableRoots...) {
			root = strings.TrimSpace(root)
			if root != "" && pathWithinRoot(filepath.Clean(token), filepath.Clean(root)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return true
		}
	}
	return false
}

func looksLikeAbsolutePath(value string) bool {
	value = strings.ReplaceAll(value, `\`, "/")
	if strings.HasPrefix(value, "//") || strings.HasPrefix(value, "/") {
		return true
	}
	return len(value) >= 3 && value[1] == ':' && value[2] == '/'
}

// CheckNetwork 校验是否允许联网工具发起请求。
func (p SandboxPolicy) CheckNetwork() error {
	if !p.NetworkAccess {
		return ErrNetworkDisabled
	}
	return nil
}
