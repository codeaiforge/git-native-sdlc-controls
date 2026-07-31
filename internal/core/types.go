// Package core is the CI-agnostic engine: it turns a set of changed paths plus a
// declared component map into a risk tier, an ordered reason list, and an evidence
// record. It must not import anything CI-, forge- or build-tool-specific.
package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Criticality is the declared blast-radius class of a component.
type Criticality string

const (
	CriticalityLow      Criticality = "low"
	CriticalityMedium   Criticality = "medium"
	CriticalityHigh     Criticality = "high"
	CriticalityCritical Criticality = "critical"
)

// Level maps a criticality onto the tier scale (low=0 … critical=3).
//
// An unrecognised or empty value fails safe to the top of the scale rather than
// the bottom: a criticality the map cannot express is exactly the case that must
// not be silently under-tiered. ValidateMap rejects such values at load time, so
// this is the second line of defence, not the first.
func (c Criticality) Level() int {
	switch Criticality(strings.ToLower(string(c))) {
	case CriticalityCritical:
		return 3
	case CriticalityHigh:
		return 2
	case CriticalityMedium:
		return 1
	case CriticalityLow:
		return 0
	default:
		return 3
	}
}

// Valid reports whether c is one of the four declared criticality classes.
func (c Criticality) Valid() bool {
	switch Criticality(strings.ToLower(string(c))) {
	case CriticalityLow, CriticalityMedium, CriticalityHigh, CriticalityCritical:
		return true
	}
	return false
}

// Tier is the risk tier of a change, T0 (lowest) to T3 (highest).
type Tier int

const (
	T0 Tier = iota
	T1
	T2
	T3
)

// MaxTier is the cap; escalations never exceed it.
const MaxTier = T3

func (t Tier) String() string { return fmt.Sprintf("T%d", int(t)) }

func (t Tier) MarshalJSON() ([]byte, error) { return json.Marshal(t.String()) }

func (t *Tier) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	var n int
	if _, err := fmt.Sscanf(s, "T%d", &n); err != nil {
		return fmt.Errorf("invalid tier %q", s)
	}
	if n < int(T0) || n > int(MaxTier) {
		return fmt.Errorf("tier %q out of range", s)
	}
	*t = Tier(n)
	return nil
}

// Component is one declared unit of blast radius.
type Component struct {
	ID                 string      `yaml:"id" json:"id"`
	Match              []string    `yaml:"match" json:"match"`
	Criticality        Criticality `yaml:"criticality" json:"criticality"`
	Shared             bool        `yaml:"shared" json:"shared"`
	Owners             []string    `yaml:"owners,omitempty" json:"owners,omitempty"`
	CrossRepoConsumers []string    `yaml:"cross_repo_consumers,omitempty" json:"cross_repo_consumers,omitempty"`
}

// Defaults are the map-wide knobs the tier algorithm reads.
type Defaults struct {
	// UnmatchedPathTier is the fail-safe floor applied when a changed path
	// matches no component at all.
	UnmatchedPathTier Criticality `yaml:"unmatched_path_tier" json:"unmatched_path_tier"`
	// BreadthThreshold is the affected-component count at which a change
	// escalates one tier for breadth.
	BreadthThreshold int `yaml:"breadth_threshold" json:"breadth_threshold"`
}

// ComponentMap is the declared topology of the repository.
type ComponentMap struct {
	Version    int         `yaml:"version" json:"version"`
	Defaults   Defaults    `yaml:"defaults" json:"defaults"`
	Components []Component `yaml:"components" json:"components"`
}

// ValidateMap rejects a map that cannot be reasoned about: an unrecognised
// criticality, a component that can never match, or a duplicate id that would
// mask another component. A typo in the map has to fail the run — the whole
// point of the control is lost if `criticality: critcal` quietly tiers at T0.
func ValidateMap(m ComponentMap) error {
	if len(m.Components) == 0 {
		return fmt.Errorf("component map declares no components")
	}
	if d := m.Defaults.UnmatchedPathTier; d != "" && !d.Valid() {
		return fmt.Errorf("defaults.unmatched_path_tier %q is not one of low|medium|high|critical", d)
	}
	seen := make(map[string]struct{}, len(m.Components))
	for i, c := range m.Components {
		switch {
		case c.ID == "":
			return fmt.Errorf("component %d has no id", i)
		case !c.Criticality.Valid():
			return fmt.Errorf("component %q: criticality %q is not one of low|medium|high|critical", c.ID, c.Criticality)
		case len(c.Match) == 0:
			return fmt.Errorf("component %q declares no match patterns: it could never be affected", c.ID)
		}
		if _, dup := seen[c.ID]; dup {
			return fmt.Errorf("component id %q is declared twice", c.ID)
		}
		seen[c.ID] = struct{}{}
	}
	return nil
}

// TierControls is the escalation required at one tier.
type TierControls struct {
	MinApprovers                int      `yaml:"min_approvers" json:"min_approvers"`
	Checks                      []string `yaml:"checks" json:"checks"`
	DeployApproval              string   `yaml:"deploy_approval,omitempty" json:"deploy_approval,omitempty"`
	RequireOwningTeamReviewer   bool     `yaml:"require_owning_team_reviewer,omitempty" json:"require_owning_team_reviewer,omitempty"`
	IndependentApproverRequired bool     `yaml:"independent_approver_required,omitempty" json:"independent_approver_required,omitempty"`
}

// TierPolicy maps a tier name ("T0".."T3") to its required controls.
type TierPolicy map[string]TierControls

// For returns the controls declared for a tier. Missing entries are an explicit
// error at load time (see ValidatePolicy), so the zero value here is never a
// silent pass.
func (p TierPolicy) For(t Tier) TierControls { return p[t.String()] }

// ValidatePolicy rejects a policy that does not cover every tier, so a missing
// entry can never read as "no controls required".
func ValidatePolicy(p TierPolicy) error {
	for t := T0; t <= MaxTier; t++ {
		if _, ok := p[t.String()]; !ok {
			return fmt.Errorf("tier policy missing entry for %s", t)
		}
	}
	return nil
}

// Provenance is the AI-authorship record for a change (CAF-SDLC-010).
type Provenance struct {
	AIAssisted bool   `json:"ai_assisted"`
	AITool     string `json:"ai_tool,omitempty"`
	AISession  string `json:"ai_session,omitempty"`
	PromptRef  string `json:"prompt_ref,omitempty"`
}

// EvidenceRecord is the portable artifact a CI run emits per change.
// Shape is fixed by docs/evidence-schema.md.
type EvidenceRecord struct {
	ChangeID         string       `json:"change_id"`
	BindingUsed      string       `json:"binding_used"`
	Tier             Tier         `json:"tier"`
	AffectedSet      []string     `json:"affected_set"`
	UnmatchedSet     []string     `json:"unmatched_set"`
	Reasons          []string     `json:"reasons"`
	ControlsEnforced TierControls `json:"controls_enforced"`

	// CAF-SDLC-010
	AIAssisted bool   `json:"ai_assisted"`
	AITool     string `json:"ai_tool,omitempty"`
	AISession  string `json:"ai_session,omitempty"`
	PromptRef  string `json:"prompt_ref,omitempty"`

	// CAF-SDLC-011. Both fields are pointers so "not verified" (nil) stays
	// distinguishable from "verified false" and "verified zero" in the stored
	// record. They are set together: present means the run had an authoritative
	// approver set, absent means it had none to check.
	AccountableApprover string `json:"accountable_approver,omitempty"`
	ApproverNeAuthor    *bool  `json:"approver_ne_author,omitempty"`
	ApproverCount       *int   `json:"approver_count,omitempty"`

	Warnings   []string  `json:"warnings,omitempty"`
	MapVersion int       `json:"map_version"`
	ComputedAt time.Time `json:"computed_at"`
}

// BindingVersion identifies the control binding this engine implements. It is
// recorded in every evidence record so a decision stays interpretable later.
const BindingVersion = "git-native-baseline@1"
