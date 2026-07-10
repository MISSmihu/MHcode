package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoaderIndexesSkillsInStableOrder(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "b-skill", "b-skill", "second")
	writeSkill(t, root, "a-skill", "a-skill", "first")

	index, err := NewLoader(root).Index()
	if err != nil {
		t.Fatal(err)
	}
	if len(index) != 2 {
		t.Fatalf("got %d skills, want 2", len(index))
	}
	if index[0].Name != "a-skill" || index[1].Name != "b-skill" {
		t.Fatalf("skills not sorted by name: %#v", index)
	}
	if index[0].SHA256 == "" {
		t.Fatal("expected sha256 to be recorded")
	}
}

func writeSkill(t *testing.T, root, dir, name, description string) {
	t.Helper()
	path := filepath.Join(root, dir)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + name + "\ndescription: " + description + "\n---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
