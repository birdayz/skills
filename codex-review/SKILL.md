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

## Step 0 — the invocation (no build step)

The tool is a single self-contained Go file at `codexreview/main.go` **next to this
SKILL.md** (stdlib-only, no deps). You **`go run` it directly** — there is no build, no
binary to manage, no `/tmp` path to remember. Use the absolute path of THIS skill's
directory (you were given it when the skill loaded — do NOT derive it from
`$0`/`readlink`; that's the shell, not the file).

```bash
SKILL_DIR="/abs/path/to/this/skill"     # the directory this SKILL.md lives in
RUN="GOWORK=off go run $SKILL_DIR/codexreview/main.go"
```

- **`go run <main.go>` runs with YOUR current directory** (the repo under review) — so codex and `git` see the right repo. This is why we pass the `.go` file, not the package directory: `go run <dir>` would fail with "outside main module" from inside another repo's module.
- **`GOWORK=off`** so an inherited `go.work` in the repo you're reviewing (which doesn't list this module) can't break the run with "not one of the workspace modules".
- The first run compiles in ~1s and is cached after; re-runs are instant.

Sanity-check once (optional): `eval "$RUN" -h | grep -q usage && echo ok`.

> **Do NOT wrap the invocation.** No `timeout …` (the tool has `--timeout`, default 25m, and process-group-kills a stuck run itself). No `2>/tmp/x >/tmp/y` redirects (the tool already saves the trace to `review.log` and the prose to `review.md`, and prints the verdict + stats to stdout). No `| tail`/`| grep` (see **Watching progress**). The whole point of the tool is that the correct invocation is *just the tool* — adding shell scaffolding around it re-introduces the very footguns it removes.

> **If you (the agent) are running inside a sandbox, add `--bypass-sandbox`.** codex sandboxes itself with `bwrap`, which **cannot nest** — inside another sandbox/agent, a plain run reviews on *less* evidence (it can't read the full repo) and silently misses findings rather than failing loudly. A Claude Code agent is frequently nested, so default to `--bypass-sandbox` unless you know you're in a bare human terminal. A review writes nothing, so there's no edit risk.

## Step 1 — run the review

Branch/PR diff review (the common case). The tool resolves the scratch dir, runs codex **read-only, non-interactive, with stdin closed and a hard timeout** (so it cannot hang), saves `review.md`/`review.log`, classifies the verdict, and — with `--post` — renders and posts the PR comment:

```bash
# Resolve the PR base (fallback to the repo default). The tool reviews the diff
# from <base> to HEAD.
base=$(gh pr view --json baseRefName -q .baseRefName 2>/dev/null \
       || git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null | sed 's@^origin/@@' \
       || echo main)

eval "$RUN" review --base "$base" --post
```

(`eval "$RUN"` expands the `GOWORK=off go run …/main.go` prefix; or just paste it inline. Nothing else wraps the call.)

Defaults: `--model gpt-5.5`, `--effort xhigh`, `--timeout 25m`. Override with `--model` / `--effort` / `--timeout`, or the `CODEX_REVIEW_MODEL` / `CODEX_REVIEW_REASONING_EFFORT` env vars. Other flags:

- `--base <branch|sha>` — review every commit since `<base>`. For a **delta re-review** after a fix, pass the previously-reviewed SHA: `--base <prev-sha>` (cheaper, focused).
- `--commit <sha>` — review a single commit instead.
- `--post` — render the comment from the Go template and post via `gh`. Omit it to only get the verdict locally (no PR needed).
- `--supersede` — with `--post`, post a **new** comment and **collapse** this tool's prior comments on the PR as "outdated" (GitHub's hide-as-outdated). A re-review loop then leaves one active comment with the earlier ones folded away — history preserved, no edit-in-place, no spam. Prior comments are found via an invisible marker the tool appends, so it works even with a custom `--template`. (`--update-last` is a deprecated alias.)
- `--pr <n>` / `--repo owner/name` — post target (default: current branch's PR, repo inferred from cwd).
- `--bypass-sandbox` — use `--dangerously-bypass-approvals-and-sandbox` instead of `-s read-only`. **Required when running inside another sandbox/agent** (codex's `bwrap` can't nest; without this it reviews on *less* evidence and can miss findings). A review writes nothing, so there's no edit risk. In a plain human terminal, omit it.
- `--keep-paths` — by default the tool rewrites codex's **absolute** file paths to **repo-relative** ones (`/home/you/proj/.claude/worktrees/r-14/apps/x.tsx:5` → `apps/x.tsx:5`), so a posted comment never leaks your local repo/worktree path and the paths stay clickable for everyone reading the PR. Pass `--keep-paths` to keep the raw absolute paths. (The full trace in `review.log` always keeps absolute paths — it's local debug.)
- `--heartbeat <dur>` — interval for the stderr progress line (default `15s`; `0` disables). See **Watching progress** below.
- `-v` / `--verbose` — stream codex's full live trace to stderr. **Async/backgrounded runs only — never on a synchronous call** (it dumps the entire multi-MB trace into your captured output). See **Sync vs async** below.

The tool prints `VERDICT: ACK|NAK|UNKNOWN`, the `review.md` path, the posted-comment URL (if `--post`), and the review prose. **It exits non-zero — and never reports an ACK — on a timeout, an empty review, or a quota/auth failure**, with the matching stderr signature, so an empty run can never be mistaken for a clean pass.

### Sync vs async — pick one before you launch

A review takes a few minutes. Decide up front how you'll run it. **If you're unsure, use synchronous** — it's the safe default and correct for almost everything; only go async when you have a specific reason (below).

- **Synchronous (default, recommended).** One foreground `Bash` call; you block until it exits, then read the `VERDICT:` line and prose from stdout. Simple and correct for almost everything. **Do NOT pass `-v`** — the verbose trace would flood the tool result with megabytes of codex's reasoning. The default heartbeat still prints to stderr; you'll see the heartbeats and the verdict together when the call returns. Bump `--timeout` for a very large diff so it isn't killed mid-review.
- **Asynchronous.** Launch the tool **backgrounded** (your harness's background-Bash mode) when you want to do other work while codex runs, or when the user wants to watch live. Here `-v` is appropriate: the live trace and heartbeats stream to the background process's output, which you poll on your own cadence. Read the final `VERDICT:` once it exits. Use **exactly one** level of backgrounding (harness background **or** `nohup … &`, never both) — the tool's own `--timeout` bounds the run.

### Watching progress — NEVER pipe codex through `tail`/`grep`

The tool is **already** built to show progress: a heartbeat on stderr every `--heartbeat` seconds reporting elapsed time, tokens used, turns (exec) / tool-calls, and the latest activity — e.g.

```
  · [3m12s · 18.4k tokens · 2 turns · 7 tool-calls] codex working · exec: git diff master
```

So **do not** wrap the tool (or raw `codex`) in `| tail`, `| grep`, `| head`, or `sed -n`. Two reasons, both fatal:

1. **It hides the live signal.** `tail`/`grep` line-buffer until EOF, so you see *nothing* until the run ends — the exact "18 minutes of blank output, is it hung?" problem the heartbeat exists to solve.
2. **It truncates the verdict.** `| tail -N` keeps the *last* N lines (the end of the review prose), and the `VERDICT:`/`ACK`/`NAK` line is near the top — so you silently drop the one line you needed.

If you want to follow the raw trace live, the tool already streams it to a file: **`tail -f <review.log>`** (path printed in the start banner) — that follows without buffering or truncating anything. To see more inline, use `-v` on an **async** run. Never filter the tool's own stdout/stderr through a pager.

### Steered analysis (not a diff review)

`codex review` **cannot take a focus prompt** — `--base`/`--commit` are mutually exclusive with `[PROMPT]` in codex. For a steered question ("what test gaps remain on this branch?", a targeted audit), use the tool's `exec` mode, which passes the prompt with stdin closed:

```bash
eval "$RUN" exec --prompt "Audit this branch for missing regression tests on the resource-limit paths" --timeout 15m
```

If the skill was invoked with `[focus instructions]` for a **branch/PR** review, tell the user the focus can't be injected into a diff review and either run the plain `--base` review (codex sees any findings docs/RFCs/TODOs in the diff itself) or, if they meant the working tree, use `exec`.

## Step 2 — classify and iterate

Read the verdict the tool printed (and `review.md`):

- **ACK / no blocking issues** → done; report to the user.
- **Findings / NAK** → treat each like any review finding: **reproduce it and judge its true severity first — don't trust a "minor / no bug" framing.** Fix the root cause, add a regression test that pins it (verify it fails *without* the fix), run the project's tests, commit. Then **re-review the delta**: `eval "$RUN" review --base <prev-sha> --post --supersede`. `--supersede` posts a fresh comment and collapses the tool's earlier comments as "outdated", so an N-round fix loop shows the latest review on top with the prior ones folded away (history kept, not deleted, not edited-in-place). Iterate until ACK on the current HEAD.

A codex ACK is only valid for the **exact HEAD it reviewed** — after any new commit, re-review (same SHA discipline as any reviewer).

## What the posted comment looks like

```
**Codex finished review in 2m21s** · `a1b2c3d`

---

<the review prose>

---

`18.4k tokens · 47 tool-calls` · Generated by Codex 0.139.0 on gpt-5.5 (xhigh) and [codex-review](https://github.com/birdayz/skills/tree/master/codex-review)
```

The footer carries **run stats** — tokens used and tool-calls codex made (plus turns in `exec` mode) — so a reader can see how much work the review actually did. (`codex-review` is a clickable link to the skill.) Override the template with `--template <file>` (a Go `text/template` with fields `.Kind .Duration .SHA .Review .Version .Model .Effort .SkillURL .Stats .Tokens .ToolCalls .Turns`).

## Why the tool (the gotchas it removes)

These are the traps a hand-written `codex` pipeline falls into — the tool handles each, but know them so you trust the output and can debug a weird run:

- **`codex exec` hangs forever on open stdin.** `codex exec` reads stdin to *append* to the prompt (`--help`: "if stdin is piped … appended as a `<stdin>` block"). With stdin left open it prints `Reading additional input from stdin...` and **blocks forever, empty output** — a `codex exec … | tail` hangs exactly this way. The tool sets `Stdin = nil` (→ /dev/null), so this is structurally impossible, plus a `--timeout` that process-group-kills a stuck run (codex + its `bwrap` children).
- **Empty output ≠ clean pass — it's a quota/auth failure.** codex exits **0 with no verdict** when the 5h/weekly rate limit is exhausted, when unauthenticated, or when it errors mid-stream. A zero-length review is a FAILED run, never an ACK. The tool refuses to report ACK on an empty review and surfaces the stderr signature (`rate limit`/`quota`/`429`/`not logged in`/…); confirm with `codex login status`.
- **Flag order is load-bearing.** `-m`, `-c model_reasoning_effort=…`, `-s|--bypass`, `-a` are global flags **before** the subcommand; `--base`/`--commit` come after `review`. Misordering errors with "unexpected argument". The tool always emits the correct order.
- **`--base`/`--commit` ⊕ `[PROMPT]`.** codex rejects a focus prompt combined with a branch/commit range. The tool keeps `review` (diff) and `exec` (steered) as separate modes and errors early if you mix them.
- **Don't double-background.** Run the tool with *one* level of backgrounding (your harness's background mode **or** `nohup … &`, never both) or just foreground it — the tool's own `--timeout` bounds the run. Doubling up makes the launching shell return instantly and you read a mid-flight log. Wait for the real exit, then read the verdict.
- **Never `| tail` / `| grep` the tool.** It buffers away the heartbeat (you go blind for minutes) AND truncates the `VERDICT:` line off the top. The tool already emits a heartbeat and streams the trace to `review.log` — follow that with `tail -f`, or use `-v` on an async run. Never pipe the tool's stdout/stderr through a pager. (See **Watching progress**.)
- **Clean tree first** — codex reads `git status`; stray edits widen/skew the review.

## Maintaining the tool

`codexreview/` is plain Go, no deps, `go vet`-clean. **Keep it a single `main.go`** (tests aside) — that's what lets the skill `go run main.go` directly from any repo without the "outside main module" error. If it ever must grow past one file, switch the skill's invocation to `go build`. Keep `gofmt`/`go vet` green. If codex changes its CLI, update the one encoding point in `main.go`:

- flag names / stdin behaviour / new quota signatures → `codexArgs` / `quotaRE` / `verdictLineRE` (cases in `verdict_test.go`).
- the **exec `--json` event schema** (`turn.completed.usage`, `item.{started,completed}` types) or the **review human-trace markers** (bare `exec`, the `tokens used` block) that the heartbeat parses → `liveTracker.parseJSON` / `parseText` (cases in `progress_test.go`, which feed real codex output shapes in byte-split chunks).

The tool is the single place that encodes how to talk to `codex` correctly — fix it there once, every caller benefits.
