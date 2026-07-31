package main

import (
	"path/filepath"
	"testing"
)

// Map governance compares control-file paths against the diff literally, so a
// path this function gets wrong is an escalation that silently never fires.
func TestRepoRelative(t *testing.T) {
	repo := t.TempDir()

	tests := []struct {
		name string
		repo string
		path string
		want string
		ok   bool
	}{
		{
			name: "relative paths, as CI passes them",
			repo: ".",
			path: "config/components.yaml",
			want: "config/components.yaml",
			ok:   true,
		},
		{
			name: "an absolute config path resolves to the diff's form",
			repo: repo,
			path: filepath.Join(repo, "config", "components.yaml"),
			want: "config/components.yaml",
			ok:   true,
		},
		{
			name: "a file at the repository root",
			repo: repo,
			path: filepath.Join(repo, "components.yaml"),
			want: "components.yaml",
			ok:   true,
		},
		{
			name: "a map kept outside the repository is ungovernable",
			repo: repo,
			path: filepath.Join(filepath.Dir(repo), "other-repo", "components.yaml"),
			ok:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := repoRelative(tc.repo, tc.path)
			if ok != tc.ok {
				t.Fatalf("repoRelative(%q, %q) ok = %v, want %v", tc.repo, tc.path, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Errorf("repoRelative(%q, %q) = %q, want %q", tc.repo, tc.path, got, tc.want)
			}
		})
	}
}
