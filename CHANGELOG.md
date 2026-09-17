# Changelog

All notable changes to this project are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning: SemVer for the
binary; the JSON schemas are versioned independently — see [docs/contract.md](docs/contract.md).

## [Unreleased]

### Added

- Published JSON Schema for the component map: [`component-map/0`](schemas/component-map/0).
  The map is the tool's input rather than something it emits, and it was previously described
  only by Go structs, an example file and prose — three things that could disagree. It versions
  separately from the evidence major, because input evolves on its own schedule.
- `schema_version` on the component map, optional and absent meaning `component-map/0`. It
  exists so a map written for a later format is refused by an older binary instead of being
  read on a guess, which would under-tier silently. It settles the collision with `version`,
  which stays what it always was: the revision of the declared topology, recorded in evidence
  as `map_version` for reproducibility.
- `provenance.generated_by` on the component map: the paths that produce a generated map, which
  join the governed set so a change to the generator self-escalates exactly as a change to the
  map does. Map governance previously assumed the map was a committed file, which fails for a map
  projected out of a build graph — the generated file may not be in the diff, and the generator is.
  Declaring the generator as an ordinary `critical` component reached the same tier but made the
  evidence state the wrong reason for it.
- Two hard errors rather than warnings, both because a governed path that never matches leaves a
  map that looks governed and is not: `ValidateMap` refuses a `generated_by` path no diff could
  name (absolute, or leaving the repository), and the CLI refuses one that is not in the
  repository under test.
- A test asserting `component-map/0` and `core.ValidateMap` reject the same maps. A schema a
  generator can satisfy while the engine refuses the result is worse than no schema. The two
  rules JSON Schema cannot express — id uniqueness, and refusing an unknown `schema_version` —
  are named in the schema as engine-enforced rather than quietly missing.

### Notes

- Additive. Every existing map keeps loading unchanged, and no evidence field moved: closing
  this finding needed no change to `evidence/0`.
- Two of the three open findings recorded in [docs/contract.md](docs/contract.md) are now closed.
  The third — fan-in as a count rather than a boolean — stays open on purpose: it is the only one
  that changes how a tier is computed, and it waits for a second caller to confirm the shape.
- `internal/core` gained a field and its validation, and no governance logic: `GovernedPaths`
  already governed any path it was handed, and the gap was only in filling it.

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
