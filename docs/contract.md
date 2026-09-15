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

- **Fields may be added at any time, in a patch release.** A consumer must ignore
  fields it does not recognise. Every schema sets `additionalProperties: false`, so
  validating against a *pinned* copy of an older schema will reject a newer document —
  validate against the schema matching the `schema_version` you received.
- **Fields will not be removed or renamed without bumping the major**, to `evidence/1`.
  A document's `schema_version` is the only thing to branch on.
- **Optional fields stay optional and keep their meaning.** In particular, an absent
  `approver_ne_author` means *not verified*, and will never come to mean `false`.

At binary `v1.0` the schemas stabilise to `evidence/1` and `policy-binding/1`, and the
promise hardens to: additive changes only within a major, and a deprecation period
before any major bump. That step is deliberately not taken yet — the first real consumer
will find something about the shape worth fixing, and fixing it is cheaper than a
compatibility shim promised too early.

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

These strings are part of the contract. The enum is in the schema.

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
