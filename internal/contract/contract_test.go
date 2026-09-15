// SPDX-License-Identifier: Apache-2.0

package contract_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/codeaiforge/git-native-sdlc-controls/internal/contract"
	"github.com/codeaiforge/git-native-sdlc-controls/internal/core"
)

// The schemas are a published contract, so the test validates against the
// committed files themselves rather than a copy: a schema edit that no longer
// describes what the tool emits has to fail here.
const (
	evidenceSchemaPath = "../../schemas/evidence/0/evidence.schema.json"
	bindingSchemaPath  = "../../schemas/policy-binding/0/policy-binding.schema.json"
)

// compile loads a committed schema by its own $id, so nothing is fetched over
// the network: the tests stay runnable offline, like the rest of the suite.
func compile(t *testing.T, path string) *jsonschema.Schema {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open schema: %v", err)
	}
	defer f.Close()

	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatalf("parse schema %s: %v", path, err)
	}
	obj, ok := doc.(map[string]any)
	if !ok {
		t.Fatalf("schema %s is not an object", path)
	}
	id, ok := obj["$id"].(string)
	if !ok || id == "" {
		t.Fatalf("schema %s has no $id: a published schema has to be addressable", path)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource(id, doc); err != nil {
		t.Fatalf("add schema %s: %v", path, err)
	}
	sch, err := c.Compile(id)
	if err != nil {
		t.Fatalf("compile schema %s: %v", path, err)
	}
	return sch
}

// validate marshals a DTO the way the CLI does and checks it against a schema.
func validate(t *testing.T, sch *jsonschema.Schema, doc any) map[string]any {
	t.Helper()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var inst any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber() // the validator compares numbers exactly, not as float64
	if err := dec.Decode(&inst); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := sch.Validate(inst); err != nil {
		t.Fatalf("emitted JSON does not satisfy the published schema:\n%v\n\n%s", err, b)
	}
	m, _ := inst.(map[string]any)
	return m
}

// testMap is a declared topology with one component of each shape the algorithm
// cares about: a low-criticality leaf, a shared high-criticality service with
// owners and cross-repo consumers, and a critical one.
func testMap() core.ComponentMap {
	return core.ComponentMap{
		Version: 2,
		Defaults: core.Defaults{
			UnmatchedPathTier: core.CriticalityHigh,
			BreadthThreshold:  3,
		},
		Components: []core.Component{
			{ID: "web-client", Match: []string{"web-client/**"}, Criticality: core.CriticalityLow},
			{ID: "reference-data", Match: []string{"reference-data/**"}, Criticality: core.CriticalityHigh,
				Shared: true, Owners: []string{"@org/data"}, CrossRepoConsumers: []string{"trade-service"}},
			{ID: "payments", Match: []string{"payments/**"}, Criticality: core.CriticalityCritical},
		},
	}
}

func testPolicy() core.TierPolicy {
	return core.TierPolicy{
		"T0": {MinApprovers: 1, Checks: []string{"lint"}, DeployApproval: "standard"},
		"T1": {MinApprovers: 1, Checks: []string{"lint", "sast"}, DeployApproval: "standard"},
		"T2": {MinApprovers: 1, Checks: []string{"lint", "sast", "secrets", "deps"},
			DeployApproval: "owner", RequireOwningTeamReviewer: true},
		"T3": {MinApprovers: 2, Checks: []string{"lint", "sast", "secrets", "deps"},
			DeployApproval: "change_advisory", RequireOwningTeamReviewer: true,
			IndependentApproverRequired: true},
	}
}

func evaluate(t *testing.T, in core.EvaluateInput) core.EvidenceRecord {
	t.Helper()
	in.Map = testMap()
	in.Policy = testPolicy()
	in.Now = time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)
	rec, err := core.Evaluate(in)
	if err != nil {
		t.Fatalf("core.Evaluate: %v", err)
	}
	return rec
}

func has(list []any, want string) bool {
	for _, v := range list {
		if s, ok := v.(string); ok && s == want {
			return true
		}
	}
	return false
}

func strs(t *testing.T, m map[string]any, key string) []any {
	t.Helper()
	v, ok := m[key].([]any)
	if !ok {
		t.Fatalf("%s is not an array: %#v", key, m[key])
	}
	return v
}

// TestEvidenceDTO covers the four shapes the record can take, each against the
// published schema and against the honesty invariant: what the run verified has
// to stay distinguishable from what it merely recorded.
func TestEvidenceDTO(t *testing.T) {
	sch := compile(t, evidenceSchemaPath)

	tests := []struct {
		name  string
		in    core.EvaluateInput
		tier  string
		pass  bool
		check func(t *testing.T, got map[string]any)
	}{
		{
			name: "T0 leaf change, approvers supplied",
			in: core.EvaluateInput{
				ChangeID: "PR-1", ChangedPaths: []string{"web-client/app.ts"},
				Author: "alice", Approvers: []string{"bob"}, ApproversKnown: true,
			},
			tier: "T0", pass: true,
			check: func(t *testing.T, got map[string]any) {
				v := got["verification"].(map[string]any)
				if v["approvers_supplied"] != true {
					t.Error("approvers_supplied = false with an approver set supplied")
				}
				if !has(strs(t, v, "verified"), "CAF-SDLC-011:approver-count") {
					t.Error("a verified approver set must appear in verification.verified")
				}
				if has(strs(t, v, "recorded_not_verified"), "CAF-SDLC-011:approver-count") {
					t.Error("a verified control must not also be listed as unverified")
				}
			},
		},
		{
			name: "T3 shared service, approver set known and empty",
			in: core.EvaluateInput{
				ChangeID: "PR-2", ChangedPaths: []string{"reference-data/Repo.java"},
				Author: "alice", ApproversKnown: true,
			},
			tier: "T3", pass: false,
			check: func(t *testing.T, got map[string]any) {
				res := got["result"].(map[string]any)
				if res["exit_code"].(json.Number).String() != "1" {
					t.Errorf("exit_code = %v, want 1 on a failing gate", res["exit_code"])
				}
				if len(strs(t, res, "violations")) == 0 {
					t.Error("a failing gate must state its violations in the record")
				}
				// A verified zero is a verified control, not an unverified one.
				if got["approver_count"].(json.Number).String() != "0" {
					t.Errorf("approver_count = %v, want 0", got["approver_count"])
				}
				v := got["verification"].(map[string]any)
				if !has(strs(t, v, "recorded_not_verified"), "owning-team-reviewer") {
					t.Error("T3 requires an owning-team reviewer this tool cannot check")
				}
			},
		},
		{
			name: "AI-assisted change, no approver set at all",
			in: core.EvaluateInput{
				ChangeID: "PR-3", ChangedPaths: []string{"reference-data/Repo.java"},
				CommitMessages: []string{"fix: refdata\n\nAI-Assisted: true\nAI-Tool: claude-code\nAI-Session: s1\nPrompt-Ref: #12\n"},
				Author:         "alice",
			},
			tier: "T3", pass: true,
			check: func(t *testing.T, got map[string]any) {
				if got["ai_assisted"] != true || got["ai_tool"] != "claude-code" {
					t.Errorf("provenance not carried onto the wire: %v %v", got["ai_assisted"], got["ai_tool"])
				}
				// The pointers vanish rather than defaulting, and the structured
				// form says the same thing without anyone parsing English.
				if _, present := got["approver_ne_author"]; present {
					t.Error("approver_ne_author must be absent when no approver set was supplied")
				}
				v := got["verification"].(map[string]any)
				if v["approvers_supplied"] != false {
					t.Error("approvers_supplied must be false when nothing was supplied")
				}
				for _, ctl := range []string{"CAF-SDLC-011:approver-count", "CAF-SDLC-011:independent-approver"} {
					if !has(strs(t, v, "recorded_not_verified"), ctl) {
						t.Errorf("%s must be recorded as unverified, not omitted", ctl)
					}
					if has(strs(t, v, "verified"), ctl) {
						t.Errorf("%s was never checked and must not appear as verified", ctl)
					}
				}
				// Reading the trailers is a check; the claim they make is not.
				if !has(strs(t, v, "verified"), "CAF-SDLC-010:provenance-complete") {
					t.Error("an AI-assisted change is checked for a tool name")
				}
				if !has(strs(t, v, "recorded_not_verified"), "CAF-SDLC-010:ai-authorship") {
					t.Error("provenance is a declaration, never a detection — it is never verified")
				}
			},
		},
		{
			name: "unmatched path escalates to the fail-safe floor",
			in: core.EvaluateInput{
				ChangeID: "PR-4", ChangedPaths: []string{"undeclared/thing.go"},
				Author: "alice", Approvers: []string{"bob"}, ApproversKnown: true,
			},
			tier: "T2", pass: true,
			check: func(t *testing.T, got map[string]any) {
				if len(strs(t, got, "unmatched_set")) != 1 {
					t.Errorf("unmatched_set = %v, want the undeclared path", got["unmatched_set"])
				}
				if len(strs(t, got, "affected_set")) != 0 {
					t.Errorf("affected_set = %v, want empty", got["affected_set"])
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dto := contract.EvidenceFromCore(evaluate(t, tc.in))
			got := validate(t, sch, dto)

			if got["schema_version"] != contract.EvidenceSchema {
				t.Errorf("schema_version = %v, want %s", got["schema_version"], contract.EvidenceSchema)
			}
			if got["tier"] != tc.tier {
				t.Errorf("tier = %v, want %s", got["tier"], tc.tier)
			}
			if got["result"].(map[string]any)["pass"] != tc.pass {
				t.Errorf("result.pass = %v, want %t", got["result"].(map[string]any)["pass"], tc.pass)
			}
			tc.check(t, got)
		})
	}
}

// The reason list is the decision trail, so its order has to survive the wire.
func TestEvidenceDTO_ReasonOrderPreserved(t *testing.T) {
	rec := evaluate(t, core.EvaluateInput{
		ChangeID:     "PR-5",
		ChangedPaths: []string{"reference-data/Repo.java", "undeclared/thing.go"},
	})
	dto := contract.EvidenceFromCore(rec)
	if len(dto.Reasons) != len(rec.Reasons) {
		t.Fatalf("reasons = %d entries, want %d", len(dto.Reasons), len(rec.Reasons))
	}
	for i := range rec.Reasons {
		if dto.Reasons[i] != rec.Reasons[i] {
			t.Errorf("reason %d = %q, want %q", i, dto.Reasons[i], rec.Reasons[i])
		}
	}
}

// Absent lists serialize as [], never null: a consumer should not have to
// special-case a value that only ever means "none".
func TestEvidenceDTO_EmptyListsAreArrays(t *testing.T) {
	dto := contract.EvidenceFromCore(evaluate(t, core.EvaluateInput{ChangeID: "PR-6"}))
	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"affected_set", "unmatched_set", "reasons"} {
		if string(raw[key]) == "null" {
			t.Errorf("%s serialized as null, want []", key)
		}
	}
}

// DTO -> JSON -> DTO -> JSON is stable: the document a consumer stores and
// replays is the document it received.
func TestEvidenceDTO_RoundTrip(t *testing.T) {
	dto := contract.EvidenceFromCore(evaluate(t, core.EvaluateInput{
		ChangeID: "PR-7", ChangedPaths: []string{"payments/auth.go"},
		CommitMessages: []string{"feat: x\n\nAI-Assisted: true\nAI-Tool: claude-code\n"},
		Author:         "alice", Approvers: []string{"bob", "carol"}, ApproversKnown: true,
	}))

	first, err := json.Marshal(dto)
	if err != nil {
		t.Fatal(err)
	}
	var back contract.EvidenceDTO
	if err := json.Unmarshal(first, &back); err != nil {
		t.Fatalf("a document this package emitted did not parse back into its own DTO: %v", err)
	}
	second, err := json.Marshal(back)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("round trip is not stable:\n%s\n%s", first, second)
	}
	if back.SchemaVersion != contract.EvidenceSchema {
		t.Errorf("schema_version = %q, want %q", back.SchemaVersion, contract.EvidenceSchema)
	}
}

// The binding is a projection of the live map and policy, not a second copy of
// them — so the table it prints has to be the table the engine tiers against.
func TestPolicyBindingDTO(t *testing.T) {
	sch := compile(t, bindingSchemaPath)
	m, p := testMap(), testPolicy()

	dto := contract.PolicyBindingFromCore(m, p)
	got := validate(t, sch, dto)

	if got["schema_version"] != contract.PolicyBindingSchema {
		t.Errorf("schema_version = %v, want %s", got["schema_version"], contract.PolicyBindingSchema)
	}
	if got["binding"] != core.BindingVersion {
		t.Errorf("binding = %v, want %s", got["binding"], core.BindingVersion)
	}

	for tier, want := range p {
		gotTier := dto.Tiers[tier]
		if gotTier.MinApprovers != want.MinApprovers {
			t.Errorf("%s min_approvers = %d, want %d", tier, gotTier.MinApprovers, want.MinApprovers)
		}
		if gotTier.IndependentApproverRequired != want.IndependentApproverRequired {
			t.Errorf("%s independent_approver_required = %t, want %t",
				tier, gotTier.IndependentApproverRequired, want.IndependentApproverRequired)
		}
	}

	e := dto.Escalation
	if e.BreadthThreshold != m.Defaults.BreadthThreshold {
		t.Errorf("breadth_threshold = %d, want %d", e.BreadthThreshold, m.Defaults.BreadthThreshold)
	}
	if e.UnmatchedPathCriticality != string(m.Defaults.UnmatchedPathTier) {
		t.Errorf("unmatched_path_criticality = %q, want %q", e.UnmatchedPathCriticality, m.Defaults.UnmatchedPathTier)
	}
	if e.Cap != core.MaxTier.String() || e.MapChangeSelfEscalatesTo != core.MaxTier.String() {
		t.Errorf("cap/map-change = %q/%q, want %q", e.Cap, e.MapChangeSelfEscalatesTo, core.MaxTier)
	}
	if e.SharedIncrement != core.TierIncrement {
		t.Errorf("shared_increment = %d, want the engine's %d", e.SharedIncrement, core.TierIncrement)
	}
}

// An unset fail-safe defaults to high in the engine; the published binding has
// to state the value that will actually be applied, not the empty declaration.
func TestPolicyBindingDTO_DefaultedFailSafeIsStated(t *testing.T) {
	m := testMap()
	m.Defaults.UnmatchedPathTier = ""
	e := contract.PolicyBindingFromCore(m, testPolicy()).Escalation
	if e.UnmatchedPathCriticality != string(core.CriticalityHigh) || e.UnmatchedPathTier != "T2" {
		t.Errorf("unmatched floor = %s (%s), want T2 (high)", e.UnmatchedPathTier, e.UnmatchedPathCriticality)
	}
}

// The evidence records committed under examples/ are the ones the README and
// the demo show. They are published documents, so they have to satisfy the
// published schema — a record that does not is either stale or the schema is.
func TestCommittedExampleRecordsValidate(t *testing.T) {
	sch := compile(t, evidenceSchemaPath)
	paths, err := filepath.Glob("../../examples/traderx/evidence/*.json")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no committed example records found: %v", err)
	}
	for _, p := range paths {
		t.Run(filepath.Base(p), func(t *testing.T) {
			f, err := os.Open(p)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			inst, err := jsonschema.UnmarshalJSON(f)
			if err != nil {
				t.Fatalf("parse %s: %v", p, err)
			}
			if err := sch.Validate(inst); err != nil {
				t.Errorf("%s does not satisfy schemas/evidence/0 — re-run 'make demo' and commit:\n%v", p, err)
			}
		})
	}
}
