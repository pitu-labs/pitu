package skills

import (
	"fmt"
	"strings"
)

// BuildCatalog returns an XML-formatted catalog of available skills for injection into CONTEXT.md.
func BuildCatalog(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<available_skills>\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "  <skill>\n    <name>%s</name>\n    <description>%s</description>\n    <location>%s</location>\n  </skill>\n",
			s.Name, s.Description, s.Path)
	}
	b.WriteString("</available_skills>")
	return b.String()
}
