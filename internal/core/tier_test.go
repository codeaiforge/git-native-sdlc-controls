package core

import (
	"strings"
	"testing"
)

func testMap() ComponentMap {
	return ComponentMap{
		Version:  1,
		Defaults: Defaults{UnmatchedPathTier: CriticalityHigh, BreadthThreshold: 4},
		Components: []Component{
			{ID: "payment-authorization", Match: []string{"services/payment-auth/**", "libs/payment-core/**"}, Criticality: CriticalityCritical},
			{ID: "shared-contracts", Match: []string{"contracts/**"}, Criticality: CriticalityHigh, Shared: true},
			{ID: "position-service", Match: []string{"position-service/**"}, Criticality: CriticalityMedium},
			{ID: "marketing-site", Match: []string{"apps/www/**"}, Criticality: CriticalityLow},
			{ID: "docs-site", Match: []string{"apps/docs/**"}, Criticality: CriticalityLow},
			{ID: "internal-tools", Match: []string{"tools/**"}, Criticality: CriticalityLow},
			{ID: "fixtures", Match: []string{"testdata/**"}, Criticality: CriticalityLow},
		},
	}
}

func TestComputeTier(t *testing.T) {
	tests := []struct {
		name         string
		paths        []string
		wantTier     Tier
		wantAffected []string
		wantReason   string // substring that must appear in the reason list
	}{
		{
			name:         "low criticality leaf change stays T0",
			paths:        []string{"apps/www/index.html"},
			wantTier:     T0,
			wantAffected: []string{"marketing-site"},
			wantReason:   "marketing-site criticality=low -> base T0",
		},
		{
			name:         "shared component escalates one tier",
			paths:        []string{"contracts/orders.yaml"},
			wantTier:     T3,
			wantAffected: []string{"shared-contracts"},
			wantReason:   "shared-contracts shared=true -> +1",
		},
		{
			name:         "escalation is capped at T3",
			paths:        []string{"services/payment-auth/auth.go", "contracts/orders.yaml"},
			wantTier:     T3,
			wantAffected: []string{"payment-authorization", "shared-contracts"},
			wantReason:   "(capped)",
		},
		{
			name:     "breadth escalates once the threshold is reached",
			paths:    []string{"apps/www/a.html", "apps/docs/b.md", "tools/c.sh", "testdata/d.json"},
			wantTier: T1,
			wantAffected: []string{"docs-site", "fixtures", "internal-tools",
				"marketing-site"},
			wantReason: "affected_set size=4 >= breadth_threshold=4 -> +1",
		},
		{
			name:       "undeclared path applies the fail-safe floor",
			paths:      []string{"scripts/deploy.sh"},
			wantTier:   T2,
			wantReason: "1 unmatched path(s) -> fail-safe floor T2 applied",
		},
		{
			name:         "fail-safe floor does not lower an already higher tier",
			paths:        []string{"services/payment-auth/auth.go", "scripts/deploy.sh"},
			wantTier:     T3,
			wantAffected: []string{"payment-authorization"},
			wantReason:   "fail-safe floor T2 already met",
		},
		{
			name:     "no changed paths is T0",
			paths:    nil,
			wantTier: T0,
		},
		{
			name:         "a path matching several components lands in each",
			paths:        []string{"libs/payment-core/x.go", "contracts/orders.yaml"},
			wantTier:     T3,
			wantAffected: []string{"payment-authorization", "shared-contracts"},
		},
	}

	m := testMap()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			affected, unmatched := ResolveComponents(tc.paths, m)
			got, reasons := ComputeTier(affected, unmatched, m)

			if got != tc.wantTier {
				t.Errorf("tier = %s, want %s (reasons: %v)", got, tc.wantTier, reasons)
			}
			if tc.wantAffected != nil {
				var ids []string
				for _, c := range affected {
					ids = append(ids, c.ID)
				}
				if strings.Join(ids, ",") != strings.Join(tc.wantAffected, ",") {
					t.Errorf("affected = %v, want %v", ids, tc.wantAffected)
				}
			}
			if tc.wantReason != "" && !containsSubstring(reasons, tc.wantReason) {
				t.Errorf("reasons %v do not contain %q", reasons, tc.wantReason)
			}
		})
	}
}

func TestComputeTierIsDeterministic(t *testing.T) {
	m := testMap()
	paths := []string{"contracts/orders.yaml", "apps/www/a.html", "scripts/x.sh"}
	reversed := []string{"scripts/x.sh", "apps/www/a.html", "contracts/orders.yaml"}

	affectedA, unmatchedA := ResolveComponents(paths, m)
	a, ra := ComputeTier(affectedA, unmatchedA, m)
	affectedB, unmatchedB := ResolveComponents(reversed, m)
	b, rb := ComputeTier(affectedB, unmatchedB, m)
	if a != b || strings.Join(ra, "|") != strings.Join(rb, "|") {
		t.Errorf("path order changed the decision: %s %v vs %s %v", a, ra, b, rb)
	}
}

func TestUnmatchedPathTierDefaultsToHigh(t *testing.T) {
	m := ComponentMap{Components: []Component{{ID: "x", Match: []string{"x/**"}, Criticality: CriticalityLow}}}
	affected, unmatched := ResolveComponents([]string{"somewhere/else.txt"}, m)
	tier, reasons := ComputeTier(affected, unmatched, m)
	if tier != T2 {
		t.Errorf("unset unmatched_path_tier gave %s, want T2 (fail safe): %v", tier, reasons)
	}
}

func TestValidateMapFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		m       ComponentMap
		wantErr string // substring; "" means the map must be accepted
	}{
		{
			name: "a valid map is accepted",
			m:    testMap(),
		},
		{
			name: "a mistyped criticality is rejected, not tiered at T0",
			m: ComponentMap{Components: []Component{
				{ID: "payment-auth", Match: []string{"services/payment-auth/**"}, Criticality: "critcal"},
			}},
			wantErr: `criticality "critcal" is not one of`,
		},
		{
			name: "a missing criticality is rejected",
			m: ComponentMap{Components: []Component{
				{ID: "payment-auth", Match: []string{"services/payment-auth/**"}},
			}},
			wantErr: "is not one of",
		},
		{
			name: "a component with no match patterns is rejected",
			m: ComponentMap{Components: []Component{
				{ID: "orphan", Criticality: CriticalityCritical},
			}},
			wantErr: "could never be affected",
		},
		{
			name: "a duplicate id is rejected: it would mask the other component",
			m: ComponentMap{Components: []Component{
				{ID: "dup", Match: []string{"a/**"}, Criticality: CriticalityCritical},
				{ID: "dup", Match: []string{"b/**"}, Criticality: CriticalityLow},
			}},
			wantErr: "declared twice",
		},
		{
			name: "a component with no id is rejected",
			m: ComponentMap{Components: []Component{
				{Match: []string{"a/**"}, Criticality: CriticalityLow},
			}},
			wantErr: "has no id",
		},
		{
			name: "a mistyped unmatched_path_tier is rejected",
			m: ComponentMap{
				Defaults:   Defaults{UnmatchedPathTier: "hihg"},
				Components: []Component{{ID: "a", Match: []string{"a/**"}, Criticality: CriticalityLow}},
			},
			wantErr: "unmatched_path_tier",
		},
		{
			name:    "an empty map is rejected",
			m:       ComponentMap{},
			wantErr: "declares no components",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMap(tc.m)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("valid map rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("invalid map accepted; want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestUnknownCriticalityFailsSafeToTheTop(t *testing.T) {
	// ValidateMap is the first line of defence; this is the second, for a caller
	// that builds a map in code and never validates it.
	if got := Criticality("critcal").Level(); got != 3 {
		t.Errorf("unknown criticality level = %d, want 3 (fail safe, not fail open)", got)
	}
	if got := Criticality("").Level(); got != 3 {
		t.Errorf("empty criticality level = %d, want 3 (fail safe, not fail open)", got)
	}
}

func containsSubstring(list []string, want string) bool {
	for _, s := range list {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}
