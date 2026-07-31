package core

import (
	"fmt"
	"strings"
)

// ComputeTier implements the CAF-SDLC-002 resolution algorithm over an already
// resolved affected/unmatched split:
//
//  1. base_tier = max(criticality) over the affected set (low=0 … critical=3).
//  2. any component with shared:true       -> +1   (high fan-in proxy)
//  3. len(affected) >= breadth_threshold   -> +1   (breadth of change)
//  4. unmatched paths present              -> tier = max(tier, unmatched_path_tier)
//
// Escalations are capped at T3. The returned reasons are ordered by the step that
// produced them, so two runs over the same input always yield the same list.
//
// This is a declared-topology approximation of blast radius, not a computed
// dependency graph — see docs/controls/CAF-SDLC-002-change-risk-tiering.md.
func ComputeTier(affected []Component, unmatched []string, m ComponentMap) (Tier, []string) {
	reasons := make([]string, 0, 4)

	if len(affected) == 0 && len(unmatched) == 0 {
		return T0, append(reasons, "no changed paths -> T0")
	}

	tier := T0
	if len(affected) > 0 {
		driver := affected[0]
		for _, c := range affected[1:] {
			if c.Criticality.Level() > driver.Criticality.Level() {
				driver = c
			}
		}
		tier = Tier(driver.Criticality.Level())
		reasons = append(reasons, fmt.Sprintf("%s criticality=%s -> base %s", driver.ID, driver.Criticality, tier))
	}

	var sharedIDs []string
	for _, c := range affected {
		if c.Shared {
			sharedIDs = append(sharedIDs, c.ID)
		}
	}
	if len(sharedIDs) > 0 {
		tier, reasons = escalate(tier, reasons, fmt.Sprintf("%s shared=true -> +1", strings.Join(sharedIDs, ", ")))
	}

	if thr := m.Defaults.BreadthThreshold; thr > 0 && len(affected) >= thr {
		tier, reasons = escalate(tier, reasons,
			fmt.Sprintf("affected_set size=%d >= breadth_threshold=%d -> +1", len(affected), thr))
	}

	if len(unmatched) > 0 {
		crit := UnmatchedPathTier(m)
		floor := Tier(crit.Level())
		state := "already met"
		if floor > tier {
			tier = floor
			state = "applied"
		}
		reasons = append(reasons, fmt.Sprintf("%d unmatched path(s) -> fail-safe floor %s %s (unmatched_path_tier=%s)",
			len(unmatched), floor, state, crit))
	}

	return tier, reasons
}

// escalate raises a tier by one, capped at T3, and records why.
func escalate(t Tier, reasons []string, reason string) (Tier, []string) {
	if t >= MaxTier {
		return MaxTier, append(reasons, reason+" (capped)")
	}
	return t + 1, append(reasons, reason)
}

// UnmatchedPathTier returns the configured fail-safe class for undeclared paths.
// An unset value defaults to high: an undeclared path is the case the map cannot
// reason about, so it must fail safe rather than fail open.
func UnmatchedPathTier(m ComponentMap) Criticality {
	if m.Defaults.UnmatchedPathTier == "" {
		return CriticalityHigh
	}
	return m.Defaults.UnmatchedPathTier
}
