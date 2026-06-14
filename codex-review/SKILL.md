---
name: codex-review
description: Kick off an external code review by OpenAI Codex (the `codex` CLI) on the current branch/PR, post it as a PR comment, and drive the fix→re-review loop. Use when the user asks for a "codex review", says "/codex-review", "kick off codex", or wants a second independent reviewer on a change. Repo-agnostic — works on any git repo with a GitHub PR.
user-invocable: true
allowed-tools: Bash, Read
argument-hint: "[--base <branch>] [--model <model>] [--effort <effort>] [--post] [focus instructions]"
---

# Codex Review

Run an independent code review by **OpenAI Codex** (the local `codex` CLI), post it to the PR, and iterate on findings like any other review gate. Works on any repository.

The fragile part — invoking `codex` so it can't hang, telling an empty/quota failure apart from a clean pass, parsing the verdict, and rendering+posting the PR comment — is done by the **`codexreview` Go tool** that ships with this skill (`codexreview/`). Drive the tool; don't hand-assemble `codex` shell pipelines (that's how the `codex exec` stdin-hang and the "empty file looks like an ACK" traps happen).

## CRITICAL: codex is MANUAL — trigger it, never wait for it

A codex review only happens because **someone ran `codex`**. There is no bot, webhook, GitHub App, or CI job that runs it. So **never poll or wait for a codex review you did not start** — if one is "expected", *run it yourself*. Codex posts under the **human's own GitHub account** (no `[bot]` login), so you can't identify its reviews by author.

## Preconditions & cost

- A full review is an LLM agent run: it **costs the user's Codex/ChatGPT credits and takes a few minutes** on a large diff. For a big change, say so before launching.
- `codex` installed and authenticated (`codex login status` reports a logged-in account; `which codex`).
- For posting: `gh` authenticated and the branch has an open PR (or pass `--pr <n>`). A review with no PR still works — just omit `--post` and read the verdict locally.
- **Commit or stash unrelated working-tree changes first.** Codex inspects the repo via `git status`/`git diff`; stray edits pollute the scope and the reported SHA.

## Step 0 — build the tool (once)

The tool is Go source in `codexreview/` **next to this SKILL.md**. Use the absolute
path of THIS skill's directory (you were given it when the skill loaded — do not try
to derive it from `$0`/`readlink`; that's the shell, not the file). Build once to a
stable path and verify the build before relying on it:

```bash
SKILL_DIR="/abs/path/to/this/skill"     # the directory this SKILL.md lives in
# cd into the module + GOWORK=off so an INHERITED go.work in the repo you're
# reviewing (which doesn't list this module) can't break the build with
# "not one of the workspace modules".
( cd "$SKILL_DIR/codexreview" && GOWORK=off go build -o /tmp/codexreview . ) \
  && /tmp/codexreview 2>&1 | grep -q 'usage:' \
  && echo "built + sane: /tmp/codexreview" \
  || { echo "BUILD FAILED — fix codexreview/ before reviewing"; exit 1; }
```

The tool has no third-party dependencies — the build is offline and instant. (`GOWORK=off go run "$SKILL_DIR/codexreview" …` also works and skips the explicit build.)

## Step 1 — run the review

Branch/PR diff review (the common case). The tool resolves the scratch dir, runs codex **read-only, non-interactive, with stdin closed and a hard timeout** (so it cannot hang), saves `review.md`/`review.log`, classifies the verdict, and — with `--post` — renders and posts the PR comment:

```bash
# Resolve the PR base (fallback to the repo default). The tool reviews the diff
# from <base> to HEAD.
base=$(gh pr view --json baseRefName -q .baseRefName 2>/dev/null \
       || git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null | sed 's@^origin/@@' \
       || echo main)

/tmp/codexreview review --base "$base" --post
```

Defaults: `--model gpt-5.5`, `--effort xhigh`, `--timeout 25m`. Override with `--model` / `--effort` / `--timeout`, or the `CODEX_REVIEW_MODEL` / `CODEX_REVIEW_REASONING_EFFORT` env vars. Other flags:

- `--base <branch|sha>` — review every commit since `<base>`. For a **delta re-review** after a fix, pass the previously-reviewed SHA: `--base <prev-sha>` (cheaper, focused).
- `--commit <sha>` — review a single commit instead.
- `--post` — render the comment from the Go template and post via `gh`. Omit it to only get the verdict locally (no PR needed).
- `--update-last` — with `--post`, edit the tool's previous comment on the PR instead of adding a new one (re-reviews don't spam; posts fresh if none exists). Found via an invisible marker the tool appends, so it works even with a custom `--template`.
- `--pr <n>` / `--repo owner/name` — post target (default: current branch's PR, repo inferred from cwd).
- `--bypass-sandbox` — use `--dangerously-bypass-approvals-and-sandbox` instead of `-s read-only`. **Required when running inside another sandbox/agent** (codex's `bwrap` can't nest; without this it reviews on *less* evidence and can miss findings). A review writes nothing, so there's no edit risk. In a plain human terminal, omit it.

The tool prints `VERDICT: ACK|NAK|UNKNOWN`, the `review.md` path, the posted-comment URL (if `--post`), and the review prose. **It exits non-zero — and never reports an ACK — on a timeout, an empty review, or a quota/auth failure**, with the matching stderr signature, so an empty run can never be mistaken for a clean pass.

### Steered analysis (not a diff review)

`codex review` **cannot take a focus prompt** — `--base`/`--commit` are mutually exclusive with `[PROMPT]` in codex. For a steered question ("what test gaps remain on this branch?", a targeted audit), use the tool's `exec` mode, which passes the prompt with stdin closed:

```bash
/tmp/codexreview exec --prompt "Audit this branch for missing regression tests on the resource-limit paths" --timeout 15m
```

If the skill was invoked with `[focus instructions]` for a **branch/PR** review, tell the user the focus can't be injected into a diff review and either run the plain `--base` review (codex sees any findings docs/RFCs/TODOs in the diff itself) or, if they meant the working tree, use `exec`.

## Step 2 — classify and iterate

Read the verdict the tool printed (and `review.md`):

- **ACK / no blocking issues** → done; report to the user.
- **Findings / NAK** → treat each like any review finding: **reproduce it and judge its true severity first — don't trust a "minor / no bug" framing.** Fix the root cause, add a regression test that pins it (verify it fails *without* the fix), run the project's tests, commit. Then **re-review the delta**: `/tmp/codexreview review --base <prev-sha> --post --update-last`. The `--update-last` edits the tool's previous comment instead of stacking a new one, so an N-round fix loop leaves ONE up-to-date codex comment, not N. Iterate until ACK on the current HEAD.

A codex ACK is only valid for the **exact HEAD it reviewed** — after any new commit, re-review (same SHA discipline as any reviewer).

## What the posted comment looks like

```
**Codex finished review in 2m21s** · `a1b2c3d`

---

<the review prose>

---

Generated by Codex 0.139.0 on gpt-5.5 (xhigh) and [codex-review](https://github.com/birdayz/skills/tree/master/codex-review)
```

(`codex-review` is a clickable link to the skill.) Override the template with `--template <file>` (a Go `text/template` with fields `.Kind .Duration .SHA .Review .Version .Model .Effort .SkillURL`).

## Why the tool (the gotchas it removes)

These are the traps a hand-written `codex` pipeline falls into — the tool handles each, but know them so you trust the output and can debug a weird run:

- **`codex exec` hangs forever on open stdin.** `codex exec` reads stdin to *append* to the prompt (`--help`: "if stdin is piped … appended as a `<stdin>` block"). With stdin left open it prints `Reading additional input from stdin...` and **blocks forever, empty output** — a `codex exec … | tail` hangs exactly this way. The tool sets `Stdin = nil` (→ /dev/null), so this is structurally impossible, plus a `--timeout` that process-group-kills a stuck run (codex + its `bwrap` children).
- **Empty output ≠ clean pass — it's a quota/auth failure.** codex exits **0 with no verdict** when the 5h/weekly rate limit is exhausted, when unauthenticated, or when it errors mid-stream. A zero-length review is a FAILED run, never an ACK. The tool refuses to report ACK on an empty review and surfaces the stderr signature (`rate limit`/`quota`/`429`/`not logged in`/…); confirm with `codex login status`.
- **Flag order is load-bearing.** `-m`, `-c model_reasoning_effort=…`, `-s|--bypass`, `-a` are global flags **before** the subcommand; `--base`/`--commit` come after `review`. Misordering errors with "unexpected argument". The tool always emits the correct order.
- **`--base`/`--commit` ⊕ `[PROMPT]`.** codex rejects a focus prompt combined with a branch/commit range. The tool keeps `review` (diff) and `exec` (steered) as separate modes and errors early if you mix them.
- **Don't double-background.** Run the tool with *one* level of backgrounding (your harness's background mode **or** `nohup … &`, never both) or just foreground it — the tool's own `--timeout` bounds the run. Doubling up makes the launching shell return instantly and you read a mid-flight log. Wait for the real exit, then read the verdict.
- **Clean tree first** — codex reads `git status`; stray edits widen/skew the review.

## Maintaining the tool

`codexreview/` is plain Go, no deps, `go vet`-clean. Keep `gofmt`/`go vet` green. If codex changes its CLI (flag names, the stdin behaviour, new quota signatures), update `codexArgs` / `quotaRE` / `verdictLineRE` in `main.go` (and the cases in `verdict_test.go`) and rebuild. The tool is the single place that encodes how to talk to `codex` correctly — fix it there once, every caller benefits.
