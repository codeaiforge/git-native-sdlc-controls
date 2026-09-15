// SPDX-License-Identifier: Apache-2.0

// Package contract is the serialization boundary. It imports internal/core and
// maps the engine's domain types onto versioned wire DTOs; nothing maps the
// other way, and core knows nothing about this package.
//
// The DTOs are written out explicitly rather than derived by tagging the core
// structs. The published shape is an interface other people depend on, so a
// rename inside the engine has to break this mapping — and show up in review —
// instead of silently changing what a consumer receives. See
// docs/adr/0001-json-output-and-versioned-contract.md.
package contract

const (
	// EvidenceSchema names the shape of the evidence record. It versions the
	// document, not the binary and not the policy: see docs/contract.md for the
	// three axes and why they move independently.
	EvidenceSchema = "evidence/0"

	// PolicyBindingSchema names the shape of the serialized policy binding.
	PolicyBindingSchema = "policy-binding/0"

	// ToolName and ToolVersion identify the binary that produced a document, so a
	// record stays attributable to a specific implementation years later.
	//
	// A plain const, not an ldflags-injected var — the release is cut
	// from a tag that matches it. Wire it to -X if builds ever diverge from tags.
	ToolName    = "sdlc-controls"
	ToolVersion = "0.2.0"
)

// ToolDTO identifies the producer of a document.
type ToolDTO struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func tool() ToolDTO { return ToolDTO{Name: ToolName, Version: ToolVersion} }

// orEmpty renders an absent list as [] rather than null: a consumer reading
// "unmatched_set": null has to special-case a value that only means "none".
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
