// SPDX-License-Identifier: Apache-2.0

package contract

import (
	"time"

	"github.com/codeaiforge/git-native-sdlc-controls/internal/core"
)

// EvidenceDTO is the wire form of one evidence record: schema evidence/0.
//
// Field names are those docs/evidence-schema.md has carried since v0.1.0. The
// additions in 0.2.0 — schema_version, tool, verification, result — are additive;
// nothing that a v0.1.0 consumer reads has moved or changed meaning.
type EvidenceDTO struct {
	SchemaVersion string  `json:"schema_version"`
	Tool          ToolDTO `json:"tool"`

	ChangeID         string      `json:"change_id"`
	BindingUsed      string      `json:"binding_used"`
	Tier             string      `json:"tier"`
	AffectedSet      []string    `json:"affected_set"`
	UnmatchedSet     []string    `json:"unmatched_set"`
	Reasons          []string    `json:"reasons"`
	ControlsEnforced ControlsDTO `json:"controls_enforced"`

	// CAF-SDLC-010.
	AIAssisted bool   `json:"ai_assisted"`
	AITool     string `json:"ai_tool,omitempty"`
	AISession  string `json:"ai_session,omitempty"`
	PromptRef  string `json:"prompt_ref,omitempty"`

	// CAF-SDLC-011. Pointers, and omitted when absent, because "not verified"
	// must not serialize as "verified false". The same distinction is stated
	// structurally in Verification for a consumer that would rather not infer it
	// from a field's absence.
	AccountableApprover string `json:"accountable_approver,omitempty"`
	ApproverNeAuthor    *bool  `json:"approver_ne_author,omitempty"`
	ApproverCount       *int   `json:"approver_count,omitempty"`

	Verification VerificationDTO `json:"verification"`
	Result       ResultDTO       `json:"result"`

	Warnings   []string  `json:"warnings,omitempty"`
	MapVersion int       `json:"map_version"`
	ComputedAt time.Time `json:"computed_at"`
}

// ControlsDTO is the tier's entry from the active policy — what the tier
// required, not what was checked. What was checked is in VerificationDTO.
type ControlsDTO struct {
	MinApprovers                int      `json:"min_approvers"`
	Checks                      []string `json:"checks"`
	DeployApproval              string   `json:"deploy_approval,omitempty"`
	RequireOwningTeamReviewer   bool     `json:"require_owning_team_reviewer,omitempty"`
	IndependentApproverRequired bool     `json:"independent_approver_required,omitempty"`
}

// VerificationDTO is the honesty invariant made machine-readable: which of the
// controls this run actually settled, and which it recorded as required without
// being able to check. Warnings on the record say the same thing in prose; these
// lists say it in a form a consumer can branch on without parsing English.
//
// Nothing here is a new judgement. Every entry is derived from a field the
// engine already set — see the identifiers documented in docs/contract.md.
type VerificationDTO struct {
	// ApproversSupplied reports whether the run had an authoritative approver
	// set. False means the approver controls below were recorded, not checked.
	ApproversSupplied   bool     `json:"approvers_supplied"`
	Verified            []string `json:"verified"`
	RecordedNotVerified []string `json:"recorded_not_verified"`
}

// ResultDTO is the gate's verdict, and the exit code the CLI returned with it.
// A run that ends in exit 2 produces no record at all, so only 0 and 1 appear.
type ResultDTO struct {
	Pass       bool     `json:"pass"`
	ExitCode   int      `json:"exit_code"`
	Violations []string `json:"violations"`
}

// Control identifiers used in VerificationDTO. Stable strings: a consumer keys
// off them, so they are part of the published contract, not free text.
const (
	ctlTier                = "CAF-SDLC-002:tier"
	ctlProvenanceComplete  = "CAF-SDLC-010:provenance-complete"
	ctlAIAuthorship        = "CAF-SDLC-010:ai-authorship"
	ctlApproverCount       = "CAF-SDLC-011:approver-count"
	ctlIndependentApprover = "CAF-SDLC-011:independent-approver"
	ctlOwningTeamReviewer  = "owning-team-reviewer"
	ctlRequiredChecks      = "required-checks"
	ctlDeployApproval      = "deploy-approval"
)

// EvidenceFromCore maps an evidence record onto the wire. It is a pure function:
// no I/O, no clock, no configuration — the same record always produces the same
// document, which is what makes the goldens in testdata/ meaningful.
func EvidenceFromCore(rec core.EvidenceRecord) EvidenceDTO {
	violations := rec.Violations()
	exit := 0
	if len(violations) > 0 {
		exit = 1
	}

	return EvidenceDTO{
		SchemaVersion: EvidenceSchema,
		Tool:          tool(),

		ChangeID:     rec.ChangeID,
		BindingUsed:  rec.BindingUsed,
		Tier:         rec.Tier.String(),
		AffectedSet:  orEmpty(rec.AffectedSet),
		UnmatchedSet: orEmpty(rec.UnmatchedSet),
		Reasons:      orEmpty(rec.Reasons),
		ControlsEnforced: ControlsDTO{
			MinApprovers:                rec.ControlsEnforced.MinApprovers,
			Checks:                      orEmpty(rec.ControlsEnforced.Checks),
			DeployApproval:              rec.ControlsEnforced.DeployApproval,
			RequireOwningTeamReviewer:   rec.ControlsEnforced.RequireOwningTeamReviewer,
			IndependentApproverRequired: rec.ControlsEnforced.IndependentApproverRequired,
		},

		AIAssisted: rec.AIAssisted,
		AITool:     rec.AITool,
		AISession:  rec.AISession,
		PromptRef:  rec.PromptRef,

		AccountableApprover: rec.AccountableApprover,
		ApproverNeAuthor:    rec.ApproverNeAuthor,
		ApproverCount:       rec.ApproverCount,

		Verification: verificationOf(rec),
		Result: ResultDTO{
			Pass:       len(violations) == 0,
			ExitCode:   exit,
			Violations: orEmpty(violations),
		},

		Warnings:   rec.Warnings,
		MapVersion: rec.MapVersion,
		ComputedAt: rec.ComputedAt,
	}
}

// verificationOf splits the controls this record touches into the ones the run
// settled and the ones it only recorded.
//
// The approver pair is driven by the same pointer the record uses to keep
// "unverified" apart from "verified false", so the two can never disagree. The
// rest are controls this engine structurally cannot check: it runs no CI jobs,
// resolves no team handles, approves no deployments, and reads provenance as a
// declaration rather than detecting it.
func verificationOf(rec core.EvidenceRecord) VerificationDTO {
	supplied := rec.ApproverNeAuthor != nil && rec.ApproverCount != nil

	verified := []string{ctlTier}
	// An AI-assisted change is checked for a tool name — a real check, and one
	// that fails the gate. What cannot be checked is the declaration itself.
	if rec.AIAssisted {
		verified = append(verified, ctlProvenanceComplete)
	}
	if supplied {
		verified = append(verified, ctlApproverCount, ctlIndependentApprover)
	}

	notVerified := []string{ctlAIAuthorship}
	if !supplied {
		notVerified = append(notVerified, ctlApproverCount, ctlIndependentApprover)
	}
	if rec.ControlsEnforced.RequireOwningTeamReviewer {
		notVerified = append(notVerified, ctlOwningTeamReviewer)
	}
	if len(rec.ControlsEnforced.Checks) > 0 {
		notVerified = append(notVerified, ctlRequiredChecks)
	}
	if rec.ControlsEnforced.DeployApproval != "" {
		notVerified = append(notVerified, ctlDeployApproval)
	}

	return VerificationDTO{
		ApproversSupplied:   supplied,
		Verified:            verified,
		RecordedNotVerified: notVerified,
	}
}
