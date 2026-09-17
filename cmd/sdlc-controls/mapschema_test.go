// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/codeaiforge/git-native-sdlc-controls/internal/core"
)

// The component map is the tool's input, not its output, so it is validated
// here in the layer that reads files rather than in internal/contract, which
// owns what the tool emits.
const mapSchemaPath = "../../schemas/component-map/0/component-map.schema.json"

func compileMapSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	f, err := os.Open(mapSchemaPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	id := doc.(map[string]any)["$id"].(string)
	c := jsonschema.NewCompiler()
	if err := c.AddResource(id, doc); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile(id)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return sch
}

// yamlAsJSON reads a YAML map the way a JSON Schema validator needs to see it.
func yamlAsJSON(t *testing.T, body []byte) any {
	t.Helper()
	var doc any
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("parse yaml: %v", err)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("yaml -> json: %v", err)
	}
	var out any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

// Every map committed in this repository is one people copy. A map that no
// longer satisfies the published schema is either stale or the schema is.
func TestCommittedMapsValidate(t *testing.T) {
	sch := compileMapSchema(t)
	maps := []string{
		"../../config/components.yaml",
		"../../config/components.example.yaml",
		"../../examples/traderx/components.yaml",
	}
	for _, p := range maps {
		t.Run(filepath.Base(filepath.Dir(p))+"/"+filepath.Base(p), func(t *testing.T) {
			body, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := sch.Validate(yamlAsJSON(t, body)); err != nil {
				t.Errorf("%s does not satisfy component-map/0:\n%v", p, err)
			}
			// The two have to agree: a schema that accepts what the engine
			// rejects is decoration.
			var m core.ComponentMap
			if err := yaml.Unmarshal(body, &m); err != nil {
				t.Fatal(err)
			}
			if err := core.ValidateMap(m); err != nil {
				t.Errorf("%s satisfies the schema but the engine rejects it: %v", p, err)
			}
		})
	}
}

// A published schema that disagrees with the engine is worse than none: a
// generator targeting it would emit maps the tool refuses. Every case below is
// rejected by ValidateMap, so the schema has to reject it too — except the two
// the schema provably cannot express, which are named rather than skipped.
func TestSchemaAndEngineAgreeOnRejection(t *testing.T) {
	sch := compileMapSchema(t)

	tests := []struct {
		name string
		body string
		// schemaCatches is false where JSON Schema cannot express the rule;
		// the engine is then the only line of defence, which the test asserts.
		schemaCatches bool
	}{
		{
			name:          "criticality typo",
			body:          "version: 1\ncomponents:\n  - id: a\n    match: [\"*.go\"]\n    criticality: critcal\n",
			schemaCatches: true,
		},
		{
			name:          "no components",
			body:          "version: 1\ncomponents: []\n",
			schemaCatches: true,
		},
		{
			name:          "component with no match patterns",
			body:          "version: 1\ncomponents:\n  - id: a\n    match: []\n    criticality: low\n",
			schemaCatches: true,
		},
		{
			name:          "component with no id",
			body:          "version: 1\ncomponents:\n  - match: [\"*.go\"]\n    criticality: low\n",
			schemaCatches: true,
		},
		{
			name:          "bad unmatched_path_tier",
			body:          "version: 1\ndefaults:\n  unmatched_path_tier: enormous\ncomponents:\n  - id: a\n    match: [\"*.go\"]\n    criticality: low\n",
			schemaCatches: true,
		},
		{
			name:          "a format this binary does not read",
			body:          "schema_version: component-map/9\nversion: 1\ncomponents:\n  - id: a\n    match: [\"*.go\"]\n    criticality: low\n",
			schemaCatches: true,
		},
		{
			// JSON Schema 2020-12 has no uniqueness-by-property keyword, so
			// this one is engine-only by necessity, not by oversight.
			name:          "duplicate component id",
			body:          "version: 1\ncomponents:\n  - id: a\n    match: [\"*.go\"]\n    criticality: low\n  - id: a\n    match: [\"*.md\"]\n    criticality: low\n",
			schemaCatches: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			schemaErr := sch.Validate(yamlAsJSON(t, []byte(tc.body)))
			var m core.ComponentMap
			if err := yaml.Unmarshal([]byte(tc.body), &m); err != nil {
				t.Fatal(err)
			}
			engineErr := core.ValidateMap(m)

			if engineErr == nil {
				t.Fatalf("the engine accepted a map it should reject: %s", tc.body)
			}
			if tc.schemaCatches && schemaErr == nil {
				t.Errorf("the engine rejects this but component-map/0 accepts it — a generator targeting the schema would emit it:\n%s", tc.body)
			}
			if !tc.schemaCatches && schemaErr != nil {
				t.Errorf("schema caught a case documented as engine-only; tighten the doc rather than leave it stale:\n%s", tc.body)
			}
		})
	}
}

// A map with no schema_version is every map written before the format was
// published. It has to keep loading, or publishing a schema would be a
// breaking change to an input format.
func TestAbsentSchemaVersionStillLoads(t *testing.T) {
	sch := compileMapSchema(t)
	body := []byte("version: 3\ncomponents:\n  - id: a\n    match: [\"*.go\"]\n    criticality: low\n")

	if err := sch.Validate(yamlAsJSON(t, body)); err != nil {
		t.Errorf("a map without schema_version must validate: %v", err)
	}
	var m core.ComponentMap
	if err := yaml.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if err := core.ValidateMap(m); err != nil {
		t.Errorf("a map without schema_version must load: %v", err)
	}
	if m.Version != 3 {
		t.Errorf("version = %d, want 3 — the content revision, not the format", m.Version)
	}
	if m.SchemaVersion != "" {
		t.Errorf("schema_version = %q, want empty", m.SchemaVersion)
	}
}

// A generated map governs whatever produced it. Without this, the generator is
// an ordinary file: changing it reshapes every tier in the repository and the
// gate says nothing.
func TestGeneratedMapGovernsItsGenerator(t *testing.T) {
	dir, base, _ := gitRepo(t)

	// A generator in the repo, and a map that declares it produces the map.
	gen := filepath.Join(dir, "tools", "gen-map.py")
	if err := os.MkdirAll(filepath.Dir(gen), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gen, []byte("# projects the map from the build graph\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "components.yaml")
	body := "schema_version: component-map/0\nversion: 1\n" +
		"provenance:\n  generated_by: [\"tools/gen-map.py\"]\n" +
		"defaults:\n  unmatched_path_tier: low\n  breadth_threshold: 0\n" +
		"components:\n  - id: docs\n    match: [\"**/*.txt\", \"**/*.py\"]\n    criticality: low\n    shared: false\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("add", "-A")
	git("-c", "commit.gpgsign=false", "commit", "-q", "-m", "add the generator and its map")

	// Touch the generator: nothing else changes, and the map itself is untouched.
	if err := os.WriteFile(gen, []byte("# now weights by fan-in\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("-c", "commit.gpgsign=false", "commit", "-q", "-m", "change how the map is generated")

	out, code := captureStdout(t, func() int {
		return runTier([]string{"--base", base, "--config", cfg, "--repo", dir,
			"--change-id", "PR-gen", "--author", "alice", "--approvers", "bob,carol"})
	})
	if code != 0 {
		t.Fatalf("runTier = %d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, "tier:    T3") {
		t.Errorf("a change to the generator must self-escalate to T3:\n%s", out)
	}
	if !strings.Contains(out, "tools/gen-map.py") {
		t.Errorf("the reason must name the generator that changed, not merely reach the tier:\n%s", out)
	}
	if !strings.Contains(out, "self-escalate") {
		t.Errorf("the escalation must be stated as map governance, not as a component's criticality:\n%s", out)
	}
}

// A declared generator that is not in the repository matches nothing in any
// diff, so governance would never fire and the map would look governed while
// being ungoverned. That has to fail at load, not pass quietly.
func TestMissingGeneratorIsAnError(t *testing.T) {
	dir, base, head := gitRepo(t)
	cfg := filepath.Join(dir, "components.yaml")
	body := "version: 1\nprovenance:\n  generated_by: [\"tools/not-here.py\"]\n" +
		"defaults:\n  unmatched_path_tier: low\n  breadth_threshold: 0\n" +
		"components:\n  - id: docs\n    match: [\"*.txt\"]\n    criticality: low\n"
	if err := os.WriteFile(cfg, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := runTier([]string{"--base", base, "--head", head, "--config", cfg, "--repo", dir}); got != 2 {
		t.Errorf("runTier with a missing generator = %d, want 2", got)
	}
}

// A path that can never appear in a diff governs nothing, so the engine refuses
// it rather than accepting a declaration that does not work.
func TestUndiffablePathsRejected(t *testing.T) {
	tests := []struct{ name, path string }{
		{"absolute", "/etc/gen.py"},
		{"escapes the repository", "../elsewhere/gen.py"},
		{"empty", "  "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := core.ComponentMap{
				Version:    1,
				Provenance: core.MapProvenance{GeneratedBy: []string{tc.path}},
				Components: []core.Component{{ID: "a", Match: []string{"*.go"}, Criticality: core.CriticalityLow}},
			}
			if err := core.ValidateMap(m); err == nil {
				t.Errorf("ValidateMap accepted generated_by %q, which no diff can name", tc.path)
			}
		})
	}
}
