package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRepo builds a throwaway repository with two commits: base adds
// file1.txt, head adds file2.txt with a multi-line commit message. It returns
// the repo dir and both commit SHAs, so gitChangedPaths, gitCommitMessages and
// the full runTier path can all exercise a real git invocation rather than a
// mock of one.
func gitRepo(t *testing.T) (dir, base, head string) {
	t.Helper()
	dir = t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q", "-b", "main")
	run("-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "base commit")
	base = run("rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "file2.txt")
	run("-c", "commit.gpgsign=false", "commit", "-q", "-m", "head commit\n\nwith a body line")
	head = run("rev-parse", "HEAD")

	return dir, base, head
}

// componentMap writes a minimal map matching *.txt at low criticality, with
// breadth escalation and the unmatched-path floor both disabled so a *.txt-only
// change resolves cleanly to T0.
func componentMap(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "components.yaml")
	yaml := "version: 1\n" +
		"defaults:\n  unmatched_path_tier: low\n  breadth_threshold: 0\n" +
		"components:\n  - id: docs\n    match: [\"*.txt\"]\n    criticality: low\n    shared: false\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Map governance compares control-file paths against the diff literally, so a
// path this function gets wrong is an escalation that silently never fires.
func TestRepoRelative(t *testing.T) {
	repo := t.TempDir()

	tests := []struct {
		name string
		repo string
		path string
		want string
		ok   bool
	}{
		{
			name: "relative paths, as CI passes them",
			repo: ".",
			path: "config/components.yaml",
			want: "config/components.yaml",
			ok:   true,
		},
		{
			name: "an absolute config path resolves to the diff's form",
			repo: repo,
			path: filepath.Join(repo, "config", "components.yaml"),
			want: "config/components.yaml",
			ok:   true,
		},
		{
			name: "a file at the repository root",
			repo: repo,
			path: filepath.Join(repo, "components.yaml"),
			want: "components.yaml",
			ok:   true,
		},
		{
			name: "a map kept outside the repository is ungovernable",
			repo: repo,
			path: filepath.Join(filepath.Dir(repo), "other-repo", "components.yaml"),
			ok:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := repoRelative(tc.repo, tc.path)
			if ok != tc.ok {
				t.Fatalf("repoRelative(%q, %q) ok = %v, want %v", tc.repo, tc.path, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Errorf("repoRelative(%q, %q) = %q, want %q", tc.repo, tc.path, got, tc.want)
			}
		})
	}
}

func TestRun_Dispatch(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"no subcommand", []string{}, 2},
		{"unknown subcommand", []string{"bogus"}, 2},
		{"help", []string{"help"}, 0},
		{"-h", []string{"-h"}, 0},
		{"--help", []string{"--help"}, 0},
		{"tier with no flags", []string{"tier"}, 2}, // --base and --config are required
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := run(tc.args); got != tc.want {
				t.Errorf("run(%v) = %d, want %d", tc.args, got, tc.want)
			}
		})
	}
}

func TestRunTier_FlagParsing(t *testing.T) {
	dir, base, head := gitRepo(t)
	cfg := componentMap(t, dir)

	tests := []struct {
		name string
		args []string
		want int
	}{
		{"missing --base", []string{"--config", cfg, "--repo", dir}, 2},
		{"missing --config", []string{"--base", base, "--head", head, "--repo", dir}, 2},
		{"unknown flag", []string{"--base", base, "--config", cfg, "--nope", "x"}, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := runTier(tc.args); got != tc.want {
				t.Errorf("runTier(%v) = %d, want %d", tc.args, got, tc.want)
			}
		})
	}
}

// TestRunTier_ExitCodeContract is the control's whole value: 0 met, 1 not met,
// 2 usage/runtime error. Each case drives runTier through a real git repo
// rather than stubbing gitChangedPaths or gitCommitMessages.
func TestRunTier_ExitCodeContract(t *testing.T) {
	dir, base, head := gitRepo(t)
	cfg := componentMap(t, dir)

	tests := []struct {
		name string
		args []string
		want int
	}{
		{
			name: "controls met: known approver satisfies min_approvers",
			args: []string{"--base", base, "--head", head, "--config", cfg, "--repo", dir,
				"--approvers", "alice"},
			want: 0,
		},
		{
			name: "controls not met: approver set known and empty",
			args: []string{"--base", base, "--head", head, "--config", cfg, "--repo", dir,
				"--approvers-known"},
			want: 1,
		},
		{
			name: "runtime error: component map does not exist",
			args: []string{"--base", base, "--head", head, "--config", filepath.Join(dir, "missing.yaml"), "--repo", dir},
			want: 2,
		},
		{
			name: "runtime error: base ref does not exist",
			args: []string{"--base", "not-a-real-ref", "--head", head, "--config", cfg, "--repo", dir},
			want: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := runTier(tc.args); got != tc.want {
				t.Errorf("runTier(%v) = %d, want %d", tc.args, got, tc.want)
			}
		})
	}
}

func TestRunTier_EvidenceOut(t *testing.T) {
	dir, base, head := gitRepo(t)
	cfg := componentMap(t, dir)
	evidencePath := filepath.Join(t.TempDir(), "evidence.json")

	got := runTier([]string{
		"--base", base, "--head", head, "--config", cfg, "--repo", dir,
		"--approvers", "alice", "--change-id", "PR-1", "--evidence-out", evidencePath,
	})
	if got != 0 {
		t.Fatalf("runTier(...) = %d, want 0", got)
	}

	b, err := os.ReadFile(evidencePath)
	if err != nil {
		t.Fatalf("--evidence-out did not write %s: %v", evidencePath, err)
	}
	var rec map[string]any
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatalf("evidence file is not valid JSON: %v", err)
	}
	if rec["change_id"] != "PR-1" {
		t.Errorf("change_id = %v, want PR-1", rec["change_id"])
	}
	if rec["tier"] != "T0" {
		t.Errorf("tier = %v, want T0", rec["tier"])
	}
}

func TestGitChangedPaths(t *testing.T) {
	dir, base, head := gitRepo(t)
	paths, err := gitChangedPaths(dir, base, head)
	if err != nil {
		t.Fatalf("gitChangedPaths: %v", err)
	}
	if len(paths) != 1 || paths[0] != "file2.txt" {
		t.Errorf("gitChangedPaths = %v, want [file2.txt]", paths)
	}
}

func TestGitChangedPaths_InvalidRef(t *testing.T) {
	dir, _, head := gitRepo(t)
	if _, err := gitChangedPaths(dir, "not-a-real-ref", head); err == nil {
		t.Error("gitChangedPaths with an invalid base ref: want error, got nil")
	}
}

func TestGitCommitMessages(t *testing.T) {
	dir, base, head := gitRepo(t)
	msgs, err := gitCommitMessages(dir, base, head)
	if err != nil {
		t.Fatalf("gitCommitMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("gitCommitMessages = %v, want 1 message", msgs)
	}
	if !strings.Contains(msgs[0], "head commit") || !strings.Contains(msgs[0], "with a body line") {
		t.Errorf("gitCommitMessages[0] = %q, want subject and body both present", msgs[0])
	}
}

func TestSplitList(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"alice", []string{"alice"}},
		{"alice, bob , carol", []string{"alice", "bob", "carol"}},
		{" , , ", nil},
	}
	for _, tc := range tests {
		got := splitList(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("splitList(%q) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("splitList(%q) = %v, want %v", tc.in, got, tc.want)
				break
			}
		}
	}
}

// A typo in a control file has to fail the run: a map or policy the engine
// cannot reason about must never read as "no controls required".
func TestLoadConfigRejectsBadInput(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	t.Run("component map", func(t *testing.T) {
		tests := []struct {
			name    string
			path    string
			wantErr bool
		}{
			{"missing file", filepath.Join(dir, "nope.yaml"), true},
			{"not yaml", write("bad.yaml", "\tnot: [valid"), true},
			{"criticality typo", write("typo.yaml",
				"version: 1\ncomponents:\n  - id: a\n    match: [\"*.txt\"]\n    criticality: critcal\n"), true},
			{"no components", write("empty.yaml", "version: 1\ncomponents: []\n"), true},
			{"valid", componentMap(t, dir), false},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := loadComponentMap(tc.path)
				if (err != nil) != tc.wantErr {
					t.Errorf("loadComponentMap(%s) err = %v, wantErr %v", tc.name, err, tc.wantErr)
				}
			})
		}
	})

	t.Run("tier policy", func(t *testing.T) {
		full := "T0: {min_approvers: 1, checks: [lint]}\nT1: {min_approvers: 1, checks: [lint]}\n" +
			"T2: {min_approvers: 1, checks: [lint]}\nT3: {min_approvers: 2, checks: [lint], independent_approver_required: true}\n"
		tests := []struct {
			name    string
			path    string
			wantErr bool
		}{
			{"empty path falls back to the built-in baseline", "", false},
			{"missing file", filepath.Join(dir, "nope-policy.yaml"), true},
			{"not yaml", write("bad-policy.yaml", "\tnot: [valid"), true},
			{"missing a tier", write("partial.yaml", "T0: {min_approvers: 1, checks: [lint]}\n"), true},
			{"complete", write("policy.yaml", full), false},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				_, err := loadPolicy(tc.path)
				if (err != nil) != tc.wantErr {
					t.Errorf("loadPolicy(%s) err = %v, wantErr %v", tc.name, err, tc.wantErr)
				}
			})
		}
	})
}

// --format json and --policy on the same run: the custom policy demands two
// approvers at T0, so one approver has to exit 1.
func TestRunTier_JSONFormatWithCustomPolicy(t *testing.T) {
	dir, base, head := gitRepo(t)
	cfg := componentMap(t, dir)
	policy := filepath.Join(dir, "policy.yaml")
	body := "T0: {min_approvers: 2, checks: [lint]}\nT1: {min_approvers: 2, checks: [lint]}\n" +
		"T2: {min_approvers: 2, checks: [lint]}\nT3: {min_approvers: 2, checks: [lint]}\n"
	if err := os.WriteFile(policy, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got := runTier([]string{"--base", base, "--head", head, "--config", cfg, "--repo", dir,
		"--policy", policy, "--format", "json", "--approvers", "alice"})
	if got != 1 {
		t.Errorf("runTier with min_approvers=2 and one approver = %d, want 1", got)
	}
}

// captureStdout runs f with stdout redirected and returns what it printed. The
// output formats are a published interface; a test that cannot read them can
// only assert exit codes.
func captureStdout(t *testing.T, f func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stdout
	os.Stdout = w

	code := f()

	os.Stdout = saved
	w.Close()
	var buf strings.Builder
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	r.Close()
	return buf.String(), code
}

// The text output is the v0.1.0 interface. 0.2.0 is additive: it added JSON
// fields and a subcommand, and must not have moved a byte of this.
func TestRunTier_TextGolden(t *testing.T) {
	dir, base, head := gitRepo(t)
	cfg := componentMap(t, dir)

	got, code := captureStdout(t, func() int {
		return runTier([]string{"--base", base, "--head", head, "--config", cfg, "--repo", dir,
			"--change-id", "PR-1", "--author", "alice", "--approvers", "bob"})
	})
	if code != 0 {
		t.Fatalf("runTier = %d, want 0", code)
	}

	want, err := os.ReadFile(filepath.Join("testdata", "tier-t0.txt"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Errorf("text output changed — it is the v0.1.0 interface:\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}

// Text and JSON are two renderings of one decision, not two decisions. If they
// can disagree on the tier, the reasons, the required controls or the exit
// code, one of them is lying to whoever reads it.
func TestRunTier_TextAndJSONAgree(t *testing.T) {
	dir, base, head := gitRepo(t)
	cfg := componentMap(t, dir)
	args := []string{"--base", base, "--head", head, "--config", cfg, "--repo", dir,
		"--change-id", "PR-1", "--author", "alice", "--approvers", "bob"}

	text, textCode := captureStdout(t, func() int { return runTier(args) })
	raw, jsonCode := captureStdout(t, func() int { return runTier(append(args, "--format", "json")) })

	if textCode != jsonCode {
		t.Errorf("exit codes differ by format: text %d, json %d", textCode, jsonCode)
	}

	var doc struct {
		Tier             string   `json:"tier"`
		Reasons          []string `json:"reasons"`
		ControlsEnforced struct {
			MinApprovers int      `json:"min_approvers"`
			Checks       []string `json:"checks"`
		} `json:"controls_enforced"`
		Result struct {
			Pass     bool `json:"pass"`
			ExitCode int  `json:"exit_code"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("--format json did not emit valid JSON: %v\n%s", err, raw)
	}

	if doc.Result.ExitCode != jsonCode {
		t.Errorf("result.exit_code = %d, but the process exited %d", doc.Result.ExitCode, jsonCode)
	}
	if doc.Result.Pass != (jsonCode == 0) {
		t.Errorf("result.pass = %t with exit %d", doc.Result.Pass, jsonCode)
	}
	if !strings.Contains(text, "tier:    "+doc.Tier) {
		t.Errorf("tier %q is not the one the text output states:\n%s", doc.Tier, text)
	}
	for _, r := range doc.Reasons {
		if !strings.Contains(text, r) {
			t.Errorf("reason %q is in the JSON but not in the text output", r)
		}
	}
	if !strings.Contains(text, fmt.Sprintf("min_approvers=%d", doc.ControlsEnforced.MinApprovers)) {
		t.Errorf("min_approvers disagrees between the formats:\n%s", text)
	}
	for _, c := range doc.ControlsEnforced.Checks {
		if !strings.Contains(text, c) {
			t.Errorf("check %q is in the JSON but not in the text output", c)
		}
	}
}

// A failing gate still has to emit its record: a consumer that only gets JSON
// on success learns nothing about the changes that were actually blocked.
func TestRunTier_JSONEmittedOnFailingGate(t *testing.T) {
	dir, base, head := gitRepo(t)
	cfg := componentMap(t, dir)

	raw, code := captureStdout(t, func() int {
		return runTier([]string{"--base", base, "--head", head, "--config", cfg, "--repo", dir,
			"--format", "json", "--approvers-known"}) // queried the forge, nobody approved
	})
	if code != 1 {
		t.Fatalf("runTier = %d, want 1 on a verified-empty approver set", code)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("no JSON on a failing gate: %v\n%s", err, raw)
	}
	result, ok := doc["result"].(map[string]any)
	if !ok {
		t.Fatalf("record has no result block: %s", raw)
	}
	if result["pass"] != false || result["exit_code"].(float64) != 1 {
		t.Errorf("result = %v, want pass false and exit_code 1", result)
	}
	if len(result["violations"].([]any)) == 0 {
		t.Error("a blocked change must name what it failed")
	}
}

// The binding subcommand publishes the policy a tier decision was made under.
// It gates nothing, so it never exits 1.
func TestRunBinding(t *testing.T) {
	dir, _, _ := gitRepo(t)
	cfg := componentMap(t, dir)

	t.Run("flag parsing", func(t *testing.T) {
		tests := []struct {
			name string
			args []string
			want int
		}{
			{"missing --config", []string{}, 2},
			{"unknown flag", []string{"--config", cfg, "--nope"}, 2},
			{"component map does not exist", []string{"--config", filepath.Join(dir, "missing.yaml")}, 2},
			{"text", []string{"--config", cfg}, 0},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if _, code := captureStdout(t, func() int { return runBinding(tc.args) }); code != tc.want {
					t.Errorf("runBinding(%v) = %d, want %d", tc.args, code, tc.want)
				}
			})
		}
	})

	t.Run("json states the tier table the engine tiers against", func(t *testing.T) {
		raw, code := captureStdout(t, func() int {
			return runBinding([]string{"--config", cfg, "--format", "json"})
		})
		if code != 0 {
			t.Fatalf("runBinding = %d, want 0", code)
		}
		var doc struct {
			SchemaVersion string `json:"schema_version"`
			Binding       string `json:"binding"`
			Tiers         map[string]struct {
				MinApprovers                int  `json:"min_approvers"`
				IndependentApproverRequired bool `json:"independent_approver_required"`
			} `json:"tiers"`
		}
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatalf("binding --format json is not valid JSON: %v\n%s", err, raw)
		}
		if doc.SchemaVersion != "policy-binding/0" {
			t.Errorf("schema_version = %q, want policy-binding/0", doc.SchemaVersion)
		}
		if doc.Binding != "git-native-baseline@1" {
			t.Errorf("binding = %q", doc.Binding)
		}
		// The default policy is compiled in; the published table has to be it.
		for tier, want := range defaultPolicy {
			got := doc.Tiers[tier]
			if got.MinApprovers != want.MinApprovers {
				t.Errorf("%s min_approvers = %d, want %d", tier, got.MinApprovers, want.MinApprovers)
			}
			if got.IndependentApproverRequired != want.IndependentApproverRequired {
				t.Errorf("%s independent_approver_required = %t, want %t",
					tier, got.IndependentApproverRequired, want.IndependentApproverRequired)
			}
		}
	})
}
