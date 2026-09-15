// SPDX-License-Identifier: Apache-2.0

package contract

import (
	"github.com/codeaiforge/git-native-sdlc-controls/internal/core"
)

// PolicyBindingDTO is the wire form of the policy that produced a tier: schema
// policy-binding/0.
//
// It is a projection, never a second copy. Every value is read from the same
// map and policy the engine tiers against, or from a constant in core — so the
// document cannot drift from the decisions it claims to explain.
type PolicyBindingDTO struct {
	SchemaVersion string  `json:"schema_version"`
	Tool          ToolDTO `json:"tool"`

	// Binding names the engine binding: the tiering algorithm and the record
	// shape. It is not affected by --policy. The tier table below is, which is
	// why the active policy is serialized rather than named.
	Binding string `json:"binding"`

	Tiers      map[string]ControlsDTO `json:"tiers"`
	Escalation EscalationDTO          `json:"escalation"`
	MapVersion int                    `json:"map_version"`
}

// EscalationDTO states the CAF-SDLC-002 rules that move a change off its base
// tier. Two come from the component map's defaults, two are engine constants.
type EscalationDTO struct {
	// UnmatchedPathCriticality is the declared fail-safe class for a changed path
	// that matches no component; UnmatchedPathTier is the tier it resolves to.
	// Both are given because the map declares one and the algorithm applies the
	// other, and a consumer should not have to reimplement the mapping.
	UnmatchedPathCriticality string `json:"unmatched_path_criticality"`
	UnmatchedPathTier        string `json:"unmatched_path_tier"`

	// SharedIncrement is added once when any affected component is declared
	// shared. BreadthThreshold is the affected-component count at which
	// BreadthIncrement is added; 0 disables the rule.
	SharedIncrement  int `json:"shared_increment"`
	BreadthThreshold int `json:"breadth_threshold"`
	BreadthIncrement int `json:"breadth_increment"`

	// MapChangeSelfEscalatesTo is the tier a change to the control files takes.
	MapChangeSelfEscalatesTo string `json:"map_change_self_escalates_to"`

	// Cap is the ceiling every escalation is clamped to.
	Cap string `json:"cap"`
}

// PolicyBindingFromCore projects the active map and policy onto the wire.
//
// Note what is deliberately absent: there is no "independent approver required
// when AI-generated" flag. CAF-SDLC-011 attaches that requirement to the tier,
// not to provenance — the baseline sets it on T3 alone, for AI-assisted and
// human changes alike. A field claiming otherwise would be a published lie about
// what the engine does. See docs/controls/CAF-SDLC-011-independent-approver.md.
func PolicyBindingFromCore(m core.ComponentMap, p core.TierPolicy) PolicyBindingDTO {
	tiers := make(map[string]ControlsDTO, int(core.MaxTier)+1)
	for t := core.T0; t <= core.MaxTier; t++ {
		c := p.For(t)
		tiers[t.String()] = ControlsDTO{
			MinApprovers:                c.MinApprovers,
			Checks:                      orEmpty(c.Checks),
			DeployApproval:              c.DeployApproval,
			RequireOwningTeamReviewer:   c.RequireOwningTeamReviewer,
			IndependentApproverRequired: c.IndependentApproverRequired,
		}
	}

	unmatched := core.UnmatchedPathTier(m)
	return PolicyBindingDTO{
		SchemaVersion: PolicyBindingSchema,
		Tool:          tool(),
		Binding:       core.BindingVersion,
		Tiers:         tiers,
		Escalation: EscalationDTO{
			UnmatchedPathCriticality: string(unmatched),
			UnmatchedPathTier:        core.Tier(unmatched.Level()).String(),
			SharedIncrement:          core.TierIncrement,
			BreadthThreshold:         m.Defaults.BreadthThreshold,
			BreadthIncrement:         core.TierIncrement,
			MapChangeSelfEscalatesTo: core.MaxTier.String(),
			Cap:                      core.MaxTier.String(),
		},
		MapVersion: m.Version,
	}
}
