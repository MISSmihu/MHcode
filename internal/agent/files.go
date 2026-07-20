package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/MISSmihu/MHcode/internal/tools"
)

const (
	maxWorkspacePreviewFileBytes = 16 << 20
	maxWorkspacePreviewBytes     = 2 << 20
	maxWorkspacePreviewLines     = 5000
)

type WorkspaceFilePreview struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Content    string `json:"content"`
	Encoding   string `json:"encoding"`
	LineEnding string `json:"lineEnding"`
	LineCount  int    `json:"lineCount"`
	Size       int64  `json:"size"`
	Truncated  bool   `json:"truncated"`
	Binary     bool   `json:"binary"`
	TooLarge   bool   `json:"tooLarge"`
}

// ResolveWorkspaceFile validates a UI-requested artifact path against the
// active workspace policy before the desktop shell opens or reveals it.
func (s *Service) ResolveWorkspaceFile(path string) (string, error) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("文件路径不能为空")
	}
	abs, err := s.sandboxPolicy().ResolveReadPath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("文件不存在或无法访问: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("目标是目录，不是文件")
	}
	return abs, nil
}

func (s *Service) OpenWorkspaceFile(path string) error {
	abs, err := s.ResolveWorkspaceFile(path)
	if err != nil {
		return err
	}
	if s.config.OpenFile == nil {
		return fmt.Errorf("当前桌面环境不支持打开文件")
	}
	return s.config.OpenFile(abs)
}

func (s *Service) ReadWorkspaceFile(path string) (WorkspaceFilePreview, error) {
	abs, err := s.ResolveWorkspaceFile(path)
	if err != nil {
		return WorkspaceFilePreview{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return WorkspaceFilePreview{}, fmt.Errorf("无法读取文件信息: %w", err)
	}

	s.stateMu.RLock()
	workspaceRoot := s.runtimeSettings.WorkspaceRoot
	s.stateMu.RUnlock()
	displayPath := filepath.Base(abs)
	if relative, relativeErr := filepath.Rel(workspaceRoot, abs); relativeErr == nil {
		displayPath = filepath.ToSlash(relative)
	}
	preview := WorkspaceFilePreview{
		Path: displayPath,
		Name: filepath.Base(abs),
		Size: info.Size(),
	}
	if info.Size() > maxWorkspacePreviewFileBytes {
		preview.Truncated = true
		preview.TooLarge = true
		return preview, nil
	}

	text, err := tools.ReadFileText(abs)
	if err != nil {
		return WorkspaceFilePreview{}, fmt.Errorf("读取文件失败: %w", err)
	}
	preview.Encoding = string(text.Encoding)
	preview.LineEnding = string(text.LineEnding)
	preview.Binary = text.Binary
	if text.Binary {
		return preview, nil
	}
	preview.LineCount = workspacePreviewLineCount(text.Content)
	preview.Content, preview.Truncated = truncateWorkspacePreview(text.Content)
	return preview, nil
}

func (s *Service) RevealWorkspaceFile(path string) error {
	abs, err := s.ResolveWorkspaceFile(path)
	if err != nil {
		return err
	}
	if s.config.RevealFile == nil {
		return fmt.Errorf("当前桌面环境不支持定位文件")
	}
	return s.config.RevealFile(abs)
}

func workspacePreviewLineCount(content string) int {
	if content == "" {
		return 0
	}
	count := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		count++
	}
	return count
}

func truncateWorkspacePreview(content string) (string, bool) {
	end := len(content)
	if end > maxWorkspacePreviewBytes {
		end = maxWorkspacePreviewBytes
		for end > 0 && !utf8.ValidString(content[:end]) {
			end--
		}
	}
	lines := 0
	for index := 0; index < end; index++ {
		if content[index] != '\n' {
			continue
		}
		lines++
		if lines >= maxWorkspacePreviewLines {
			end = index + 1
			break
		}
	}
	return content[:end], end < len(content)
}
