# Contributing

## Build and test

```console
$ make build   # CGO_ENABLED=0 go build -o bin/sdlc-controls ./cmd/sdlc-controls
$ make test    # go test ./...
$ make lint    # go vet ./...
$ make demo    # ./scripts/demo.sh — tiers four changes in a throwaway TraderX-shaped repo
```

Go 1.22 or later. The binary has one dependency, `gopkg.in/yaml.v3`, used above `internal/core` only.
A JSON Schema validator (`github.com/santhosh-tekuri/jsonschema/v6`) is a **test-only** dependency —
it validates the emitted documents against the published schemas and must never reach the binary:

```console
$ go list -deps ./cmd/sdlc-controls | grep santhosh
# should print nothing
```

## The one architectural rule

`internal/core` must not import anything CI-, forge- or build-tool-specific — standard library only.
Everything that talks to git, reads a file, or knows what a pull request is belongs in `cmd/` or in a
CI adapter. This is what makes the tooling-agnostic claim checkable rather than aspirational:

```console
$ go list -deps ./internal/core | grep '\.' | grep -v github.com/codeaiforge
# should print nothing: no third-party or hosted-forge packages
```

## Changes to controls

A change to a control definition in `docs/controls/` or to the schemas in `config/` is a change to
what this repository asserts. State the reasoning in the pull request, and keep the stated ceiling of
a control honest — if a change makes a control weaker in some case, say so in the doc rather than in
the PR description alone.

## Changes to the published contract

`internal/contract` and `schemas/` are an interface other people depend on. The rules, in full in
[docs/contract.md](docs/contract.md):

- **Adding a field** is fine in any release. Add it to the DTO, the schema, and
  `docs/evidence-schema.md` together.
- **Removing or renaming** one means a new schema major (`evidence/1`), a new directory under
  `schemas/`, and a note in the CHANGELOG. Do not edit `evidence/0` to mean something else.
- Never marshal a `core` type straight onto the wire. The explicit DTO mapping is the review
  chokepoint that stops an engine refactor from silently reshaping a published document.
- Never let a control that was not checked reach the wire looking checked. `verification.verified`
  and `verification.recorded_not_verified` are derived from engine state, and every control belongs
  to exactly one of them.

The text output is a contract too, and `cmd/sdlc-controls/testdata/tier-t0.txt` is a golden of the
v0.1.0 output. If a change makes that test fail, the change is breaking — not the test.

## AI-assisted commits

On commits that were AI-generated or AI-assisted, add the provenance trailers:

```
AI-Assisted: true
AI-Tool: claude-code
AI-Session: <session id>
```

They are not required on every commit. They are required on the ones they apply to — a repository
that ships an AI-provenance control and leaves its own AI-authored commits unlabelled has answered the
question of how seriously to take it.

## Pull requests

The controls workflow tiers your PR and posts the tier and reasons to the job summary, with the
evidence record attached as an artifact. If the tier looks wrong, that is worth reporting: either the
component map is stale or the algorithm is.

Expect the gate to be red until your PR has the approvals its tier requires — it reads the actual
review state and re-runs when a review is submitted or dismissed. A T3 change needs two approvers, one
of whom is not you. That is the control doing its job on the repository that ships it, and the run
before anyone reviews is meant to fail.
