package agent

import (
	"fmt"
	"os"
	"strings"
)

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
