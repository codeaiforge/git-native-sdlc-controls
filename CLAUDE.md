# CLAUDE.md — project brief

Read this before changing anything in this repository.

## 0. GUARDRAIL — read first (public/private boundary)

This repo is the **public engine**. The following are a separate **PRIVATE** corpus (business moat)
and must **never** be created or committed here:

- the clause-precise DORA / RTS (EU) 2024/1774 control mapping (article + point citations)
- the maturity scoring rubric and the readiness-assessment methodology
- any client-facing offer, pricing, positioning, or go-to-market material
- any client data, client repo names, or engagement details

If any task would introduce the above, **STOP**. Keep this repo to: the engine, the agnostic control
definitions, the evidence schema, and the TraderX demo. Refer to DORA only lightly ("supports DORA
change-management outcomes") — never with article/point precision.

## 1. What the project is

A git-native, tooling-agnostic reference implementation of SDLC controls-as-code. It assigns a risk
tier to every change and enforces proportionate controls (review, approval, evidence) in CI, using
only git primitives — changed paths and a declared component map — with **no build-tool dependency**.
Reference implementation, not a product. Demo target: **FINOS TraderX**.

`CODEOWNERS` is **not** an input: nothing in the engine reads it, ownership comes from the `owners`
field on a component, and enforcement of owner review belongs to the forge. Do not reintroduce the
claim without the code to back it.

Controls in scope (v0.1): **CAF-SDLC-002** (change risk tiering by blast radius),
**CAF-SDLC-010** (AI-generated change provenance), **CAF-SDLC-011** (independent approver for AI
changes).

## 2. Conventions

- **Language:** Go. Module path `github.com/codeaiforge/git-native-sdlc-controls`. Go 1.22+.
- **Core is CI-agnostic:** package `internal/core` is pure — it must **not** import any
  CI/forge/build-tool specifics. This split embodies the tooling-agnostic claim.
- **The CLI is the product;** the GitHub Action is a Docker-based wrapper around the same binary.
- **Single static binary** is the primary release artifact (`CGO_ENABLED=0`).
- **License:** Apache-2.0.
- **Determinism:** every tier decision emits an ordered reason list.
- **Dogfood:** the repo enforces its own controls in `.gitea/workflows/controls.yml` (where its
  pull requests are actually reviewed) and `.github/workflows/controls.yml` (the mirror). Same
  gate, same binary, same flags; they differ only in how the approver set is read out of the
  forge API, and in the artifact action version (Gitea 1.22 answers as GHES, so it needs
  `upload-artifact@v3`). Change one, change the other. Both fire on pushes to `main` as well as pull requests:
  a push run tiers and records but cannot verify approvals, and must not pass `--approvers-known`.
- **Provenance (soft):** on **AI-assisted** commits, use trailers `AI-Assisted:`, `AI-Tool:`,
  `AI-Session:`. Not required on every commit — but do not claim provenance-tracking while leaving
  AI-authored commits unlabelled. Commits are authored manually; Claude Code should add these trailers
  to commits it proposes.
- **FINOS:** "aligned to the emerging FINOS SDLC controls framework" — not endorsed by, or part of,
  FINOS. No `finos-` prefix.

## 3. Architecture principles

- **`internal/core` is pure.** Standard library only: no CI, no forge, no build tool, no I/O. It takes
  changed paths, commit messages and a declared map, and returns a tier, ordered reasons and an
  evidence record. `go list -deps ./internal/core` is the check, not a code review convention.
- **The CLI is the product.** `cmd/sdlc-controls` does all the I/O: shelling out to git, reading YAML,
  formatting output, choosing an exit code.
- **The Docker action wraps the binary.** `action/` has no logic. A GitLab, Jenkins or Azure adapter
  is a job definition, not a second implementation.
- **Git-native only.** If a feature needs `mvn`, `npm` or `bazel` to work, it does not belong here —
  that dependency is exactly what stops controls reaching the repositories that need them most.
- **Deterministic.** Same inputs → same tier *and* same reason list, in the same order. Sets are
  sorted; diff order never affects the outcome.

## 4. The honest ceiling (CAF-SDLC-002)

Blast radius is a **declared-topology approximation** — a human-maintained `criticality` and a
`shared` boolean — not a computed dependency graph. Consequences, to be stated plainly wherever the
control is described:

- an undeclared high fan-in component is **under-tiered**;
- the map drifts from the repository unless it is maintained;
- `shared` is a boolean stand-in for a fan-in count.

Two mitigations keep it honest without pretending it is a graph: unmatched paths fail safe to
`unmatched_path_tier`, and changes to the map or policy self-escalate to the top tier. Never claim
graph-grade accuracy for this.

## 5. Evidence record

One record per change; the shape is fixed by `docs/evidence-schema.md`:

```json
{
  "change_id": "PR-123",
  "binding_used": "git-native-baseline@1",
  "tier": "T3",
  "affected_set": ["payment-authorization", "shared-contracts"],
  "unmatched_set": [],
  "reasons": ["payment-authorization criticality=critical -> base T3",
              "shared-contracts shared=true -> +1 (capped)"],
  "controls_enforced": { "min_approvers": 2, "independent_approver_required": true,
                         "checks": ["lint","sast","secrets","deps"] },
  "ai_assisted": false,
  "map_version": 1,
  "computed_at": "2026-07-30T09:00:00Z"
}
```

`approver_ne_author` is a pointer in Go and omitted in JSON when the run had no approver set to check:
an unverified control is never recorded as passed.

## 6. Demo

FINOS TraderX (`examples/traderx/`): polyglot, independently deployed services — the case where a
build-tool-based approach needs four integrations and this one needs none. The walkthrough contrasts a
`web-client` change passing at T0 against a one-line `reference-data` change escalating to T3 because
that component is declared `shared`.

## 7. Definition of done — v0.1

- `go build ./...`, `go vet ./...` and `go test ./...` all pass.
- `internal/core` imports nothing outside the standard library.
- The tier algorithm implements §002 exactly, including the cap, the fail-safe floor and map
  governance, with an ordered reason for every rule that fires.
- Table-driven tests cover: T0 leaf change, `shared` escalation, the T3 cap, breadth escalation, the
  unmatched-path fail-safe, trailer parsing, and the independent-approver control.
- The CLI exits 0 / 1 / 2 (controls met / not met / error) and can write the evidence record to a file.
- `.gitea/workflows/controls.yml` and `.github/workflows/controls.yml` tier this repository's own
  pull requests against `config/components.yaml`, post the tier and reasons, and upload the evidence
  artifact.
- No file from the §0 private list exists anywhere in the repo.

## 8. Do not

- Do not create the private DORA catalogue / methodology / GTM material.
- Do not add product branding or imply a commercial product.
- Do not imply FINOS endorsement or use a `finos-` prefix.
- Do not make `internal/core` depend on any CI platform or build tool.
- Do not overclaim graph accuracy — this is the declared-topology approximation.
- Do not record an unverified control as passed.
