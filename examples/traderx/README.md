# Demo: FINOS TraderX

[TraderX](https://github.com/finos/traderX) is a reference trading application with a handful of
independently deployed services. It is a good demo target because it is polyglot (Java, .NET, Node,
Python) — exactly the case where a build-tool-based tiering approach needs four integrations and a
git-native one needs none.

`components.yaml` here maps TraderX's services onto declared criticality. `reference-data` is marked
`shared: true` because other services depend on it; the criticality values are read off the project's
architecture-as-code description of which service calls which.

*This repository is aligned to the emerging FINOS SDLC controls framework. It is not endorsed by, or
part of, FINOS. TraderX is used here only as a public, realistic demo target.*

## Run it

```console
$ git clone https://github.com/finos/traderX && cd traderX
$ cp ../git-native-sdlc-controls/examples/traderx/components.yaml .
$ sdlc-controls tier --base main --head my-branch --config components.yaml
```

Copy the map into the repository being tiered rather than pointing at it across a directory boundary:
a control file outside the repository under test cannot be seen by the diff, so map-governance
self-escalation could not apply to it. The tool says so in the evidence when that happens.

## Walkthrough: two changes, two answers

### A trivial UI change passes at T0

A change touching `web-client/src/app/trades.component.ts`:

```console
$ sdlc-controls tier --base main --head ui-tweak --config components.yaml
change:  main..ui-tweak
tier:    T0
binding: git-native-baseline@1
affected: web-client
unmatched: (none)
reasons:
  - web-client criticality=low -> base T0
required: min_approvers=1 checks=lint
ai_assisted: false
warning: approver set not supplied: approver controls recorded but not verified by this run
```

One reviewer, a linter, merge. The controls stay proportionate to the change — which is the point:
a framework that treats every change as high risk gets routed around within a quarter.

### A shared-service change escalates and demands an independent approver

A change touching `reference-data/src/main/java/.../SecurityRepository.java`:

```console
$ sdlc-controls tier --base main --head refdata-fix --config components.yaml
change:  main..refdata-fix
tier:    T3
binding: git-native-baseline@1
affected: reference-data
unmatched: (none)
reasons:
  - reference-data criticality=high -> base T2
  - reference-data shared=true -> +1
required: min_approvers=2 checks=lint, sast, secrets, deps independent_approver=true
ai_assisted: false
warning: approver set not supplied: approver controls recorded but not verified by this run
warning: tier requires an owning-team reviewer (no owners declared in the component map): enforced by CODEOWNERS and branch protection, not verified by this run
```

The same one-line diff, in a service other services depend on, needs two approvers, the full check
set, and — if the commits carry `AI-Assisted: true` — an approver who is not the author
(CAF-SDLC-011). Nobody filed a change request to make that happen; the tier came from the diff.

Note what the tool refuses to claim. It was given no approver set and this map declares no `owners`,
so it records both controls as required-and-unverified instead of passing them. Add `owners:` to the
map and pass `--approvers` in CI and those two lines become checks rather than warnings.

### What the demo does not show

`reference-data` escalates because someone **declared** `shared: true`. If a new high fan-in service
is added to TraderX and nobody updates this map, it is tiered on its own criticality alone and is
under-tiered. That is the declared-topology ceiling, stated plainly in
[CAF-SDLC-002](../../docs/controls/CAF-SDLC-002-change-risk-tiering.md); changes to this map
self-escalate to T3 precisely because the map is the thing that can go stale.
