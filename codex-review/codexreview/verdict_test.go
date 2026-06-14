package main

import "testing"

func TestClassifyVerdict(t *testing.T) {
	cases := []struct {
		name, review, want string
	}{
		// Standard verdict lines.
		{"bare ACK", "Looks fine.\n\nACK", "ACK"},
		{"bare NAK", "Found a bug.\n\nNAK", "NAK"},
		{"bold NAK", "...\n\n**NAK.**", "NAK"},
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
