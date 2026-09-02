# Contributing

## Build and test

```console
$ make build   # CGO_ENABLED=0 go build -o bin/sdlc-controls ./cmd/sdlc-controls
$ make test    # go test ./...
$ make lint    # go vet ./...
$ make demo    # ./scripts/demo.sh — tiers four changes in a throwaway TraderX-shaped repo
```

Go 1.22 or later. The only dependency is `gopkg.in/yaml.v3`, and it is used above `internal/core`
only.

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
