# The published contract

Two JSON documents are a contract other people are invited to depend on:

| Document | Schema | Emitted by |
|---|---|---|
| **evidence record** — one per change: the tier, why, what it required, what was verified | [`schemas/evidence/0/evidence.schema.json`](../schemas/evidence/0/evidence.schema.json) | `sdlc-controls tier --format json`, and `--evidence-out <path>` |
| **policy binding** — the active tier table and escalation rules | [`schemas/policy-binding/0/policy-binding.schema.json`](../schemas/policy-binding/0/policy-binding.schema.json) | `sdlc-controls binding --config <map> [--policy <p>] --format json` |

Field-by-field notes on the evidence record are in [evidence-schema.md](evidence-schema.md).
The reasoning behind the split is [ADR-0001](adr/0001-json-output-and-versioned-contract.md).

## Three version axes, and why they are separate

| Axis | Spelling | What it means | Moves when |
|---|---|---|---|
| **Binary** | `0.2.0`, in `tool.version` and the git tag | which implementation produced the document | any release |
| **Schema** | `evidence/0`, `policy-binding/0`, in `schema_version` | the *shape* of the document | the shape changes incompatibly |
| **Policy instance** | `git-native-baseline@1`, in `binding` / `binding_used` | the engine binding the decision was made under | the tiering algorithm or record semantics change |

Collapsing any two of these loses information a consumer needs. A patch release of the
binary must not read as a contract change; a schema fix must not imply the algorithm
moved; and a decision has to stay reproducible — `binding_used` plus `map_version` are
what let a past tier be recomputed years later.

The two schemas version independently of each other too. `evidence/1` alongside
`policy-binding/0` is a legitimate state.

## What "experimental" means here

Both schemas carry `"x-status": "experimental"` and sit at major `0`. While the binary
is `0.x`:

- **Fields may be added at any time, in a patch release.** Every extension point in
  both schemas leaves `additionalProperties` open, so a **pinned** copy of the schema
  keeps validating documents from a newer binary. Pin one if you want to; you do not
  have to re-fetch on every producer upgrade. Your own parser must ignore fields it does
  not recognise.
- **Fields will not be removed or renamed without bumping the major**, to `evidence/1`.
  Narrowing what an existing field means counts as a removal. A document's
  `schema_version` is the only thing to branch on.
- **Optional fields stay optional and keep their meaning.** In particular, an absent
  `approver_ne_author` means *not verified*, and will never come to mean `false`.
- **The control identifiers are an open set.** New controls add new identifiers within
  `evidence/0`, so `verification.verified` and `verification.recorded_not_verified` are
  typed as plain strings rather than an enum. Tolerate one you do not know — do not
  reject the document, and do not assume an unknown identifier means "passed".
- **Two sets stay closed**, because the algorithm fixes them rather than the schema: the
  tier scale (`T0`..`T3`, from `core.MaxTier`) and `result.exit_code` (`0` or `1`; a run
  that exits `2` emits no document). Changing either is a major bump by definition.

What the open schemas deliberately do **not** do is let the producer emit undocumented
fields. That is enforced on this side instead: a test asserts every key the tool emits is
described in the committed schema, so a DTO field added without a schema entry fails the
build. Openness is for the consumer's validator, not a licence for the producer.

At binary `v1.0` the schemas stabilise to `evidence/1` and `policy-binding/1`, and the
promise hardens to: additive changes only within a major, and a deprecation period
before any major bump. That step is deliberately not taken yet — the first real consumer
will find something about the shape worth fixing, and fixing it is cheaper than a
compatibility shim promised too early.

## Open findings at major 0

Major 0 exists to collect these. Recorded as found, from a caller that projects its
component map out of a real build graph. None of them is worked around by patching the
engine, and none is fixed yet.

### `shared` is a boolean where the graph has a count

A component with three dependents and one with three hundred both declare `shared: true`
and both escalate by exactly `TierIncrement` — one tier. A caller that has computed real
fan-in has nowhere to put it.

This is the ceiling CAF-SDLC-002 already states, met by the first consumer able to do
better. What is new is that the information now exists and the schema drops it.

Note the near miss: `cross_repo_consumers` is already a list on a component, so the map
*can* carry a set of dependents — but it is inert for tiering. It raises a warning and
never moves the tier. A graph-backed caller writing fan-in there gets a record that
mentions it and a tier that ignores it, which is worse than no field at all.

Candidate shapes, neither chosen: a `fan_in: <int>` field, or a `shared` that accepts a
threshold rather than a bool. The reason this is not a one-field change is that both
reopen the escalation model — today every rule adds exactly one tier and the result is
capped at `T3`. Proportional escalation from a count needs bands, and bands belong in the
tier policy rather than the map, so the fix spans both schemas. Worth doing deliberately,
at `evidence/1` and a versioned map schema, not as a patch.

### A generated map cannot be governed

Map governance assumes the map is a committed file: the CLI resolves `--config` and
`--policy` to repo-relative paths, and a change to either self-escalates to `T3`. For a
caller that *generates* the map from a build graph, the generated file may not be in the
diff at all — the generator is, and governance cannot see it.

Two precisions on where the gap actually is:

- **The engine already expresses this.** `core.EvaluateInput.GovernedPaths` is a plain
  `[]string` and will govern any path handed to it. What is missing is a way to fill it:
  the CLI only ever populates it from `--config` and `--policy`, and the map has no way
  to name the thing that produced it. A fix is a CLI flag and a map field, and touches
  `internal/core` not at all.
- **The workaround's real cost is the record, not the tier.** Declaring the generator as
  an ordinary `criticality: critical` component does escalate to `T3`, so the gate behaves
  correctly. But the evidence record then reads `generator criticality=critical -> base T3`
  instead of `control map changed (...) -> self-escalate to T3`, and `affected_set` carries
  an entry that is not a component. The decision is right and its stated reason is wrong,
  which is the one failure mode this tool is built to avoid.

### The component map has no published schema

`evidence/0` and `policy-binding/0` are versioned artifacts under `schemas/`. The component
map is not one: it is Go structs, `config/components.example.yaml`, and the prose in
CAF-SDLC-002. So a finding "against the map schema" is currently a finding against three
things that can disagree. Publishing the map as a versioned schema — and deciding whether
it shares the evidence major or versions separately — is a prerequisite for acting on the
two findings above.

## The honesty invariant, in machine-readable form

The tool's purpose is the distinction between a control it **checked** and a control it
**recorded as required and could not check**. Flattening that to a boolean would make
the document worse than useless, so it is carried three ways at once:

1. **Absent fields.** `approver_ne_author`, `approver_count` and `accountable_approver`
   are omitted entirely when the run had no approver set. Absent never means `false`.
2. **`verification.verified` / `verification.recorded_not_verified`.** Stable control
   identifiers, so a consumer can branch without parsing English.
3. **`warnings`.** The same facts in prose, for whoever reads the record directly.

The three are derived from the same engine state and cannot disagree.

### Control identifiers

These strings are part of the contract. The schema lists them as `examples` rather than
an enum, because the set grows as controls are added — see the stability rules above.

| Identifier | Verified when | Otherwise |
|---|---|---|
| `CAF-SDLC-002:tier` | always — the tier is computed by this run | — |
| `CAF-SDLC-010:provenance-complete` | the change declared AI assistance, so the `AI-Tool` trailer was checked for | absent |
| `CAF-SDLC-010:ai-authorship` | **never** | always recorded-not-verified: a trailer is a declaration, not a detection |
| `CAF-SDLC-011:approver-count` | an authoritative approver set was supplied | recorded-not-verified |
| `CAF-SDLC-011:independent-approver` | an authoritative approver set was supplied | recorded-not-verified |
| `owning-team-reviewer` | **never** | recorded-not-verified when the tier requires one — expanding a team handle needs a forge API |
| `required-checks` | **never** | recorded-not-verified when the tier names any — the CI jobs the policy names run them |
| `deploy-approval` | **never** | recorded-not-verified when the tier names one — this tool does not see deployments |

A control appears in exactly one of the two lists, never both, never neither.

## The serialization boundary

```text
internal/core       domain types, standard library only
   ↓ (one way)
internal/contract   explicit wire DTOs + mapping; asserts schema_version
   ↓
cmd/sdlc-controls   renders text or JSON, chooses an exit code
```

`internal/core` does not import `internal/contract`, and never will: the dependency
points one way so the engine stays free of wire concerns and the contract stays free to
version on its own. The DTOs are written out by hand rather than derived from core
structs — see the ADR for why that is the point rather than an oversight.

## Consuming a document

- Read `schema_version` first and branch on it.
- Treat unknown fields as additive; do not fail on them.
- `reasons` is **ordered**. The order is the order the algorithm applied the rules, and
  it is part of the meaning.
- `result.exit_code` is the process exit code that accompanied the document: `0` met,
  `1` not met. A run that exits `2` produced an error and no document at all.
- Do not infer the policy from the evidence. Fetch it with `sdlc-controls binding`,
  which projects the same map and policy the tiering used.
