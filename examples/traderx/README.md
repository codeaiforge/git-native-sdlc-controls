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
$ make demo
```

That builds a throwaway repository shaped like TraderX, commits four changes to it, tiers each one
against `components.yaml`, and refreshes the evidence records in [evidence/](evidence/). It needs
git and Go and nothing else: no network, no clone of TraderX, and — the point of the exercise — no
Java, .NET, Node or Python toolchain for the services it is tiering. The script is
[scripts/demo.sh](../../scripts/demo.sh); the four records it writes are committed, so the output of
a run is readable before you make one.

To run it against the real repository instead:

```console
$ git clone https://github.com/finos/traderX && cd traderX
$ cp ../git-native-sdlc-controls/examples/traderx/components.yaml .
$ sdlc-controls tier --base main --head my-branch --config components.yaml
```

Copy the map into the repository being tiered rather than pointing at it across a directory boundary:
a control file outside the repository under test cannot be seen by the diff, so map-governance
self-escalation could not apply to it. The tool says so in the evidence when that happens.

## Walkthrough: four changes, four answers

The transcripts below are the output of `make demo`, verbatim.

### A trivial UI change passes at T0

One line in `web-client/src/app/trades.component.ts`, one reviewer:

```console
$ sdlc-controls tier --base main --head ui-tweak --config components.yaml --change-id DEMO-t0-web-client --evidence-out evidence/t0-web-client.json --author alice --approvers bob
change:  DEMO-t0-web-client
tier:    T0
binding: git-native-baseline@1
affected: web-client
unmatched: (none)
reasons:
  - web-client criticality=low -> base T0
required: min_approvers=1 checks=lint
ai_assisted: false
exit: 0 — controls met
```

One reviewer, a linter, merge. The controls stay proportionate to the change — which is the point:
a framework that treats every change as high risk gets routed around within a quarter.

### The same size diff in a shared service escalates to T3

One line in `reference-data/src/main/java/SecurityRepository.java`, the same single reviewer:

```console
$ sdlc-controls tier --base main --head refdata-fix --config components.yaml --change-id DEMO-t3-reference-data --evidence-out evidence/t3-reference-data.json --author alice --approvers bob
change:  DEMO-t3-reference-data
tier:    T3
binding: git-native-baseline@1
affected: reference-data
unmatched: (none)
reasons:
  - reference-data criticality=high -> base T2
  - reference-data shared=true -> +1
required: min_approvers=2 checks=lint, sast, secrets, deps independent_approver=true
ai_assisted: false
warning: tier requires an owning-team reviewer (no owners declared in the component map): enforced by CODEOWNERS and branch protection, not verified by this run
FAIL: T3 requires 2 approver(s), found 1
exit: 1 — controls not met, the gate blocks the merge
```

Same diff size, different answer: a service other services depend on needs two approvers and the full
check set. Nobody filed a change request to make that happen — the tier came from the diff, and the
gate exits 1.

### An AI-assisted change at T3 cannot be approved by its own author

The same `reference-data` fix, this time with CAF-SDLC-010 trailers on the commit, approved by alice
who wrote it:

```console
$ sdlc-controls tier --base main --head refdata-ai --config components.yaml --change-id DEMO-t3-ai-self-approved --evidence-out evidence/t3-ai-self-approved.json --author alice --approvers alice
change:  DEMO-t3-ai-self-approved
tier:    T3
binding: git-native-baseline@1
affected: reference-data
unmatched: (none)
reasons:
  - reference-data criticality=high -> base T2
  - reference-data shared=true -> +1
required: min_approvers=2 checks=lint, sast, secrets, deps independent_approver=true
ai_assisted: true (tool=claude-code)
warning: tier requires an owning-team reviewer (no owners declared in the component map): enforced by CODEOWNERS and branch protection, not verified by this run
FAIL: T3 requires 2 approver(s), found 1
FAIL: T3 requires an approver distinct from the author (CAF-SDLC-011)
exit: 1 — controls not met, the gate blocks the merge
```

Two separate failures: it is short an approver, and short an *independent* one. The provenance is not
inferred — it is read from the `AI-Assisted:`, `AI-Tool:`, `AI-Session:` and `Prompt-Ref:` trailers the
commit carries, so it travels with the change into any other repository or tool.

### With an independent approver, it passes

```console
$ sdlc-controls tier --base main --head refdata-ai --config components.yaml --change-id DEMO-t3-ai-independent --evidence-out evidence/t3-ai-independent.json --author alice --approvers alice,carol
change:  DEMO-t3-ai-independent
tier:    T3
binding: git-native-baseline@1
affected: reference-data
unmatched: (none)
reasons:
  - reference-data criticality=high -> base T2
  - reference-data shared=true -> +1
required: min_approvers=2 checks=lint, sast, secrets, deps independent_approver=true
ai_assisted: true (tool=claude-code)
warning: tier requires an owning-team reviewer (no owners declared in the component map): enforced by CODEOWNERS and branch protection, not verified by this run
exit: 0 — controls met
```

[`evidence/t3-ai-independent.json`](evidence/t3-ai-independent.json) records who cleared it, and that
they were not the author:

```json
  "ai_assisted": true,
  "ai_tool": "claude-code",
  "ai_session": "demo-4f21",
  "prompt_ref": "TRADERX-412",
  "accountable_approver": "carol",
  "approver_ne_author": true,
  "approver_count": 2,
```

## What the tool refuses to claim

The owning-team warning survives every T3 run above, the passing one included. Expanding a team handle
into its members needs a forge API, so the tool records that rule as required-and-unverified rather
than passed; declaring `owners:` in the map makes the warning name the team, it does not make the
check happen. Branch protection enforces it.

The approver controls are only checked because the demo passes `--approvers`. Drop the flag and a
warning replaces the verdict, and `approver_ne_author` and `approver_count` vanish from the record
entirely rather than defaulting to something reassuring. That is the shape of every gap here:
unverified is written down, never silently satisfied.

## What the demo does not show

`reference-data` escalates because someone **declared** `shared: true`. If a new high fan-in service
is added to TraderX and nobody updates this map, it is tiered on its own criticality alone and is
under-tiered. That is the declared-topology ceiling, stated plainly in
[CAF-SDLC-002](../../docs/controls/CAF-SDLC-002-change-risk-tiering.md); changes to this map
self-escalate to T3 precisely because the map is the thing that can go stale.
