# Changelog

All notable changes to this project are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning: SemVer for the
binary; the JSON schemas are versioned independently — see [docs/contract.md](docs/contract.md).

## [Unreleased]

## [0.2.0] - 2026-09-15

### Added

- `sdlc-controls binding` prints the active policy binding — the tier table and the
  CAF-SDLC-002 escalation rules — as text or as schema `policy-binding/0` JSON. It is a
  projection of the same map and policy the engine tiers against, not a second copy.
- Published JSON Schemas, Draft 2020-12, committed in-repo and validated in CI against
  the JSON the tool actually emits: [`schemas/evidence/0`](schemas/evidence/0) and
  [`schemas/policy-binding/0`](schemas/policy-binding/0). Both are experimental at major 0
  and leave `additionalProperties` open at every extension point, so a consumer can pin a
  schema and keep validating documents from a newer binary. The control identifiers are an
  open set for the same reason. Only the sets the algorithm fixes — the tier scale and
  `result.exit_code` — are closed. That the producer emits nothing undocumented is enforced
  by a test rather than by a closed schema.
- Evidence records gain four additive fields:
  - `schema_version` — the document shape, versioned independently of the binary;
  - `tool` — the name and version of the binary that produced the record;
  - `verification` — `verified` and `recorded_not_verified` control identifiers, the
    machine-readable form of what this run checked versus what it only recorded;
  - `result` — `pass`, `exit_code` and the list of violations, so a consumer no longer
    has to observe a process exit code to know whether the gate blocked the change.
- [`docs/contract.md`](docs/contract.md) and
  [ADR-0001](docs/adr/0001-json-output-and-versioned-contract.md) document the contract,
  the three version axes, and the stability promise.

### Changed

- The JSON path now serializes explicit wire DTOs in `internal/contract` rather than
  marshalling `core.EvidenceRecord` directly. No field a v0.1.0 consumer reads has moved
  or changed meaning; the mapping exists so a future rename inside the engine breaks a
  build instead of a consumer.
- `core.TierIncrement` names the single-tier escalation step the algorithm already
  applied, so the published binding can state the engine's own value.

### Notes

- **Additive release.** Text output is byte-identical to v0.1.0 and remains the default;
  a golden test enforces it. Exit codes are unchanged in both formats, and `--format json`
  still prints its record on a failing gate.
- The JSON Schema validator is a test-only dependency. The binary is still a single
  static artifact whose only runtime dependency is `gopkg.in/yaml.v3`.
- Schemas stay experimental (major 0) while the binary is 0.x.

## [0.1.0] - 2026-09-03

- Initial reference implementation: change-risk tiering from a declared component map
  (CAF-SDLC-002), AI provenance from git trailers (CAF-SDLC-010), the independent-approver
  control (CAF-SDLC-011), the evidence record, the CLI, the Docker action and the TraderX demo.

[Unreleased]: https://github.com/codeaiforge/git-native-sdlc-controls/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/codeaiforge/git-native-sdlc-controls/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/codeaiforge/git-native-sdlc-controls/releases/tag/v0.1.0
