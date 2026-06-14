package main

import (
	"slices"
	"testing"
)

// TestCodexArgs_BypassExcludesApprovalFlag pins the codex CLI rule that
// --dangerously-bypass-approvals-and-sandbox MUST NOT be combined with
// -a/--ask-for-approval (codex errors "cannot be used with '--ask-for-approval'").
// The bypass path is exactly what a NESTED agent uses, so getting it wrong breaks the
// most common in-sandbox invocation — as it did until this was fixed.
func TestCodexArgs_BypassExcludesApprovalFlag(t *testing.T) {
	bypass := codexArgs(config{mode: "review", base: "main", bypass: true}, "/tmp/x")
	if !slices.Contains(bypass, "--dangerously-bypass-approvals-and-sandbox") {
		t.Fatalf("bypass args missing the bypass flag: %v", bypass)
	}
	if slices.Contains(bypass, "-a") {
		t.Fatalf("bypass args include -a/--ask-for-approval, which codex rejects alongside --dangerously-bypass-…: %v", bypass)
	}
	if slices.Contains(bypass, "-s") {
		t.Fatalf("bypass args include -s sandbox flag, redundant with the bypass: %v", bypass)
	}

	// The sandboxed (default) path MUST keep -s read-only AND -a never.
	sandboxed := codexArgs(config{mode: "review", base: "main", bypass: false}, "/tmp/x")
	if !slices.Contains(sandboxed, "-a") || !slices.Contains(sandboxed, "never") {
		t.Fatalf("sandboxed args dropped -a never: %v", sandboxed)
	}
	if i := slices.Index(sandboxed, "-s"); i < 0 || sandboxed[i+1] != "read-only" {
		t.Fatalf("sandboxed args missing -s read-only: %v", sandboxed)
	}

	// exec mode carries --json + -o <lastMsg> and the prompt last.
	ex := codexArgs(config{mode: "exec", prompt: "do X", bypass: false}, "/tmp/last.txt")
	if !slices.Contains(ex, "--json") || !slices.Contains(ex, "-o") {
		t.Fatalf("exec args missing --json/-o: %v", ex)
	}
	if ex[len(ex)-1] != "do X" {
		t.Fatalf("exec prompt must be the final arg, got %q in %v", ex[len(ex)-1], ex)
	}
	if i := slices.Index(ex, "-o"); i < 0 || ex[i+1] != "/tmp/last.txt" {
		t.Fatalf("exec -o must point at lastMsgPath: %v", ex)
	}
	// Global flags must precede the subcommand (codex requires this ordering).
	if slices.Index(ex, "exec") < slices.Index(ex, "-m") {
		t.Fatalf("subcommand must come after global flags: %v", ex)
	}
}

func TestClassifyVerdict(t *testing.T) {
	cases := []struct {
		name, review, want string
	}{
		// Standard verdict lines.
		{"bare ACK", "Looks fine.\n\nACK", "ACK"},
		{"bare NAK", "Found a bug.\n\nNAK", "NAK"},
		{"bold NAK", "...\n\n**NAK.**", "NAK"},
		{"underscore-emphasis NAK", "...\n_NAK_", "NAK"},
		{"NAKED is not NAK", "the change is NAKED of tests", "UNKNOWN"},
		{"ACKNOWLEDGE is not ACK", "ACKNOWLEDGE the risk", "UNKNOWN"},
		{"LGTM line", "all good\nLGTM", "ACK"},
		{"ACK with tail", "ACK — implement it.", "ACK"},

		// NAK precedence: a blocking NAK must win over a co-occurring ACK.
		{"ack then nak lines", "ACK for the docs change.\n\nNAK on the code: race.", "NAK"},
		{"nak then ack lines", "NAK on the code.\nACK once you fix it.", "NAK"},

		// Mid-prose mentions must NOT be read as the verdict (the P0 false positives).
		{"mid-prose nak, no line", "(ack). Overall NAK because race.", "UNKNOWN"},
		{"not a clean ack phrase", "This isn't a clean ACK; needs work.", "UNKNOWN"},

		// Code spans are stripped: a verdict word inside code is not the verdict.
		{"ack inside fenced code", "```go\n// returns ACK\nfunc f(){}\n```\nNAK", "NAK"},
		{"ack inside inline code", "the `ACK` flag is set.\nNAK", "NAK"},
		{"only code ack", "```\nACK\n```", "UNKNOWN"},

		// No verdict at all.
		{"no verdict", "Some prose with no verdict word.", "UNKNOWN"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyVerdict(c.review)
			// UNKNOWN carries a trailing explanation; compare on the prefix.
			if c.want == "UNKNOWN" {
				if got[:7] != "UNKNOWN" {
					t.Fatalf("classifyVerdict(%q) = %q, want UNKNOWN…", c.review, got)
				}
				return
			}
			if got != c.want {
				t.Fatalf("classifyVerdict(%q) = %q, want %q", c.review, got, c.want)
			}
		})
	}
}

func TestCapBuffer(t *testing.T) {
	c := &capBuffer{max: 4}
	n, _ := c.Write([]byte("abcdef")) // over the cap
	if n != 6 {
		t.Fatalf("Write reported %d, want 6 (must report full consumption so the child isn't blocked)", n)
	}
	if c.String() != "abcd" {
		t.Fatalf("capBuffer kept %q, want %q", c.String(), "abcd")
	}
	c.Write([]byte("xyz")) // already at cap → dropped
	if c.String() != "abcd" {
		t.Fatalf("capBuffer grew past cap: %q", c.String())
	}
}
