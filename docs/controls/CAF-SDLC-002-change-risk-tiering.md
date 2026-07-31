# CAF-SDLC-002 — Change risk tiering by blast radius

## Statement

Every change is assigned a risk tier derived from its declared blast radius. Higher tiers escalate the
required review, approvers and checks. The tier is computed from the change itself, not from the
judgement of whoever opened the pull request.

## Inputs

| Input | Source |
|---|---|
| changed paths | `git diff --name-only --no-renames <base>...<head>` |
| declared topology | `components.yaml` |
| escalation table | `tier-policy.yaml` |

`--no-renames` is deliberate. With rename detection on, git reports only the destination of a moved
file, so moving code *out* of a critical component would drop that component from the affected set
and under-tier the change. A rename is a change to both components and is tiered as one.

No build tool, package manager or language toolchain is consulted. Any repository that has a git
history and a component map can be tiered.

## Component map schema

```yaml
version: 1
defaults:
  unmatched_path_tier: high   # fail-safe class for paths nobody declared
  breadth_threshold: 4        # affected components at which breadth escalates
components:
  - id: payment-authorization       # stable identifier, appears in evidence
    match: ["services/payment-auth/**", "libs/payment-core/**"]
    criticality: critical           # low | medium | high | critical
    shared: false                   # high fan-in proxy: many callers depend on it
    owners: ["@org/payments-core"]
    cross_repo_consumers: []        # optional; consumers outside this repository
```

`match` patterns support `*` (within a path segment), `**` (across segments) and `?`. A path may match
several components; every match counts.

The map is validated when it is loaded, and an invalid map **fails the run** (exit 2) rather than
being tiered around. Rejected: a `criticality` or `unmatched_path_tier` outside
`low|medium|high|critical`, a component with no `id`, a component with no `match` patterns, and a
duplicate `id`. This matters more than it looks — `criticality: critcal` is a plausible typo, and a
tolerant parser would have tiered a critical component at T0 and passed the change.

## Resolution algorithm (deterministic)

1. Collect the changed paths for the change.
2. Match each path against every component. Matches accumulate into `affected_set`; a path that
   matches nothing goes to `unmatched_set`.
3. `base_tier = max(criticality)` over `affected_set`, on the scale `low=0, medium=1, high=2,
   critical=3` → `T0..T3`.
4. Escalate, capping at `T3`:
   - any component with `shared: true` → `+1`
   - `len(affected_set) >= breadth_threshold` → `+1`
   - `unmatched_set` non-empty → `tier = max(tier, unmatched_path_tier)` (fail-safe)
5. Emit the tier, an ordered `reasons[]`, both sets, and any cross-repo warnings.

The same inputs always produce the same tier *and* the same reason list, in the same order. Sets are
sorted; path order in the diff does not affect the outcome.

## Tiers → escalation

Taken verbatim from `tier-policy.yaml`; see `config/tier-policy.example.yaml` for the baseline:

| Tier | Approvers | Checks | Deploy approval | Notes |
|---|---|---|---|---|
| T0 | 1 | lint | standard | |
| T1 | 1 | lint, sast | standard | |
| T2 | 1 | lint, sast, secrets, deps | owner | owning-team reviewer required |
| T3 | 2 | lint, sast, secrets, deps | change advisory | independent approver required |

The engine records which controls a tier demands. Enforcement is split: approver rules are enforced by
the forge's branch protection, checks by the CI jobs themselves, and the tier decision plus the
evidence record by this tool.

Two of those controls are therefore recorded but not verified here, and the record says so rather than
implying they passed:

- `require_owning_team_reviewer` — expanding a team handle into its members needs a forge API, which
  the engine will not take a dependency on. The evidence carries a warning naming the `owners` of the
  affected components, for CODEOWNERS and branch protection to enforce.
- approver count and independence — verified when the run is given an approver set (`--approvers`) or
  told that an empty one is authoritative (`--approvers-known`). With neither, `approver_ne_author`
  and `approver_count` are absent rather than `false` and `0`. See CAF-SDLC-011.

## The honest ceiling

This control approximates blast radius from **declared topology** — a human-maintained `criticality`
and `shared` flag — not from a computed dependency graph. The consequences are specific:

- a high fan-in component that nobody declared `shared: true` will be **under-tiered**;
- a component whose real callers changed will not re-tier until someone edits the map;
- `shared` is a boolean stand-in for what is really a fan-in count.

This is a deliberate trade: a computed graph is more accurate but needs per-language build-tool
integration, which would end the tooling-agnostic property that makes this approach portable. The
fail-safe floor for unmatched paths and the map-governance rule below are what keep the approximation
honest; they do not make it a graph.

## Map governance

- Changes to the component map or the tier policy **self-escalate to the top tier**. The file that
  decides how much scrutiny a change gets is itself the highest-scrutiny file in the repository.
- The comparison is literal, so control-file paths are first put in the form the diff uses — relative
  to the repository root. `--config /abs/path/config/components.yaml` and `--config
  config/components.yaml` self-escalate identically.
- A control file that cannot be expressed that way — a map kept **outside** the repository under test,
  as when one map tiers several repos — is reported in the evidence as ungovernable. Governance
  genuinely cannot apply to it, and an absent escalation must not read as "the map was untouched".
- With no `--policy`, the built-in baseline policy is in force. It is compiled into the binary, so no
  file in the diff can change it and there is nothing to govern.
- Any unmatched path is reported as a warning that the map has drifted from the repository.
