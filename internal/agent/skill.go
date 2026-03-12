package agent

import (
	"embed"
	"strings"
)

//go:embed skills/*.md
var builtinSkillFS embed.FS

// Skill represents a loadable knowledge module that provides domain-specific
// instructions to an agent. Skills are injected into the agent's prompt.
type Skill struct {
	ID          string // unique identifier (e.g., "git-master")
	Name        string // display name (e.g., "Git Master")
	Description string // short description for Orchestrator
	Content     string // full markdown content injected into prompt
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
		})
	}
}
