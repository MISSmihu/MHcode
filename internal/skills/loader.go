package skills

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Loader struct {
	root string
}

func NewLoader(root string) Loader {
	return Loader{root: root}
}

func (l Loader) Index() ([]IndexEntry, error) {
	entries := []IndexEntry{}
	err := filepath.WalkDir(l.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		entry, err := parseSkillFile(path)
		if err != nil {
			return err
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []IndexEntry{}, nil
		}
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func parseSkillFile(path string) (IndexEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return IndexEntry{}, err
	}
	sum := sha256.Sum256(data)
	meta := parseFrontmatter(string(data))
	name := meta["name"]
	description := meta["description"]
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}
	return IndexEntry{
		Name:        name,
		Version:     1,
		Trigger:     summarizeTrigger(description),
		Summary:     summarizeSkill(name),
		SHA256:      "sha256:" + hex.EncodeToString(sum[:]),
		Description: description,
	}, nil
}

func parseFrontmatter(content string) map[string]string {
	meta := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	if !scanner.Scan() || scanner.Text() != "---" {
		return meta
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		meta[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return meta
}

func summarizeTrigger(description string) string {
	if description == "" {
		return "manual"
	}
	parts := strings.Split(description, "。")
	first := strings.TrimSpace(parts[0])
	if first == "" {
		return "manual"
	}
	return first
}

func summarizeSkill(name string) string {
	switch name {
	case "mhcode-agent-core":
		return "统一管理推理强度、Skills、MCP、工具调用、缓存命中和成本控制"
	default:
		return "按需加载 Skill 正文并生成稳定索引"
	}
}
