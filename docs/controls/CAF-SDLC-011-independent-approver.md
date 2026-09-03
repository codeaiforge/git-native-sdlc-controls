# CAF-SDLC-011 — Independent approver for AI-generated changes

## Statement

A change carrying a tier the policy marks `independent_approver_required` must be approved by an
account other than the one recorded as its author, and the evidence must name that approver.

The accountability question this answers is not "did a machine write it" but "which human is
accountable for it having merged".

Read that statement literally, because the gap between it and the intent behind it is where this
control can be oversold. The intent is that AI-generated work is not waved through by the person who
prompted it. What the engine can check is narrower, and the difference is set out below.

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

## What this does not check

Four limits, none of them incidental:

- **The rule is attached to the tier, not to AI provenance.** `Violations()` fires on
  `independent_approver_required`, which the baseline policy sets on T3 alone. So *every* T3 change
  needs an independent approver, AI-assisted or not — and an AI-assisted change at T0, T1 or T2 needs
  none from this engine. If AI provenance should demand independence at every tier, that is a policy
  you write (set the flag on lower tiers) or an escalation rule this engine does not yet have. It is
  not something the current baseline does.
- **"Author" is a handle the caller supplies, not the person who prompted.** The engine compares
  approvers against whatever `--author` was passed — in practice the forge account that raised the
  change. Whoever ran the model is not an input, is not knowable from a diff, and is not checked.
- **Identity is string comparison.** Handles are matched case-insensitively after trimming. One human
  with two accounts satisfies the check; so does a service account approving on someone's behalf.
  Tying accounts to people is the forge's job.
- **The committer is not checked at all.** There is no committer input to the engine. A rule about
  author *and* committer would need one.

None of these make the control useless: at the tier where blast radius is highest, a change cannot
merge on its author's own approval alone, and the record names who accepted it. They do mean the
control should not be described as "an AI change cannot be approved by its prompter" — it is
segregation of duties between two forge accounts, at a tier the policy chooses.

## Evidence fields

| Field | Meaning |
|---|---|
| `accountable_approver` | the approver who satisfies segregation of duties |
| `approver_ne_author` | whether that approver is distinct from the author; absent when unverified |
| `approver_count` | number of approvers seen, checked against the tier's `min_approvers` |

## Basis

Generic segregation of duties: the party that produces a change is not the party that accepts it. The
control is stated in those terms and carries no regulation-specific citation.
