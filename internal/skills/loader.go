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
	root   string
	source fs.FS
}

func NewLoader(root string) Loader {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return Loader{root: ".", source: os.DirFS(root)}
}

func NewFSLoader(source fs.FS, root string) Loader {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	return Loader{root: filepath.ToSlash(root), source: source}
}

func (l Loader) Index() ([]IndexEntry, error) {
	entries := []IndexEntry{}
	if l.source == nil {
		return entries, nil
	}
	err := fs.WalkDir(l.source, l.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		data, err := fs.ReadFile(l.source, path)
		if err != nil {
			return err
		}
		entry := parseSkillData(path, data)
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

type LoadedSkill struct {
	Name    string
	SHA256  string
	Content string
}

func (l Loader) Load(name string) (LoadedSkill, error) {
	name = strings.TrimSpace(name)
	if name == "" || l.source == nil {
		return LoadedSkill{}, fs.ErrNotExist
	}
	var loaded LoadedSkill
	err := fs.WalkDir(l.source, l.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		data, err := fs.ReadFile(l.source, path)
		if err != nil {
			return err
		}
		entry := parseSkillData(path, data)
		if entry.Name != name {
			return nil
		}
		if len(data) > 256*1024 {
			return errors.New("skill file exceeds 256 KiB")
		}
		loaded = LoadedSkill{Name: entry.Name, SHA256: entry.SHA256, Content: string(data)}
		return fs.SkipAll
	})
	if err != nil && !errors.Is(err, fs.SkipAll) {
		return LoadedSkill{}, err
	}
	if loaded.Name == "" {
		return LoadedSkill{}, fs.ErrNotExist
	}
	return loaded, nil
}

func parseSkillData(path string, data []byte) IndexEntry {
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
	}
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
