package core

import (
	"regexp"
	"strings"
)

// trailerRE matches a git trailer line: "Key: value".
var trailerRE = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z0-9-]*)\s*:\s*(.+?)\s*$`)

// ParseProvenance reads CAF-SDLC-010 trailers out of commit messages:
//
//	AI-Assisted: true
//	AI-Tool: claude-code
//	AI-Session: <id>
//	Prompt-Ref: <issue or task>
//
// Keys are case-insensitive. A change is AI-assisted if any of its commits says
// so; for the descriptive fields the last commit that carries one wins, so the
// most recent statement of tool is the one recorded.
//
// AI-Session is the exception. It is a convenience for the clean case where one
// session produced the change: where several did, there is no single session
// that produced it, and the field is left empty rather than filled with
// whichever one happened to come last. An absent session is honest; an
// arbitrary one is a claim about how the change was made that is not true.
func ParseProvenance(commitMessages []string) Provenance {
	var p Provenance
	var sessions []string
	seen := map[string]struct{}{}

	for _, msg := range commitMessages {
		for _, line := range strings.Split(msg, "\n") {
			mt := trailerRE.FindStringSubmatch(line)
			if mt == nil {
				continue
			}
			key, value := strings.ToLower(mt[1]), mt[2]
			switch key {
			case "ai-assisted":
				if isTruthy(value) {
					p.AIAssisted = true
				}
			case "ai-tool":
				p.AITool = value
			case "ai-session":
				if _, dup := seen[value]; !dup {
					seen[value] = struct{}{}
					sessions = append(sessions, value)
				}
			case "prompt-ref":
				p.PromptRef = value
			}
		}
	}

	if len(sessions) == 1 {
		p.AISession = sessions[0]
	}
	// A named tool or session is itself a claim of AI assistance; treat it as one
	// so a missing AI-Assisted trailer cannot understate provenance. Sessions
	// count here even when too many of them to record a single one.
	if p.AITool != "" || len(sessions) > 0 {
		p.AIAssisted = true
	}
	return p
}

func isTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "yes", "y", "1":
		return true
	}
	return false
}
