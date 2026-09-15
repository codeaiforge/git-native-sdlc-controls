# Evidence schema

One record per change, emitted by every run. It is the audit artifact: it states the tier, why that
tier, what the tier required, and what was verified. A record is meant to be readable years later by
someone who has neither the repository nor the CI system in front of them.

The machine-readable form is schema **`evidence/0`**, published as
[`schemas/evidence/0/evidence.schema.json`](../schemas/evidence/0/evidence.schema.json) and validated
in CI against the JSON the tool actually emits. The versioning policy — and what "experimental"
commits to — is in [contract.md](contract.md).

```json
{
  "schema_version": "evidence/0",
  "tool": { "name": "sdlc-controls", "version": "0.2.0" },
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
  "verification": {
    "approvers_supplied": false,
    "verified": ["CAF-SDLC-002:tier"],
    "recorded_not_verified": ["CAF-SDLC-010:ai-authorship", "CAF-SDLC-011:approver-count",
                              "CAF-SDLC-011:independent-approver", "owning-team-reviewer",
                              "required-checks"]
  },
  "result": { "pass": true, "exit_code": 0, "violations": [] },
  "warnings": ["approver set not supplied: approver controls recorded but not verified by this run"],
  "map_version": 1,
  "computed_at": "2026-07-30T09:00:00Z"
}
```

## Fields

| Field | Type | Notes |
|---|---|---|
| `schema_version` | `"evidence/0"` | the shape of this document; versioned independently of the binary and of the binding |
| `tool` | object | `name` and `version` of the binary that produced the record |
| `change_id` | string | PR/MR identifier, or `<base>..<head>` when none is supplied |
| `binding_used` | string | control binding and version this decision was made under |
| `tier` | `"T0".."T3"` | the computed risk tier |
| `affected_set` | string[] | component IDs the change touched, sorted |
| `unmatched_set` | string[] | changed paths that matched no component, sorted |
| `reasons` | string[] | ordered decision trail; one entry per rule that fired |
| `controls_enforced` | object | the tier's entry from the tier policy |
| `ai_assisted` | bool | CAF-SDLC-010 |
| `ai_tool` | string | CAF-SDLC-010; the tool named on the change |
| `ai_session` | string | CAF-SDLC-010; optional, and **absent when the change spans more than one session** — there is no single session to name |
| `prompt_ref` | string | CAF-SDLC-010; optional pointer to the issue or task |
| `accountable_approver` | string | CAF-SDLC-011; omitted when no approver set was available |
| `approver_ne_author` | bool | CAF-SDLC-011; **absent means not verified**, not "false" |
| `approver_count` | int | **distinct** approvers seen (folded case-insensitively), checked against `min_approvers`; **absent means not verified**, and `0` means verified nobody |
| `verification` | object | `approvers_supplied`, plus `verified` and `recorded_not_verified` control identifiers — the structured form of what this run checked; see [contract.md](contract.md#control-identifiers) |
| `result` | object | `pass`, `exit_code` (0 or 1) and `violations` — the gate's verdict, so a consumer need not observe a process exit code |
| `warnings` | string[] | cross-repo consumers, stale-map hints, unverified controls — the prose form of `verification` |
| `map_version` | int | `version` of the component map used |
| `computed_at` | RFC 3339 (UTC) | when the decision was made |

## Reading a record

- `reasons` is the decision, not a log. Every escalation that fired appears, in the order the
  algorithm applied it, with the component or threshold that caused it.
- An absent `approver_ne_author` means the run had no approver set to check. That is a deliberate
  distinction from a verified failure: this file never records an unverified control as passed. The
  same fact is stated positively in `verification.recorded_not_verified`, so nothing has to be
  inferred from a missing field.
- `verification` and `warnings` are two renderings of one state and cannot disagree: every control
  appears in exactly one of `verified` and `recorded_not_verified`, never both and never neither.
- `binding_used` and `map_version` together let a past decision be reproduced: the same engine binding
  against the same map version yields the same tier.
