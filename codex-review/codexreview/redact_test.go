package main

import "testing"

// TestStripRepoPaths pins the rewrite that keeps a posted comment from leaking the
// reviewer's local repo/worktree path: codex reports findings with the absolute path of
// the directory it ran in, and we collapse the repo-root prefix to a repo-relative tail.
func TestStripRepoPaths(t *testing.T) {
	cases := []struct {
		name   string
		review string
		roots  []string
		want   string
	}{
		{
			name:   "worktree path via exact root",
			review: "[P1] foo — /home/me/proj/.claude/worktrees/review-14/apps/adp-ui/src/x.tsx:456-462",
			roots:  []string{"/home/me/proj/.claude/worktrees/review-14"},
			want:   "[P1] foo — apps/adp-ui/src/x.tsx:456-462",
		},
		{
			name:   "worktree path via best-effort fallback (root not supplied)",
			review: "see /home/me/proj/.claude/worktrees/review-14/apps/adp-ui/src/x.tsx:5 for details",
			roots:  nil,
			want:   "see apps/adp-ui/src/x.tsx:5 for details",
		},
		{
			name:   "plain repo root (no worktree)",
			review: "bug at /home/me/proj/internal/server.go:120",
			roots:  []string{"/home/me/proj"},
			want:   "bug at internal/server.go:120",
		},
		{
			name:   "multiple paths in one finding",
			review: "/home/me/proj/a.go:1 calls /home/me/proj/b.go:2",
			roots:  []string{"/home/me/proj"},
			want:   "a.go:1 calls b.go:2",
		},
		{
			name:   "longest root wins (nested roots, sorted by caller)",
			review: "x /home/me/proj/.claude/worktrees/r/pkg/y.go:9",
			roots:  []string{"/home/me/proj/.claude/worktrees/r", "/home/me/proj"},
			want:   "x pkg/y.go:9",
		},
		{
			name:   "path outside the repo is left alone",
			review: "global config at /etc/codex/config.toml is fine",
			roots:  []string{"/home/me/proj"},
			want:   "global config at /etc/codex/config.toml is fine",
		},
		{
			name:   "bare repo-root mention without a trailing path is untouched",
			review: "the repo /home/me/proj looks clean",
			roots:  []string{"/home/me/proj"},
			want:   "the repo /home/me/proj looks clean",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := stripRepoPaths(c.review, c.roots); got != c.want {
				t.Fatalf("stripRepoPaths(%q)\n  = %q\n want %q", c.review, got, c.want)
			}
		})
	}
}
