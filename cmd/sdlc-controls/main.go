// Command sdlc-controls tiers a change from its git diff and a declared
// component map, then reports whether the change meets its tier's controls.
//
// The binary is the product: every CI adapter (the Docker action here, or a
// GitLab/Jenkins/Azure job elsewhere) is a thin wrapper around this same
// invocation. Nothing CI-specific lives below cmd/.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/codeaiforge/git-native-sdlc-controls/internal/core"
)

const usage = `sdlc-controls — git-native SDLC controls

Usage:
  sdlc-controls tier --base <ref> [--head <ref>] --config <path> [options]

Options:
  --base <ref>          base ref of the change (required)
  --head <ref>          head ref of the change (default HEAD)
  --config <path>       component map YAML (required)
  --policy <path>       tier policy YAML (default: built-in baseline policy)
  --format json|text    output format (default text)
  --evidence-out <path> also write the evidence record as JSON to this path
  --change-id <id>      identifier recorded in the evidence (default <base>..<head>)
  --author <handle>     change author, for the independent-approver control
  --approvers <a,b>     comma-separated approvers; omit if unavailable, in which
                        case approver controls are recorded but not verified
  --approvers-known     the approver set given is authoritative even if empty:
                        "nobody approved this" rather than "nobody asked". Set it
                        when the caller queried the forge and got an answer
  --repo <path>         repository root (default: current directory)

Exit codes: 0 controls met, 1 controls not met, 2 usage or runtime error.
`

func main() {
	os.Exit(run(os.Args[1:]))
}

// run is main's dispatch, taking args without the program name so it is
// callable from tests without spawning a subprocess to observe os.Exit.
func run(args []string) int {
	if len(args) < 1 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	switch args[0] {
	case "tier":
		return runTier(args[1:])
	case "-h", "--help", "help":
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n%s", args[0], usage)
		return 2
	}
}

func runTier(args []string) int {
	fs := flag.NewFlagSet("tier", flag.ContinueOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	var (
		base        = fs.String("base", "", "base ref")
		head        = fs.String("head", "HEAD", "head ref")
		configPath  = fs.String("config", "", "component map YAML")
		policyPath  = fs.String("policy", "", "tier policy YAML")
		format      = fs.String("format", "text", "output format: text or json")
		evidenceOut = fs.String("evidence-out", "", "write evidence JSON to this path")
		changeID    = fs.String("change-id", "", "change identifier")
		author      = fs.String("author", "", "change author")
		approvers   = fs.String("approvers", "", "comma-separated approvers")
		approversOK = fs.Bool("approvers-known", false, "the approver set is authoritative even if empty")
		repo        = fs.String("repo", ".", "repository root")
	)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *base == "" || *configPath == "" {
		fmt.Fprint(os.Stderr, "error: --base and --config are required\n\n"+usage)
		return 2
	}

	// A non-empty list is self-evidently a known approver set. --approvers-known
	// covers the case the list alone cannot express: the caller queried the forge
	// and the honest answer was nobody. A verified zero has to fail the tier's
	// min_approvers; an unknown set must not, so the two cannot share a spelling.
	approverList := splitList(*approvers)
	approversKnown := *approversOK || len(approverList) > 0

	cmap, err := loadComponentMap(*configPath)
	if err != nil {
		return fail(err)
	}
	policy, err := loadPolicy(*policyPath)
	if err != nil {
		return fail(err)
	}

	changedPaths, err := gitChangedPaths(*repo, *base, *head)
	if err != nil {
		return fail(err)
	}
	commitMessages, err := gitCommitMessages(*repo, *base, *head)
	if err != nil {
		return fail(err)
	}

	id := *changeID
	if id == "" {
		id = fmt.Sprintf("%s..%s", *base, *head)
	}

	// Map governance (CAF-SDLC-002). The control files are compared against the
	// diff, which names paths relative to the repository root, so they are put in
	// that form first — otherwise `--config /abs/path/components.yaml` never
	// matches `config/components.yaml` and the escalation silently never fires.
	//
	// An omitted --policy is not a gap: the built-in baseline policy is compiled
	// into the binary, so no file in the diff can change it.
	controlFiles := []string{*configPath}
	if *policyPath != "" {
		controlFiles = append(controlFiles, *policyPath)
	}
	var governed, ungovernable []string
	for _, p := range controlFiles {
		if rel, ok := repoRelative(*repo, p); ok {
			governed = append(governed, rel)
		} else {
			ungovernable = append(ungovernable, p)
		}
	}

	rec, err := core.Evaluate(core.EvaluateInput{
		ChangeID:          id,
		ChangedPaths:      changedPaths,
		CommitMessages:    commitMessages,
		Map:               cmap,
		Policy:            policy,
		GovernedPaths:     governed,
		UngovernablePaths: ungovernable,
		Author:            *author,
		Approvers:         approverList,
		ApproversKnown:    approversKnown,
		Now:               time.Now(),
	})
	if err != nil {
		return fail(err)
	}

	encoded, err := encodeEvidence(rec)
	if err != nil {
		return fail(err)
	}
	if *evidenceOut != "" {
		if err := os.WriteFile(*evidenceOut, encoded, 0o644); err != nil {
			return fail(err)
		}
	}

	violations := rec.Violations()
	if *format == "json" {
		fmt.Print(string(encoded))
	} else {
		printText(rec, violations)
	}

	if len(violations) > 0 {
		return 1
	}
	return 0
}

// encodeEvidence renders the record with HTML escaping off: the reasons contain
// "->", and an evidence file is read by people and auditors, not a browser.
func encodeEvidence(rec core.EvidenceRecord) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rec); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func printText(rec core.EvidenceRecord, violations []string) {
	fmt.Printf("change:  %s\n", rec.ChangeID)
	fmt.Printf("tier:    %s\n", rec.Tier)
	fmt.Printf("binding: %s\n", rec.BindingUsed)
	fmt.Printf("affected: %s\n", orNone(rec.AffectedSet))
	fmt.Printf("unmatched: %s\n", orNone(rec.UnmatchedSet))
	fmt.Println("reasons:")
	for _, r := range rec.Reasons {
		fmt.Printf("  - %s\n", r)
	}
	c := rec.ControlsEnforced
	fmt.Printf("required: min_approvers=%d checks=%s", c.MinApprovers, orNone(c.Checks))
	if c.IndependentApproverRequired {
		fmt.Print(" independent_approver=true")
	}
	fmt.Println()
	fmt.Printf("ai_assisted: %t", rec.AIAssisted)
	if rec.AITool != "" {
		fmt.Printf(" (tool=%s)", rec.AITool)
	}
	fmt.Println()
	for _, w := range rec.Warnings {
		fmt.Printf("warning: %s\n", w)
	}
	for _, v := range violations {
		fmt.Printf("FAIL: %s\n", v)
	}
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "(none)"
	}
	return strings.Join(s, ", ")
}

func fail(err error) int {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	return 2
}

func loadComponentMap(path string) (core.ComponentMap, error) {
	var m core.ComponentMap
	b, err := os.ReadFile(path)
	if err != nil {
		return m, fmt.Errorf("read component map: %w", err)
	}
	if err := yaml.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("parse component map %s: %w", path, err)
	}
	// A map the engine cannot reason about is an error, not a low tier.
	if err := core.ValidateMap(m); err != nil {
		return m, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// repoRelative expresses a control-file path the way `git diff` names it:
// relative to the repository root, with forward slashes. Governance compares the
// two literally, so a path that cannot be put in that form — a component map
// kept outside the repository under test — is reported as ungovernable rather
// than quietly never matching.
//
// ponytail: symlinks are not resolved. A repo reached through a symlinked path
// degrades to "ungovernable" — a warning in the record, never a silent pass.
func repoRelative(repo, path string) (string, bool) {
	repoAbs, err := filepath.Abs(repo)
	if err != nil {
		return "", false
	}
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(repoAbs, pathAbs)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
}

// defaultPolicy is the baseline escalation used when no --policy is given. It
// mirrors config/tier-policy.example.yaml so the tool is useful with one file.
var defaultPolicy = core.TierPolicy{
	"T0": {MinApprovers: 1, Checks: []string{"lint"}, DeployApproval: "standard"},
	"T1": {MinApprovers: 1, Checks: []string{"lint", "sast"}, DeployApproval: "standard"},
	"T2": {MinApprovers: 1, Checks: []string{"lint", "sast", "secrets", "deps"},
		DeployApproval: "owner", RequireOwningTeamReviewer: true},
	"T3": {MinApprovers: 2, Checks: []string{"lint", "sast", "secrets", "deps"},
		DeployApproval: "change_advisory", RequireOwningTeamReviewer: true,
		IndependentApproverRequired: true},
}

func loadPolicy(path string) (core.TierPolicy, error) {
	if path == "" {
		return defaultPolicy, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tier policy: %w", err)
	}
	var p core.TierPolicy
	if err := yaml.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parse tier policy %s: %w", path, err)
	}
	if err := core.ValidatePolicy(p); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

func gitChangedPaths(repo, base, head string) ([]string, error) {
	// base...head: the diff against the merge base, i.e. what this change adds
	// rather than what has landed on the base branch meanwhile.
	//
	// --no-renames: with rename detection on, git reports only the destination
	// of a moved file, so moving code out of a critical component would drop
	// that component from the affected set entirely. Both sides of a rename are
	// a change to both components.
	out, err := git(repo, "diff", "--name-only", "--no-renames", base+"..."+head)
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

func gitCommitMessages(repo, base, head string) ([]string, error) {
	// %x00 separates commits; message bodies contain newlines.
	out, err := git(repo, "log", "--format=%B%x00", base+".."+head)
	if err != nil {
		return nil, err
	}
	var msgs []string
	for _, m := range strings.Split(out, "\x00") {
		if strings.TrimSpace(m) != "" {
			msgs = append(msgs, m)
		}
	}
	return msgs, nil
}

func git(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out), nil
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
