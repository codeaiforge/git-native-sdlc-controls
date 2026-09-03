package core

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// EvaluateInput is everything the engine needs. It is deliberately plain data:
// the caller (CLI, action, any CI adapter) is responsible for gathering it, and
// core stays free of any forge or build-tool knowledge.
type EvaluateInput struct {
	ChangeID       string
	ChangedPaths   []string
	CommitMessages []string
	Map            ComponentMap
	Policy         TierPolicy

	// GovernedPaths are the component-map and tier-policy files themselves,
	// named the way the diff names them — relative to the repository root.
	// Changing them self-escalates to the top tier (CAF-SDLC-002 map governance).
	GovernedPaths []string

	// UngovernablePaths are control files the caller could not express relative
	// to the repository under test — typically a component map kept outside it.
	// Governance cannot see them, and the record says so rather than leaving the
	// absent escalation to look like a change that never touched the map.
	UngovernablePaths []string

	// Author and Approvers feed CAF-SDLC-011. ApproversKnown records whether the
	// caller could supply an approver set at all; when it cannot, approver
	// controls are reported as unverified rather than silently passed.
	Author         string
	Approvers      []string
	ApproversKnown bool

	Now time.Time
}

// Evaluate is the single entrypoint: changed paths + declared map in, evidence
// record out. It performs no I/O.
func Evaluate(in EvaluateInput) (EvidenceRecord, error) {
	if err := ValidateMap(in.Map); err != nil {
		return EvidenceRecord{}, err
	}
	if err := ValidatePolicy(in.Policy); err != nil {
		return EvidenceRecord{}, err
	}

	affected, unmatched := ResolveComponents(in.ChangedPaths, in.Map)
	tier, reasons := ComputeTier(affected, unmatched, in.Map)

	if governed := governedChanges(in); len(governed) > 0 {
		if tier < MaxTier {
			tier = MaxTier
		}
		reasons = append(reasons, fmt.Sprintf("control map changed (%s) -> self-escalate to %s",
			strings.Join(governed, ", "), MaxTier))
	}

	prov := ParseProvenance(in.CommitMessages)
	rec := BuildEvidence(in, tier, affected, unmatched, reasons, prov)
	return rec, nil
}

// BuildEvidence assembles the portable record from an already-computed decision.
func BuildEvidence(in EvaluateInput, tier Tier, affected []Component, unmatched []string, reasons []string, prov Provenance) EvidenceRecord {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}

	ids := make([]string, 0, len(affected))
	for _, c := range affected {
		ids = append(ids, c.ID)
	}

	controls := in.Policy.For(tier)
	rec := EvidenceRecord{
		ChangeID:         in.ChangeID,
		BindingUsed:      BindingVersion,
		Tier:             tier,
		AffectedSet:      ids,
		UnmatchedSet:     append([]string{}, unmatched...),
		Reasons:          reasons,
		ControlsEnforced: controls,
		AIAssisted:       prov.AIAssisted,
		AITool:           prov.AITool,
		AISession:        prov.AISession,
		PromptRef:        prov.PromptRef,
		Warnings:         warnings(in, affected, unmatched, controls),
		MapVersion:       in.Map.Version,
		ComputedAt:       now.UTC(),
	}

	if in.ApproversKnown {
		approvers := distinctApprovers(in.Approvers)
		approver, independent := accountableApprover(in.Author, approvers)
		count := len(approvers)
		rec.AccountableApprover = approver
		rec.ApproverNeAuthor = &independent
		rec.ApproverCount = &count
	}

	return rec
}

// Violations lists the controls this record demonstrably fails. Checks declared
// in the policy (lint, sast, …) are executed and enforced by CI, not here; this
// only reports what the evidence itself can settle.
func (r EvidenceRecord) Violations() []string {
	var v []string
	if r.AIAssisted && r.AITool == "" {
		v = append(v, "AI-assisted change is missing an AI-Tool trailer (CAF-SDLC-010)")
	}
	if r.ApproverNeAuthor == nil || r.ApproverCount == nil {
		return v // approver set unknown — reported as a warning instead
	}
	if *r.ApproverCount < r.ControlsEnforced.MinApprovers {
		v = append(v, fmt.Sprintf("%s requires %d approver(s), found %d",
			r.Tier, r.ControlsEnforced.MinApprovers, *r.ApproverCount))
	}
	if r.ControlsEnforced.IndependentApproverRequired && !*r.ApproverNeAuthor {
		v = append(v, fmt.Sprintf("%s requires an approver distinct from the author (CAF-SDLC-011)", r.Tier))
	}
	return v
}

// distinctApprovers folds an approver list to one entry per identity: trimmed,
// compared case-insensitively, first spelling kept.
//
// min_approvers is a count of approvers, not of list entries, and the engine
// cannot assume its caller already deduplicated. The two workflows shipped here
// do (`jq unique`), but the documented way to reach a new CI system is a job
// definition rather than a second implementation, and such a job passes on
// whatever the forge API returned. Counting entries would let one reviewer
// listed twice satisfy a two-approver tier, and would put "approver_count": 2
// in the evidence when one human approved — the record asserting something that
// did not happen, which is the failure this tool exists to prevent.
//
// Identity is still a handle: this collapses "alice" and "Alice", not one human
// with two accounts. See docs/controls/CAF-SDLC-011-independent-approver.md.
func distinctApprovers(approvers []string) []string {
	seen := make(map[string]struct{}, len(approvers))
	out := make([]string, 0, len(approvers))
	for _, a := range approvers {
		if a = strings.TrimSpace(a); a == "" {
			continue
		}
		key := strings.ToLower(a)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, a)
	}
	return out
}

// accountableApprover picks the approver that satisfies segregation of duties:
// the first approver who is not the author. Falls back to the first approver so
// the record still names someone accountable when none is independent.
func accountableApprover(author string, approvers []string) (string, bool) {
	for _, a := range approvers {
		if !strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(author)) {
			return a, true
		}
	}
	if len(approvers) > 0 {
		return approvers[0], false
	}
	return "", false
}

func governedChanges(in EvaluateInput) []string {
	if len(in.GovernedPaths) == 0 {
		return nil
	}
	governed := map[string]struct{}{}
	for _, p := range in.GovernedPaths {
		governed[normalizePath(p)] = struct{}{}
	}
	var hits []string
	for _, p := range in.ChangedPaths {
		if _, ok := governed[normalizePath(p)]; ok {
			hits = append(hits, normalizePath(p))
		}
	}
	sort.Strings(hits)
	return hits
}

func warnings(in EvaluateInput, affected []Component, unmatched []string, controls TierControls) []string {
	var w []string
	for _, c := range affected {
		if len(c.CrossRepoConsumers) > 0 {
			w = append(w, fmt.Sprintf("%s has cross-repo consumers (%s): downstream impact is outside this repo's diff",
				c.ID, strings.Join(c.CrossRepoConsumers, ", ")))
		}
	}
	if len(unmatched) > 0 {
		w = append(w, fmt.Sprintf("%d changed path(s) matched no component — the component map may be stale", len(unmatched)))
	}
	for _, p := range in.UngovernablePaths {
		w = append(w, fmt.Sprintf("control file %s is outside the repository under test: map-governance self-escalation cannot apply to it", p))
	}
	if !in.ApproversKnown {
		w = append(w, "approver set not supplied: approver controls recorded but not verified by this run")
	}
	// The owning-team rule is enforced by CODEOWNERS and branch protection: this
	// tool cannot expand a team handle into members without a forge API, and will
	// not record what it did not check.
	if controls.RequireOwningTeamReviewer {
		who := "no owners declared in the component map"
		if owners := ownersOf(affected); len(owners) > 0 {
			who = strings.Join(owners, ", ")
		}
		w = append(w, fmt.Sprintf("tier requires an owning-team reviewer (%s): enforced by CODEOWNERS and branch protection, not verified by this run", who))
	}
	return w
}

// ownersOf lists the declared owners of the affected components, deduplicated
// and sorted so the warning is stable across runs.
func ownersOf(affected []Component) []string {
	seen := map[string]struct{}{}
	var owners []string
	for _, c := range affected {
		for _, o := range c.Owners {
			if _, ok := seen[o]; ok {
				continue
			}
			seen[o] = struct{}{}
			owners = append(owners, o)
		}
	}
	sort.Strings(owners)
	return owners
}
