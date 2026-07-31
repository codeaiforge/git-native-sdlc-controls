# CAF-SDLC-011 — Independent approver for AI-generated changes

## Statement

An AI-generated change cannot be approved by the human who prompted it. A named approver, distinct
from the change's author and committer, is required before merge.

The accountability question this answers is not "did a machine write it" but "which human is
accountable for it having merged".

## Implementation

Two halves, deliberately:

1. **Provenance** (CAF-SDLC-010) marks the change as AI-assisted.
2. **The forge** enforces the approval: a branch-protection rule requiring an approving review from
   someone other than the author, applied to AI-flagged changes and to any change the tier policy
   marks `independent_approver_required`.

This tool computes the requirement and records the outcome; it does not replace branch protection.
Where an approver set is available to the run, the engine verifies it and fails the gate on
self-approval. Where it is not, the evidence record says so explicitly rather than passing silently —
an unverified control is recorded as unverified.

Those are three states, not two, and the run has to distinguish them:

| The run knows | How it is expressed | Result |
|---|---|---|
| nothing about approvals | `--approvers` omitted | recorded unverified; the gate does not fail on it |
| that nobody approved | `--approvers-known` with an empty `--approvers` | `approver_count: 0`; **fails** `min_approvers` |
| who approved | `--approvers a,b` | verified against `min_approvers` and against the author |

The middle row is the one that matters. A CI job that queries the forge and gets no approvals has
learned something, and a gate that treats that answer the same as never having asked is not a gate.
The caller asserts which case it is, because only the caller knows whether it looked.

## Evidence fields

| Field | Meaning |
|---|---|
| `accountable_approver` | the approver who satisfies segregation of duties |
| `approver_ne_author` | whether that approver is distinct from the author; absent when unverified |
| `approver_count` | number of approvers seen, checked against the tier's `min_approvers` |

## Basis

Generic segregation of duties: the party that produces a change is not the party that accepts it. The
control is stated in those terms and carries no regulation-specific citation.
