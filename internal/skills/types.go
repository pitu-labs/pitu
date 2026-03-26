package skills

// Skill represents a discovered AgentSkills-compatible skill.
type Skill struct {
	Name        string
	Description string
	Path        string // absolute path to SKILL.md
}
