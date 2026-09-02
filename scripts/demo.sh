#!/usr/bin/env bash
# make demo — the whole tool, offline, in one command.
#
# Builds a throwaway repository shaped like FINOS TraderX, tiers four changes
# against examples/traderx/components.yaml, and refreshes the evidence records
# committed under examples/traderx/evidence/. Needs git and the built binary;
# no network, no clone, no build tool for the languages in the demo repo — that
# last one being the entire point.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
bin=$root/bin/sdlc-controls
out=$root/examples/traderx/evidence

demo=$(mktemp -d)
trap 'rm -rf "$demo"' EXIT

# A demo repository with no committer identity configured is a demo that fails
# on a clean CI runner.
export GIT_AUTHOR_NAME=alice GIT_AUTHOR_EMAIL=alice@example.com
export GIT_COMMITTER_NAME=alice GIT_COMMITTER_EMAIL=alice@example.com

git -C "$demo" init -q -b main
mkdir -p "$demo"/web-client/src/app "$demo"/reference-data/src/main/java "$demo"/evidence

# The component map lives inside the repository under test, so a change to the
# map is visible to the diff and self-escalates like any other change.
cp "$root/examples/traderx/components.yaml" "$demo/components.yaml"
echo 'export const columns = ["symbol", "qty"];' > "$demo/web-client/src/app/trades.component.ts"
echo 'class SecurityRepository { String isin(String s) { return s; } }' \
  > "$demo/reference-data/src/main/java/SecurityRepository.java"
git -C "$demo" add -A
git -C "$demo" commit -qm 'initial import'

branch() { git -C "$demo" checkout -q -b "$1" main; }

branch ui-tweak
echo 'export const columns = ["symbol", "qty", "px"];' > "$demo/web-client/src/app/trades.component.ts"
git -C "$demo" commit -qam 'feat(web-client): show price column'

branch refdata-fix
sed -i 's/return s;/return s.trim();/' "$demo/reference-data/src/main/java/SecurityRepository.java"
git -C "$demo" commit -qam 'fix(reference-data): trim ISIN lookups'

branch refdata-ai
sed -i 's/return s;/return s.trim().toUpperCase();/' "$demo/reference-data/src/main/java/SecurityRepository.java"
git -C "$demo" commit -qam 'fix(reference-data): normalise ISIN lookups

AI-Assisted: true
AI-Tool: claude-code
AI-Session: demo-4f21
Prompt-Ref: TRADERX-412'

git -C "$demo" checkout -q main

# run <headline> <branch> <evidence-name> [extra flags...]
run() {
  local headline=$1 branch=$2 name=$3
  shift 3
  local args=(tier --base main --head "$branch" --config components.yaml
              --change-id "DEMO-$name" --evidence-out "evidence/$name.json" "$@")
  local code=0
  echo
  echo "─── $headline"
  echo
  echo "\$ sdlc-controls ${args[*]}"
  (cd "$demo" && "$bin" "${args[@]}") || code=$?
  case $code in
    0) echo "exit: 0 — controls met" ;;
    1) echo "exit: 1 — controls not met, the gate blocks the merge" ;;
    *) echo "exit: $code — error"; exit "$code" ;;
  esac
  cp "$demo/evidence/$name.json" "$out/$name.json"
}

run "A UI tweak: one line in web-client, reviewed by one person" \
    ui-tweak t0-web-client --author alice --approvers bob

run "The same size diff in a shared service: one approver is no longer enough" \
    refdata-fix t3-reference-data --author alice --approvers bob

run "An AI-assisted change, self-approved (CAF-SDLC-010, CAF-SDLC-011)" \
    refdata-ai t3-ai-self-approved --author alice --approvers alice

run "The same AI-assisted change with an independent approver" \
    refdata-ai t3-ai-independent --author alice --approvers alice,carol

echo
echo "─── Evidence"
echo
echo "Four records refreshed in examples/traderx/evidence/ — the committed copies"
echo "differ from these only in computed_at, which is the wall clock of this run:"
ls -1 "$out"
