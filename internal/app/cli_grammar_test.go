package app

import (
	"bytes"
	"strings"
	"testing"
)

// The M23.0 grammar normalization renamed or removed a set of command
// spellings with no deprecation aliases. These tests pin the grammar surface:
// new spellings parse, removed spellings are parser errors, and help output
// advertises only the new names.

func TestNewCommandSpellingsParse(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{
		{"doctor"},
		{"clarify", "add"},
		{"clarify", "run"},
		{"clarify", "show"},
		{"contract", "show"},
		{"contract", "accept"},
		{"execute", "plan"},
		{"execute", "show"},
		{"review", "plan"},
		{"review", "finding", "add"},
		{"review", "finding", "resolve"},
		{"review", "proposal", "collect"},
		{"review", "proposal", "accept"},
		{"review", "proposal", "reject"},
		{"review", "fix", "run"},
		{"review", "fix", "apply"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := testApp(root).Run(append(args, "--help"), &stdout, &stderr)
			if code != 0 {
				t.Fatalf("%v --help exited %d, stderr: %s", args, code, stderr.String())
			}
			if !strings.Contains(stdout.String(), "Usage:") {
				t.Fatalf("%v --help did not print usage:\n%s", args, stdout.String())
			}
		})
	}
}

func TestContractShowHasDraftFlag(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"contract", "show", "--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("contract show --help exited %d, stderr: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "--draft") {
		t.Fatalf("contract show --help does not advertise --draft:\n%s", stdout.String())
	}
}

func TestRemovedCommandSpellingsAreRejected(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{
		{"agents", "doctor"},
		{"clarify", "ask", "question?"},
		{"clarify", "loop"},
		{"clarify", "status"},
		{"clarify", "list"},
		{"clarify", "suggest"},
		{"contract", "show-draft"},
		{"contract", "accept-draft"},
		{"execute", "dry-run"},
		{"execute", "status"},
		{"review", "dry-run"},
		{"review", "add-finding", "message"},
		{"review", "resolve", "f_001"},
		{"review", "propose-findings"},
		{"review", "fix"},
		{"review", "apply-fix-outcomes"},
		{"review", "accept-proposal", "p_001"},
		{"review", "reject-proposal", "p_001"},
		{"review", "prepare"},
		{"review", "loop"},
		{"task", "current"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := testApp(root).Run(args, &stdout, &stderr)
			if code != 2 {
				t.Fatalf("%v exited %d, want parser error 2, stderr: %s", args, code, stderr.String())
			}
			if stderr.Len() == 0 {
				t.Fatalf("%v produced empty stderr", args)
			}
		})
	}
}

// Help output may echo a rejected token in a parser diagnostic, but the
// advertised command lists must only carry the new grammar.
func TestHelpAdvertisesOnlyNewCommandNames(t *testing.T) {
	root := t.TempDir()
	removed := []string{
		"agents doctor",
		"show-draft",
		"accept-draft",
		"dry-run",
		"add-finding",
		"propose-findings",
		"apply-fix-outcomes",
		"accept-proposal",
		"reject-proposal",
	}
	for _, helpArgs := range [][]string{
		{"--help"},
		{"clarify", "--help"},
		{"contract", "--help"},
		{"execute", "--help"},
		{"review", "--help"},
		{"review", "finding", "--help"},
		{"review", "fix", "--help"},
		{"review", "proposal", "--help"},
		{"task", "--help"},
	} {
		var stdout, stderr bytes.Buffer
		if code := testApp(root).Run(helpArgs, &stdout, &stderr); code != 0 {
			t.Fatalf("%v exited %d, stderr: %s", helpArgs, code, stderr.String())
		}
		got := stdout.String()
		for _, old := range removed {
			if strings.Contains(got, old) {
				t.Fatalf("%v advertises removed spelling %q:\n%s", helpArgs, old, got)
			}
		}
	}

	var stdout, stderr bytes.Buffer
	if code := testApp(root).Run([]string{"clarify", "--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("clarify --help exited %d, stderr: %s", code, stderr.String())
	}
	got := stdout.String()
	for _, old := range []string{"clarify ask", "clarify loop", "clarify status", "clarify list", "clarify suggest"} {
		if strings.Contains(got, old) {
			t.Fatalf("clarify --help advertises removed spelling %q:\n%s", old, got)
		}
	}
	for _, want := range []string{"add", "answer", "run", "show"} {
		if !strings.Contains(got, want) {
			t.Fatalf("clarify --help missing %q:\n%s", want, got)
		}
	}

	stdout.Reset()
	if code := testApp(root).Run([]string{"task", "--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("task --help exited %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "task current") {
		t.Fatalf("task --help still advertises a current command:\n%s", stdout.String())
	}
	stdout.Reset()
	if code := testApp(root).Run([]string{"execute", "--help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("execute --help exited %d, stderr: %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "execute status") {
		t.Fatalf("execute --help still advertises a status command:\n%s", stdout.String())
	}
}

// TestUsageErrorsAdvertiseNewGrammar pins every hand-written usage-error
// string to the new grammar: reverting any handler to an old spelling must
// fail a test, not just read stale.
func TestUsageErrorsAdvertiseNewGrammar(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		args  []string
		usage string
	}{
		{[]string{"clarify", "add"}, "usage: pactum clarify add [run_id] <question>"},
		{[]string{"clarify", "answer"}, "usage: pactum clarify answer [run_id] <question_id> <answer>"},
		{[]string{"execute", "show", "a", "b"}, "usage: pactum execute show [run_id] [attempt_id]"},
		{[]string{"review", "finding", "add"}, "usage: pactum review finding add [run_id] <message>"},
		{[]string{"review", "finding", "resolve"}, "usage: pactum review finding resolve [run_id] <finding_id>"},
		{[]string{"review", "proposal", "collect", "a", "b"}, "usage: pactum review proposal collect [run_id] [reviewer_attempt_id]"},
		{[]string{"review", "fix", "apply", "a", "b"}, "usage: pactum review fix apply [run_id] [fixer_attempt_id]"},
		{[]string{"review", "proposal", "accept"}, "usage: pactum review proposal accept [run_id] <proposal_id>"},
		{[]string{"review", "proposal", "reject"}, "usage: pactum review proposal reject [run_id] <proposal_id>"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := testApp(root).Run(tc.args, &stdout, &stderr); code == 0 {
				t.Fatalf("%v must fail with a usage error", tc.args)
			}
			if !strings.Contains(stderr.String(), tc.usage) {
				t.Fatalf("%v stderr = %q, want %q", tc.args, stderr.String(), tc.usage)
			}
		})
	}
}

// TestClarifyAnswerRecommendedFlagErrors pins the hand-written error paths of
// the --recommended/--all-recommended flag surface: the conflict, the
// arg-shape violations, and the question-id check.
func TestClarifyAnswerRecommendedFlagErrors(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"clarify", "answer", "q_001", "--recommended", "--all-recommended"}, "not both"},
		{[]string{"clarify", "answer", "q_001", "--all-recommended"}, "usage: pactum clarify answer [run_id] --all-recommended"},
		{[]string{"clarify", "answer", "--recommended"}, "usage: pactum clarify answer [run_id] <question_id> --recommended"},
		{[]string{"clarify", "answer", "blob", "--recommended"}, "expected a question id"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := testApp(root).Run(tc.args, &stdout, &stderr); code == 0 {
				t.Fatalf("%v must fail", tc.args)
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("%v stderr = %q, want %q", tc.args, stderr.String(), tc.want)
			}
		})
	}
}
