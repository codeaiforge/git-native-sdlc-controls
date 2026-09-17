// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
