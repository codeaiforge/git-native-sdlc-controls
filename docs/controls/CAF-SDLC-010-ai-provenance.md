# CAF-SDLC-010 — AI-generated change provenance

## Statement

Every change records whether it was AI-generated or AI-assisted, by which tool, and under which
session. The record lives with the change, in the forge, and survives independently of whatever IDE or
agent produced the code.

## Implementation

Git trailers on the commits that make up the change:

```
feat: add tier policy loader

AI-Assisted: true
AI-Tool: claude-code
AI-Session: 4f2c1a80-7d3e-4b19-9c55-0ae61b2d8f34
Prompt-Ref: #12
```

`AI-Assisted` and `AI-Tool` are the control's payload: whether a machine wrote it, and which one.
Those two carry the accountability question.

`AI-Session` is a convenience on top of them — the tool's own session identifier, recorded verbatim so
the conversation behind a change can be retrieved. It is optional, and it is only meaningful for the
clean case where one session produced the change. Where several did, there is no single session that
produced it, so the field is **absent** rather than holding whichever id came last. No list, no
sentinel, no enumeration: a change with many sessions simply has no session, and `ai_assisted` plus
`ai_tool` still say everything the control requires.

`Prompt-Ref` is the human-facing pointer — the issue or task the work came from. Keep it apart from
`AI-Session`: a hand-assigned label in `AI-Session` reads well in a log but resolves to nothing, and an
identifier nobody can resolve is decoration on an audit field.

`Prompt-Ref` points at something a team already keeps. It is not an instruction to commit prompts:
prompts carry the context around them, and in a public repository that is how client detail and
internal material arrive by accident. Reference the work item; leave the transcript where it lives.

PR labels are accepted as an equivalent signal where a team prefers them; CI normalises either into
the same evidence fields.

Parsing rules, as implemented in `internal/core/provenance.go`:

- trailer keys are case-insensitive;
- a change is AI-assisted if **any** of its commits says so;
- naming a tool or session implies assistance, so a missing `AI-Assisted:` trailer cannot understate
  provenance;
- where several commits declare a tool, the most recent wins;
- where several commits declare **different** sessions, none is recorded — see below.

CI validates presence: an AI-assisted change with no `AI-Tool` fails the gate, because provenance that
records only "an AI was involved" is not provenance.

## Evidence fields

| Field | Meaning |
|---|---|
| `ai_assisted` | whether any commit in the change declared AI assistance |
| `ai_tool` | the tool named on the change |
| `ai_session` | optional; the session identifier, present only when exactly one session is declared |
| `prompt_ref` | optional pointer to the issue or task that produced the change |

## Ceiling

The trailer is a **declaration**, not a detection. Nothing here proves a change was written by a
human, and this control does not attempt to infer it. It makes the honest case cheap to record and the
dishonest case an explicit, attributable act — which is the property an audit needs, and the most a
forge-level control can offer without a build-tool or IDE dependency.
