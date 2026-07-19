package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorkspaceFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "page.html")
	if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewService(ServiceConfig{SkillsDir: t.TempDir()})
	svc.runtimeSettings.WorkspaceRoot = root
	svc.runtimeSettings.FilesystemAccess = "workspace-write"

	resolved, err := svc.ResolveWorkspaceFile("page.html")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != path {
		t.Fatalf("resolved = %q, want %q", resolved, path)
	}
	if _, err := svc.ResolveWorkspaceFile(filepath.Join(root, "missing.html")); err == nil {
		t.Fatal("不存在的文件不应允许打开")
	}
}

func TestOpenAndRevealWorkspaceFileUseHostCallbacks(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "page.html")
	if err := os.WriteFile(path, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	opened, revealed := "", ""
	svc := NewService(ServiceConfig{
		SkillsDir: t.TempDir(),
		OpenFile: func(path string) error {
			opened = path
			return nil
		},
		RevealFile: func(path string) error {
			revealed = path
			return nil
		},
	})
	svc.runtimeSettings.WorkspaceRoot = root
	svc.runtimeSettings.FilesystemAccess = "workspace-write"
	if err := svc.OpenWorkspaceFile("page.html"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RevealWorkspaceFile("page.html"); err != nil {
		t.Fatal(err)
	}
	if opened != path || revealed != path {
		t.Fatalf("opened=%q revealed=%q want=%q", opened, revealed, path)
	}
}
