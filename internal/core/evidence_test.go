package core

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testPolicy() TierPolicy {
	return TierPolicy{
		"T0": {MinApprovers: 1, Checks: []string{"lint"}},
		"T1": {MinApprovers: 1, Checks: []string{"lint", "sast"}},
		"T2": {MinApprovers: 1, Checks: []string{"lint", "sast", "secrets", "deps"}, RequireOwningTeamReviewer: true},
		"T3": {MinApprovers: 2, Checks: []string{"lint", "sast", "secrets", "deps"}, IndependentApproverRequired: true},
	}
}

func baseInput() EvaluateInput {
	return EvaluateInput{
		ChangeID:     "PR-123",
		ChangedPaths: []string{"apps/www/index.html"},
		Map:          testMap(),
		Policy:       testPolicy(),
		Now:          time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC),
	}
}

func TestEvaluateEmitsTierAndControls(t *testing.T) {
	in := baseInput()
	rec, err := Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Tier != T0 {
		t.Errorf("tier = %s, want T0", rec.Tier)
	}
	if rec.BindingUsed != BindingVersion {
		t.Errorf("binding = %q, want %q", rec.BindingUsed, BindingVersion)
	}
	if rec.ControlsEnforced.MinApprovers != 1 {
		t.Errorf("min_approvers = %d, want 1", rec.ControlsEnforced.MinApprovers)
	}
	if rec.ApproverNeAuthor != nil || rec.ApproverCount != nil {
		t.Error("approver fields should be absent when no approver set was supplied")
	}
	if !containsSubstring(rec.Warnings, "approver set not supplied") {
		t.Errorf("expected an unverified-approvers warning, got %v", rec.Warnings)
	}
}

func TestEvaluateRejectsAnUnreasonableMap(t *testing.T) {
	in := baseInput()
	in.Map.Components = append(in.Map.Components,
		Component{ID: "typo", Match: []string{"typo/**"}, Criticality: "critcal"})
	if _, err := Evaluate(in); err == nil {
		t.Fatal("expected an error for a mistyped criticality; a typo must not tier at T0")
	}
}

func TestUngovernableControlFileIsRecorded(t *testing.T) {
	in := baseInput()
	in.UngovernablePaths = []string{"../elsewhere/components.yaml"}

	rec, err := Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstring(rec.Warnings, "map-governance self-escalation cannot apply") {
		t.Errorf("a control file outside the repo must be reported, got %v", rec.Warnings)
	}
}

func TestOwningTeamReviewerIsRecordedAsUnverified(t *testing.T) {
	in := baseInput()
	in.ChangedPaths = []string{"contracts/orders.yaml"} // shared-contracts -> T3
	in.Map.Components[1].Owners = []string{"@org/platform"}
	t3 := in.Policy["T3"]
	t3.RequireOwningTeamReviewer = true
	in.Policy["T3"] = t3

	rec, err := Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if !containsSubstring(rec.Warnings, "@org/platform") ||
		!containsSubstring(rec.Warnings, "not verified by this run") {
		t.Errorf("owning-team requirement must name the owners and say it was not verified, got %v", rec.Warnings)
	}
}

func TestEvaluateRejectsIncompletePolicy(t *testing.T) {
	in := baseInput()
	delete(in.Policy, "T3")
	if _, err := Evaluate(in); err == nil {
		t.Fatal("expected an error for a policy missing T3; a missing tier must not read as no controls")
	}
}

func TestControlMapChangeSelfEscalates(t *testing.T) {
	in := baseInput()
	in.ChangedPaths = []string{"apps/www/index.html", "./config/components.yaml"}
	in.GovernedPaths = []string{"config/components.yaml"}

	rec, err := Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Tier != T3 {
		t.Errorf("tier = %s, want T3 for a control-map change: %v", rec.Tier, rec.Reasons)
	}
	if !containsSubstring(rec.Reasons, "control map changed") {
		t.Errorf("reasons %v do not explain the escalation", rec.Reasons)
	}
}

func TestIndependentApproverControl(t *testing.T) {
	tests := []struct {
		name            string
		author          string
		approvers       []string
		wantCount       int
		wantIndependent bool
		wantViolation   string
	}{
		{
			name:            "self-approval fails the T3 independent-approver control",
			author:          "dev-a",
			approvers:       []string{"dev-a", "dev-a"},
			wantCount:       1,
			wantIndependent: false,
			wantViolation:   "distinct from the author",
		},
		{
			name:            "a second, independent approver satisfies it",
			author:          "dev-a",
			approvers:       []string{"dev-a", "dev-b"},
			wantCount:       2,
			wantIndependent: true,
		},
		{
			name:            "one independent approver still misses the T3 count",
			author:          "dev-a",
			approvers:       []string{"dev-b"},
			wantCount:       1,
			wantIndependent: true,
			wantViolation:   "requires 2 approver(s), found 1",
		},
		{
			name:            "a verified empty approver set fails, it does not pass unverified",
			author:          "dev-a",
			approvers:       nil,
			wantCount:       0,
			wantIndependent: false,
			wantViolation:   "requires 2 approver(s), found 0",
		},
		{
			// The gap that matters: one reviewer listed twice must not satisfy a
			// two-approver tier, and must not be recorded as two approvers. A
			// forge that returns a reviewer twice, or a CI adapter that does not
			// deduplicate, would otherwise buy an approval that never happened.
			name:            "one approver listed twice is one approver",
			author:          "dev-a",
			approvers:       []string{"dev-b", "dev-b"},
			wantCount:       1,
			wantIndependent: true,
			wantViolation:   "requires 2 approver(s), found 1",
		},
		{
			name:            "the same handle in two spellings is one approver",
			author:          "dev-a",
			approvers:       []string{"dev-b", "DEV-B", " dev-b "},
			wantCount:       1,
			wantIndependent: true,
			wantViolation:   "requires 2 approver(s), found 1",
		},
		{
			name:            "deduplication does not swallow a genuine second approver",
			author:          "dev-a",
			approvers:       []string{"dev-b", "DEV-B", "dev-c"},
			wantCount:       2,
			wantIndependent: true,
		},
		{
			name:            "the author padding the set out cannot buy the count",
			author:          "dev-a",
			approvers:       []string{"dev-a", "DEV-A", " dev-a "},
			wantCount:       1,
			wantIndependent: false,
			wantViolation:   "requires 2 approver(s), found 1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := baseInput()
			in.ChangedPaths = []string{"services/payment-auth/auth.go"} // T3
			in.Author = tc.author
			in.Approvers = tc.approvers
			in.ApproversKnown = true

			rec, err := Evaluate(in)
			if err != nil {
				t.Fatal(err)
			}
			if rec.Tier != T3 {
				t.Fatalf("tier = %s, want T3", rec.Tier)
			}
			if rec.ApproverNeAuthor == nil || *rec.ApproverNeAuthor != tc.wantIndependent {
				t.Errorf("approver_ne_author = %v, want %v", rec.ApproverNeAuthor, tc.wantIndependent)
			}
			if rec.ApproverCount == nil {
				t.Errorf("approver_count is nil, want %d", tc.wantCount)
			} else if *rec.ApproverCount != tc.wantCount {
				t.Errorf("approver_count = %d, want %d", *rec.ApproverCount, tc.wantCount)
			}
			violations := rec.Violations()
			if tc.wantViolation == "" {
				if len(violations) != 0 {
					t.Errorf("expected no violations, got %v", violations)
				}
				return
			}
			if !containsSubstring(violations, tc.wantViolation) {
				t.Errorf("violations %v do not contain %q", violations, tc.wantViolation)
			}
		})
	}
}

func TestAIAssistedChangeNeedsATool(t *testing.T) {
	in := baseInput()
	in.CommitMessages = []string{"feat: thing\n\nAI-Assisted: true\n"}

	rec, err := Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.AIAssisted {
		t.Fatal("provenance was not carried into the evidence record")
	}
	if !containsSubstring(rec.Violations(), "missing an AI-Tool trailer") {
		t.Errorf("expected a CAF-SDLC-010 violation, got %v", rec.Violations())
	}
}

func TestEvidenceRecordSerialisesToTheDocumentedShape(t *testing.T) {
	in := baseInput()
	in.ChangedPaths = []string{"services/payment-auth/auth.go", "contracts/orders.yaml"}

	rec, err := Evaluate(in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"change_id":"PR-123"`,
		`"binding_used":"git-native-baseline@1"`,
		`"tier":"T3"`,
		`"affected_set":["payment-authorization","shared-contracts"]`,
		`"unmatched_set":[]`,
		`"ai_assisted":false`,
		`"computed_at":"2026-07-30T09:00:00Z"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("evidence JSON is missing %s\ngot: %s", want, b)
		}
	}
}
