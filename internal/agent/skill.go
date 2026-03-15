package agent

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed skills/*.md
var builtinSkillFS embed.FS

// Skill represents a loadable knowledge module that provides domain-specific
// instructions to an agent. Skills are injected into the agent's prompt.
type Skill struct {
	ID          string   // unique identifier (e.g., "git-master")
	Name        string   // display name (e.g., "Git Master")
	Description string   // short description for Orchestrator
	Content     string   // full markdown content injected into prompt
	Globs       []string // file patterns for auto-activation (e.g., ["*.tsx", "src/components/**"])
	Source      string   // "builtin", "global", "project"
}

// SkillRegistry holds all available skills (built-in + custom).
type SkillRegistry struct {
	skills map[string]*Skill
}

// NewSkillRegistry creates a registry pre-loaded with built-in skills.
func NewSkillRegistry() *SkillRegistry {
	r := &SkillRegistry{
		skills: make(map[string]*Skill),
	}
	r.loadBuiltins()
	return r
}

// Get returns a skill by ID, or nil if not found.
func (r *SkillRegistry) Get(id string) *Skill {
	return r.skills[id]
}

// Resolve returns skills for the given IDs. Unknown IDs are silently skipped.
func (r *SkillRegistry) Resolve(ids []string) []*Skill {
	var result []*Skill
	for _, id := range ids {
		if s := r.skills[id]; s != nil {
			result = append(result, s)
		}
	}
	return result
}

// All returns all registered skills.
func (r *SkillRegistry) All() []*Skill {
	result := make([]*Skill, 0, len(r.skills))
	for _, s := range r.skills {
		result = append(result, s)
	}
	return result
}

// AllIDs returns all registered skill IDs.
func (r *SkillRegistry) AllIDs() []string {
	ids := make([]string, 0, len(r.skills))
	for id := range r.skills {
		ids = append(ids, id)
	}
	return ids
}

// IsValidSkill checks if the given ID is a registered skill.
func (r *SkillRegistry) IsValidSkill(id string) bool {
	_, ok := r.skills[id]
	return ok
}

// Register adds a skill to the registry (used for custom skills).
func (r *SkillRegistry) Register(s *Skill) {
	r.skills[s.ID] = s
}

// FormatSkillsContent combines the content of multiple skills into
// a single prompt section with headers.
func FormatSkillsContent(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var parts []string
	for _, s := range skills {
		parts = append(parts, s.Content)
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// loadBuiltins reads embedded markdown files and registers them as built-in skills.
func (r *SkillRegistry) loadBuiltins() {
	builtins := []struct {
		id, name, description, filename string
	}{
		{
			id:          "git-master",
			name:        "Git Master",
			description: "Atomic commits, rebase, history search, conflict resolution",
			filename:    "skills/git-master.md",
		},
		{
			id:          "code-review",
			name:        "Code Review",
			description: "Code review checklist: correctness, design, readability, security",
			filename:    "skills/code-review.md",
		},
		{
			id:          "testing",
			name:        "Testing",
			description: "Test strategies, TDD patterns, table-driven tests, test quality",
			filename:    "skills/testing.md",
		},
		{
			id:          "documentation",
			name:        "Documentation",
			description: "Technical writing, API docs, README structure, doc style",
			filename:    "skills/documentation.md",
		},
	}

	for _, b := range builtins {
		data, err := builtinSkillFS.ReadFile(b.filename)
		if err != nil {
			continue // skip if embedded file not found (shouldn't happen)
		}
		r.Register(&Skill{
			ID:          b.id,
			Name:        b.name,
			Description: b.description,
			Content:     string(data),
			Source:      "builtin",
		})
	}
}

// LoadCustomSkills loads markdown skill files from a directory.
// Files must end in .md. YAML frontmatter (--- delimited) is parsed for metadata.
// Source tag identifies the origin: "global" or "project".
// If a skill ID conflicts with an existing one, project > global > builtin.
func (r *SkillRegistry) LoadCustomSkills(dirPath, source string) (int, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // directory doesn't exist — not an error
		}
		return 0, fmt.Errorf("read skills dir %s: %w", dirPath, err)
	}

	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dirPath, entry.Name()))
		if err != nil {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".md")
		skill := parseSkillFile(id, string(data), source)

		// Priority: project > global > builtin
		existing := r.skills[skill.ID]
		if existing != nil {
			priority := map[string]int{"builtin": 0, "global": 1, "project": 2}
			if priority[source] <= priority[existing.Source] {
				continue // don't override higher-priority skill
			}
		}

		r.Register(skill)
		loaded++
	}

	return loaded, nil
}

// MatchSkillsForFile returns skills whose Globs match the given file path.
// Used for auto-activation of skills based on file patterns.
func (r *SkillRegistry) MatchSkillsForFile(filePath string) []*Skill {
	var matched []*Skill
	for _, s := range r.skills {
		if len(s.Globs) == 0 {
			continue
		}
		for _, glob := range s.Globs {
			if ok, _ := filepath.Match(glob, filepath.Base(filePath)); ok {
				matched = append(matched, s)
				break
			}
			// Also try matching against the full relative path
			if ok, _ := filepath.Match(glob, filePath); ok {
				matched = append(matched, s)
				break
			}
		}
	}
	return matched
}

// CustomSkillIDs returns IDs of non-builtin skills, sorted.
func (r *SkillRegistry) CustomSkillIDs() []string {
	var ids []string
	for id, s := range r.skills {
		if s.Source != "builtin" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// --- YAML frontmatter parser ---

// parseSkillFile extracts metadata from YAML frontmatter and markdown body.
// Format:
//
//	---
//	description: "Short description for Orchestrator"
//	globs: ["*.tsx", "src/components/**"]
//	---
//	# Skill Title
//	Markdown content...
func parseSkillFile(id, raw, source string) *Skill {
	skill := &Skill{
		ID:     id,
		Name:   formatSkillName(id),
		Source: source,
	}

	content := raw

	// Check for YAML frontmatter (--- delimited)
	if strings.HasPrefix(strings.TrimSpace(raw), "---") {
		trimmed := strings.TrimSpace(raw)
		rest := trimmed[3:] // skip opening ---
		endIdx := strings.Index(rest, "\n---")
		if endIdx >= 0 {
			frontmatter := rest[:endIdx]
			content = strings.TrimSpace(rest[endIdx+4:]) // skip closing ---\n

			// Parse simple YAML fields (no full YAML parser — keep zero dependencies)
			for _, line := range strings.Split(frontmatter, "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}

				if k, v, ok := parseYAMLLine(line); ok {
					switch k {
					case "description":
						skill.Description = v
					case "name":
						skill.Name = v
					case "globs":
						skill.Globs = parseYAMLStringArray(v)
					}
				}
			}
		}
	}

	skill.Content = content

	// Extract description from first paragraph if not in frontmatter
	if skill.Description == "" {
		lines := strings.SplitN(content, "\n", 5)
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" && !strings.HasPrefix(l, "#") {
				skill.Description = l
				if len(skill.Description) > 100 {
					skill.Description = skill.Description[:100] + "..."
				}
				break
			}
		}
	}

	return skill
}

// parseYAMLLine extracts key: value from a simple YAML line.
func parseYAMLLine(line string) (key, value string, ok bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:idx])
	value = strings.TrimSpace(line[idx+1:])
	// Remove quotes
	value = strings.Trim(value, `"'`)
	return key, value, true
}

// parseYAMLStringArray parses ["a", "b", "c"] from a YAML-like string.
func parseYAMLStringArray(s string) []string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		// Single value
		if s != "" {
			return []string{strings.Trim(s, `"'`)}
		}
		return nil
	}

	inner := s[1 : len(s)-1]
	parts := strings.Split(inner, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// formatSkillName converts "my-skill" to "My Skill".
func formatSkillName(id string) string {
	parts := strings.Split(id, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
