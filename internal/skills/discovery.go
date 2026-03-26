package skills

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type skillFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// Discover scans dirs in order. First occurrence of a skill name wins (project > user precedence).
func Discover(dirs []string) []Skill {
	seen := map[string]bool{}
	var result []Skill

	for _, dir := range dirs {
		expanded := expandHome(dir)
		entries, err := os.ReadDir(expanded)
		if err != nil {
			continue // dir doesn't exist — skip silently
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skillMD := filepath.Join(expanded, entry.Name(), "SKILL.md")
			fm, err := parseFrontmatter(skillMD)
			if err != nil {
				continue
			}
			if fm.Description == "" {
				log.Printf("skills: %s has no description — skipped", skillMD)
				continue
			}
			if seen[fm.Name] {
				continue // already have higher-precedence version
			}
			seen[fm.Name] = true
			result = append(result, Skill{
				Name:        fm.Name,
				Description: fm.Description,
				Path:        skillMD,
			})
		}
	}
	return result
}

func parseFrontmatter(path string) (*skillFrontmatter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return nil, fmt.Errorf("no frontmatter")
	}
	end := strings.Index(content[3:], "---")
	if end < 0 {
		return nil, fmt.Errorf("unclosed frontmatter")
	}
	yamlPart := content[3 : end+3]
	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(yamlPart), &fm); err != nil {
		return nil, err
	}
	return &fm, nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}
