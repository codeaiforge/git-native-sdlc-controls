package core

import (
	"regexp"
	"sort"
	"strings"
)

// ResolveComponents matches changed paths against the declared component map.
// A path may match several components; every match lands in affected. Paths that
// match nothing land in unmatched and drive the fail-safe in ComputeTier.
//
// Both results are sorted (components by ID, paths lexically) so the same input
// always produces the same evidence record.
func ResolveComponents(changedPaths []string, m ComponentMap) (affected []Component, unmatched []string) {
	byID := map[string]Component{}
	unmatchedSet := map[string]struct{}{}
	globs := compileGlobs(m)

	for _, p := range changedPaths {
		p = normalizePath(p)
		if p == "" {
			continue
		}
		hit := false
		for _, c := range m.Components {
			if componentMatches(c, p, globs) {
				byID[c.ID] = c
				hit = true
			}
		}
		if !hit {
			unmatchedSet[p] = struct{}{}
		}
	}

	for _, c := range byID {
		affected = append(affected, c)
	}
	sort.Slice(affected, func(i, j int) bool { return affected[i].ID < affected[j].ID })

	for p := range unmatchedSet {
		unmatched = append(unmatched, p)
	}
	sort.Strings(unmatched)

	return affected, unmatched
}

func componentMatches(c Component, path string, globs map[string]*regexp.Regexp) bool {
	for _, pattern := range c.Match {
		if re, ok := globs[pattern]; ok && re.MatchString(path) {
			return true
		}
	}
	return false
}

// normalizePath strips the leading "./" and any surrounding whitespace that
// git output or hand-written config can carry.
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	return strings.TrimPrefix(p, "./")
}

// ponytail: globs are translated to regexps rather than pulling in a glob
// library — `*`, `**` and `?` cover every pattern the component map needs.
// Swap in a real glob package if brace expansion or character classes ever matter.
func compileGlobs(m ComponentMap) map[string]*regexp.Regexp {
	globs := map[string]*regexp.Regexp{}
	for _, c := range m.Components {
		for _, pattern := range c.Match {
			if _, ok := globs[pattern]; !ok {
				globs[pattern] = compileGlob(pattern)
			}
		}
	}
	return globs
}

func compileGlob(pattern string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString(`^`)
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(`.*`) // ** crosses directory separators
				i++
			} else {
				b.WriteString(`[^/]*`)
			}
		case '?':
			b.WriteString(`[^/]`)
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString(`$`)
	// The generated expression is always valid: every literal byte is quoted.
	return regexp.MustCompile(b.String())
}
