package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, dir, folder, name, desc string) {
	t.Helper()
	skillDir := filepath.Join(dir, folder)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n正文\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTriggeredSkillLoadsFullInstructions(t *testing.T) {
	globalSkills := t.TempDir()
	writeSkill(t, globalSkills, "helper", "project-helper", "项目专属构建流程")
	svc := NewService(ServiceConfig{SkillsDir: globalSkills})
	index := svc.loadSkillsIndex()
	loaded := svc.loadTriggeredSkills("请调用 project-helper 完成构建", index)
	if len(loaded) != 1 || !strings.Contains(loaded[0], "正文") {
		t.Fatalf("triggered skills = %#v", loaded)
	}
	if unrelated := svc.loadTriggeredSkills("普通闲聊", index); len(unrelated) != 0 {
		t.Fatalf("unrelated prompt loaded skills: %#v", unrelated)
	}
}

// TestProjectSkillsAutoLoad 验证活动项目工作区下的 skills 会被自动合并加载。
func TestProjectSkillsAutoLoad(t *testing.T) {
	globalSkills := t.TempDir()
	workspace := t.TempDir()

	// 全局技能。
	writeSkill(t, globalSkills, "core", "mhcode-agent-core", "全局核心技能")
	// 项目内技能（workspace/skills/xxx）。
	writeSkill(t, filepath.Join(workspace, "skills"), "proj-skill", "project-helper", "项目专属技能")

	svc := NewService(ServiceConfig{SkillsDir: globalSkills})
	svc.runtimeSettings.WorkspaceRoot = workspace

	index := svc.loadSkillsIndex()
	names := map[string]bool{}
	for _, e := range index {
		names[e.Name] = true
	}
	if !names["mhcode-agent-core"] {
		t.Fatal("应加载全局技能 mhcode-agent-core")
	}
	if !names["project-helper"] {
		t.Fatal("应自动加载项目内技能 project-helper")
	}
}

// TestProjectSkillsOverrideGlobal 验证同名技能项目内覆盖全局。
func TestProjectSkillsOverrideGlobal(t *testing.T) {
	globalSkills := t.TempDir()
	workspace := t.TempDir()
	writeSkill(t, globalSkills, "shared", "shared-skill", "全局版本")
	writeSkill(t, filepath.Join(workspace, "skills"), "shared", "shared-skill", "项目版本")

	svc := NewService(ServiceConfig{SkillsDir: globalSkills})
	svc.runtimeSettings.WorkspaceRoot = workspace

	index := svc.loadSkillsIndex()
	count := 0
	var desc string
	for _, e := range index {
		if e.Name == "shared-skill" {
			count++
			desc = e.Description
		}
	}
	if count != 1 {
		t.Fatalf("同名技能应去重为1个, got %d", count)
	}
	if desc != "项目版本" {
		t.Fatalf("同名应项目内覆盖, got %q", desc)
	}
}
