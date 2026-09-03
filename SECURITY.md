# Security policy

## Reporting a vulnerability

Use GitHub's private vulnerability reporting: **Security → Advisories → Report a vulnerability** on
[the GitHub mirror](https://github.com/codeaiforge/git-native-sdlc-controls/security/advisories/new).
Please do not open a public issue for a vulnerability first.

Development happens on Gitea and GitHub is a push mirror, but the mirror is where reports go: Gitea
has no private-reporting or advisory mechanism, and a security report should not have to be filed in
public to reach someone.

This is a reference implementation maintained part-time. Expect an acknowledgement within a week and
an honest answer about whether and when it will be fixed — not a guaranteed patch window. If a report
is valid and you want credit in the advisory, say so.

## Supported versions

`main` and the most recent tag. There is no backport branch and no supported release train: a fix
lands on `main` and in the next tag, and older tags are not patched. Pin a tag if you run this in a
pipeline that matters, and watch the repository for releases.

## What counts as a vulnerability here

This tool decides how much scrutiny a change receives. The interesting failures are therefore the ones
that make a change look safer than it is:

- a change **under-tiered** by a bug in path matching, component resolution or the escalation
  algorithm — a path that should have matched a critical component and did not;
- a control **recorded as passed** that the run did not actually verify, or an evidence field that
  overstates what was checked;
- an evidence record that can be **forged or altered** through the tool's own inputs — a commit
  message, branch name or component map crafted to write a false record;
- the fail-safes not firing: an unmatched path that does not escalate, or a change to the component
  map or tier policy that does not self-escalate;
- ordinary code execution, path traversal or argument injection through `--config`, `--policy`,
  `--repo` or the refs passed to `git`.

Report those privately.

## What does not

- **The declared-topology ceiling.** A high fan-in component that nobody declared `shared: true` is
  under-tiered. That is a documented limitation, not a bug — see
  [CAF-SDLC-002](docs/controls/CAF-SDLC-002-change-risk-tiering.md) and the honest-ceiling section of
  the [README](README.md). A component map that has drifted from its repository is the same.
- **Controls the tool says it did not verify.** Owning-team review and the named checks (lint, sast,
  secrets, deps) are enforced by branch protection and by CI jobs, not here. The record says so on
  every run.
- **Your own CI configuration.** A gate whose result is not required by branch protection is not
  enforced, and this tool cannot make it so.

## This tool is not a security boundary

It reports a decision and an exit code. Anyone who can change the pipeline that calls it, or the
branch-protection rules that require it, can bypass it — as with any CI gate. Protect the workflow
file and the component map with the same rules you protect production code with; this repository's own
map declares `.github/**` and `config/**` as high-criticality for exactly that reason.
