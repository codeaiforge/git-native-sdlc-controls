# 1. A published, independently-versioned contract for the JSON output

- Status: accepted
- Date: 2026-09-15

## Context

v0.1.0 already emitted JSON: `tier --format json` and `--evidence-out` marshalled
`core.EvidenceRecord` directly, and `docs/evidence-schema.md` described the result in
a table. That is a contract in everything but name — consumers can depend on it, and
nothing stopped an internal refactor from changing it.

Three things were missing before it could honestly be called one:

- **No version.** A document gave no way to tell which shape it was, so a consumer
  could not branch on it and a producer could not evolve it.
- **No machine-readable honesty.** The distinction the tool exists to make — what this
  run *verified* against what it *recorded but could not check* — reached the wire only
  as absent pointer fields and a list of English warnings. A consumer had to parse
  prose to learn that an approver control had not been checked.
- **No way to read the policy.** A record named `git-native-baseline@1` and stopped
  there. The tier table and escalation rules that produced the decision lived in a Go
  var and a YAML file the consumer might never see.

## Decision drivers

- The contract must outlive internal refactors of `internal/core`.
- A binary patch release must not look like a contract change.
- The tool's honesty about verified-versus-recorded must survive serialization.
- The pure, dependency-free core and single-static-binary property must be preserved.
- v0.1.0 consumers must not break. This release is additive.

## Considered options

1. Keep marshalling core structs, and document the result harder.
2. A dedicated `internal/contract` package with explicit wire DTOs and mapping.
3. Emit an ad-hoc JSON blob from the CLI with no published schema.

## Decision outcome

Chosen: **option 2**, with published JSON Schemas and a `schema_version` envelope
independent of the binary version.

- Wire DTOs are written out explicitly and mapped from core types. The mapping is the
  chokepoint: a rename in the engine breaks compilation of `EvidenceFromCore` and is
  visible in review, rather than silently changing what a consumer receives.
- Three version axes: binary (SemVer, git tag), schema (`evidence/0`,
  `policy-binding/0`), policy instance (`git-native-baseline@1`).
- Schemas ship experimental at major 0 while the binary is 0.x; they stabilise to
  major 1 at binary v1.0.
- `verification.verified` / `verification.recorded_not_verified` carry the honesty
  invariant structurally. They are derived from fields the engine already sets — the
  approver pointers and the tier's own controls — never from a new judgement.
- The JSON Schema validator is a test-only dependency and never enters the binary.
  `go list -deps ./cmd/sdlc-controls` is the check.

### Field names: kept, not modernised

The DTO reuses the snake_case names v0.1.0 published (`binding_used`, `affected_set`,
`controls_enforced`). Renaming them to a house style would have broken every existing
consumer, `docs/evidence-schema.md`, and the committed example records that CI diffs
against a live run — a breaking change bought with nothing but taste. `evidence/0`
therefore describes the shape that already existed, plus four additive fields:
`schema_version`, `tool`, `verification` and `result`.

### What the policy binding deliberately does not say

The binding document has no "independent approver required when AI-generated" flag.
CAF-SDLC-011 attaches that requirement to the **tier** — the baseline sets it on T3
alone, for AI-assisted and human changes alike. A field asserting otherwise would be a
published claim the engine does not implement, which is the failure mode this whole
tool exists to prevent.

## Consequences

- Good: contract stability decoupled from internal change; auditable, versioned evidence.
- Good: core stays pure; the binary stays a single static artifact with one dependency.
- Good: `result` now states pass, exit code and violations, so a consumer no longer has
  to observe a process exit code to know whether the gate blocked the change.
- Cost: an explicit DTO and mapping to maintain, and a deliberate step to evolve the schema.
- Cost: `core.TierIncrement` was named so the binding could state the engine's own value
  instead of carrying a second copy. A small intrusion into core, and the alternative was
  a constant that could silently disagree with the algorithm.
- Follow-up: revisit the stability promise before v1.0 (see `docs/contract.md`).
