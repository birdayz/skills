package main

import (
	"strings"
	"testing"
)

// feed writes s to the tracker in `chunk`-byte slices so the per-line state machine is
// exercised across Write boundaries (real codex output arrives in arbitrary chunks, often
// splitting a line — a parser that assumed whole-line writes would silently miscount).
func feed(t *liveTracker, s string, chunk int) {
	b := []byte(s)
	for i := 0; i < len(b); i += chunk {
		j := i + chunk
		if j > len(b) {
			j = len(b)
		}
		t.Write(b[i:j])
	}
}

func TestParseJSON_ExecEvents(t *testing.T) {
	// The exact event shapes `codex exec --json` emits (verified against codex v0.139.0):
	// a turn with one tool call, then a final agent message, then turn.completed w/ usage.
	stream := strings.Join([]string{
		`{"type":"thread.started","thread_id":"x"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.started","item":{"id":"i0","type":"command_execution","command":"git diff master","status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"i0","type":"command_execution","command":"git diff master","status":"completed"}}`,
		`{"type":"item.completed","item":{"id":"i1","type":"agent_message","text":"Looks good.\nACK"}}`,
		`{"type":"turn.completed","usage":{"input_tokens":11969,"cached_input_tokens":4992,"output_tokens":383,"reasoning_output_tokens":14}}`,
	}, "\n") + "\n"

	for _, chunk := range []int{1, 7, 4096} { // byte-at-a-time, mid-line, whole-blob
		tr := &liveTracker{cap: &capBuffer{max: 1 << 20}, jsonMode: true}
		feed(tr, stream, chunk)
		turns, tools, tokens, _, last := tr.progress()
		if turns != 1 {
			t.Errorf("chunk=%d turns=%d, want 1", chunk, turns)
		}
		if tools != 1 {
			t.Errorf("chunk=%d toolCalls=%d, want 1 (one command_execution started)", chunk, tools)
		}
		if tokens != 11969+383 {
			t.Errorf("chunk=%d tokens=%d, want %d (input+output)", chunk, tokens, 11969+383)
		}
		// Last activity is the final agent message's first line.
		if last != "msg: Looks good." {
			t.Errorf("chunk=%d last=%q, want %q", chunk, last, "msg: Looks good.")
		}
	}
}

func TestParseJSON_IgnoresGarbage(t *testing.T) {
	// A non-JSON line (codex can interleave a stray notice) must not crash or corrupt state.
	tr := &liveTracker{cap: &capBuffer{max: 1 << 20}, jsonMode: true}
	feed(tr, "not json at all\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":5}}\n", 3)
	turns, _, tokens, _, _ := tr.progress()
	if turns != 1 || tokens != 15 {
		t.Fatalf("turns=%d tokens=%d, want 1 and 15 (garbage line ignored)", turns, tokens)
	}
}

func TestParseText_ReviewTrace(t *testing.T) {
	// The review human-trace shape: bare `exec` markers before each command, and a
	// `tokens used` line whose successor is the comma-grouped count.
	trace := strings.Join([]string{
		"OpenAI Codex v0.139.0",
		"exec",
		"/usr/sbin/bash -lc 'git diff master'",
		"exec",
		"/usr/sbin/bash -lc ls",
		"codex",
		"tokens used",
		"18,358",
		"Some reasoning about the diff.",
	}, "\n") + "\n"

	for _, chunk := range []int{1, 5, 4096} {
		tr := &liveTracker{cap: &capBuffer{max: 1 << 20}, jsonMode: false}
		feed(tr, trace, chunk)
		_, tools, tokens, _, last := tr.progress()
		if tools != 2 {
			t.Errorf("chunk=%d toolCalls=%d, want 2 (two `exec` markers)", chunk, tools)
		}
		if tokens != 18358 {
			t.Errorf("chunk=%d tokens=%d, want 18358 (comma-grouped count after `tokens used`)", chunk, tokens)
		}
		if last != "Some reasoning about the diff." {
			t.Errorf("chunk=%d last=%q, want last reasoning line", chunk, last)
		}
	}
}

func TestParseText_TokensUsedWithoutCountDoesNotSwallow(t *testing.T) {
	// If codex prints `tokens used` but the NEXT line isn't a count (e.g. it's the next
	// `exec` marker), the parser must not silently eat that line — the tool-call still
	// counts and tokens stays 0.
	tr := &liveTracker{cap: &capBuffer{max: 1 << 20}, jsonMode: false}
	feed(tr, "tokens used\nexec\n/usr/sbin/bash -lc ls\n", 6)
	_, tools, tokens, _, last := tr.progress()
	if tools != 1 {
		t.Errorf("toolCalls=%d, want 1 (the `exec` after a count-less `tokens used` must still count)", tools)
	}
	if tokens != 0 {
		t.Errorf("tokens=%d, want 0", tokens)
	}
	if last != "/usr/sbin/bash -lc ls" {
		t.Errorf("last=%q, want the command line", last)
	}
}

func TestParseInt_TrailingJunk(t *testing.T) {
	// codex could annotate the count; only the leading number must be read.
	if got := parseInt("18,358 (cached: 4,992)"); got != 18358 {
		t.Fatalf("parseInt with trailing junk = %d, want 18358", got)
	}
}

func TestParseText_TokensUsedNotConfusedByProse(t *testing.T) {
	// A `tokens used` substring inside prose (not a standalone line) must NOT arm the
	// number capture; only the exact bare line does.
	tr := &liveTracker{cap: &capBuffer{max: 1 << 20}, jsonMode: false}
	feed(tr, "no tokens used here, just 999 in prose\n", 4)
	_, _, tokens, _, _ := tr.progress()
	if tokens != 0 {
		t.Fatalf("tokens=%d, want 0 (mid-prose mention must not capture)", tokens)
	}
}

func TestHumanInt(t *testing.T) {
	cases := map[int]string{0: "0", 42: "42", 999: "999", 1000: "1.0k", 18358: "18.4k", 1_500_000: "1.5M"}
	for n, want := range cases {
		if got := humanInt(n); got != want {
			t.Errorf("humanInt(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestPlural(t *testing.T) {
	cases := []struct {
		n    int
		noun string
		want string
	}{
		{0, "turn", "0 turns"}, {1, "turn", "1 turn"}, {2, "turn", "2 turns"},
		{1, "tool-call", "1 tool-call"}, {47, "tool-call", "47 tool-calls"},
	}
	for _, c := range cases {
		if got := plural(c.n, c.noun); got != c.want {
			t.Errorf("plural(%d,%q) = %q, want %q", c.n, c.noun, got, c.want)
		}
	}
}

func TestMetricsLine(t *testing.T) {
	// review: no turns, tokens omitted when 0.
	if got := (result{Mode: "review", ToolCalls: 5, Tokens: 18358}).metricsLine(); got != "18.4k tokens · 5 tool-calls" {
		t.Errorf("review metricsLine = %q", got)
	}
	if got := (result{Mode: "review", ToolCalls: 0, Tokens: 0}).metricsLine(); got != "0 tool-calls" {
		t.Errorf("review zero metricsLine = %q (tokens must be omitted at 0)", got)
	}
	// exec: turns present.
	if got := (result{Mode: "exec", Turns: 1, ToolCalls: 3, Tokens: 24300}).metricsLine(); got != "24.3k tokens · 1 turn · 3 tool-calls" {
		t.Errorf("exec metricsLine = %q", got)
	}
}

func TestParseInt(t *testing.T) {
	cases := map[string]int{"18,358": 18358, "  42 ": 42, "1,000,000": 1000000, "": 0, "n/a": 0}
	for s, want := range cases {
		if got := parseInt(s); got != want {
			t.Errorf("parseInt(%q) = %d, want %d", s, got, want)
		}
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("\n  \nfirst real\nsecond"); got != "first real" {
		t.Fatalf("firstLine = %q, want %q", got, "first real")
	}
	if got := firstLine("   \n\n"); got != "" {
		t.Fatalf("firstLine(blank) = %q, want empty", got)
	}
}

func TestClip(t *testing.T) {
	if got := clip("hello", 10); got != "hello" {
		t.Errorf("clip short = %q", got)
	}
	if got := clip("hello world", 5); got != "hello…" {
		t.Errorf("clip long = %q, want %q", got, "hello…")
	}
	// Multi-byte runes must not be split mid-rune.
	if got := clip("héllo wörld", 4); got != "héll…" {
		t.Errorf("clip rune = %q, want %q", got, "héll…")
	}
}
