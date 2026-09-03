# Evidence schema

One record per change, emitted by every run. It is the audit artifact: it states the tier, why that
tier, what the tier required, and what was verified. A record is meant to be readable years later by
someone who has neither the repository nor the CI system in front of them.

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

## Fields

| Field | Type | Notes |
|---|---|---|
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
| `warnings` | string[] | cross-repo consumers, stale-map hints, unverified controls |
| `map_version` | int | `version` of the component map used |
| `computed_at` | RFC 3339 (UTC) | when the decision was made |

## Reading a record

- `reasons` is the decision, not a log. Every escalation that fired appears, in the order the
  algorithm applied it, with the component or threshold that caused it.
- An absent `approver_ne_author` means the run had no approver set to check. That is a deliberate
  distinction from a verified failure: this file never records an unverified control as passed.
- `binding_used` and `map_version` together let a past decision be reproduced: the same engine binding
  against the same map version yields the same tier.
