---
name: codex-review
description: Kick off an external code review by OpenAI Codex (the `codex` CLI) on the current branch/PR, post it as a PR comment, and drive the fix→re-review loop. Use when the user asks for a "codex review", says "/codex-review", "kick off codex", or wants a second independent reviewer on a change. Repo-agnostic — works on any git repo with a GitHub PR.
user-invocable: true
allowed-tools: Bash, Read
argument-hint: "[--base <branch>] [--model <model>] [--reasoning-effort <effort>] [focus instructions]"
---

# Codex Review

Run an independent code review by **OpenAI Codex** (the local `codex` CLI) on the current branch, post it to the PR, and iterate on findings like any other review gate. Works on any repository.

## CRITICAL: codex is MANUAL — trigger it, never wait for it

A codex review only happens because **someone ran the `codex` CLI**. There is no bot, webhook, GitHub App, or CI job that runs it automatically. Therefore:

- **Never poll or wait for a codex review you did not start** — it will never arrive on its own. If a codex review is "expected", that means *run it yourself* with the steps below.
- Codex posts under the **human's own GitHub account**, not a dedicated `[bot]` login — so you can't identify its reviews by author, and a hand-pasted review looks identical to a CLI-posted one.

## Preconditions & cost

- A full review is an LLM agent run: it **costs the user's Codex/ChatGPT credits and can take a few minutes** on a large diff. For a big change, say so before launching.
- `codex` installed and authenticated: `codex login status` should report a logged-in account. (`which codex` to confirm it's on PATH.)
- `gh` authenticated, and the current branch has an open PR (or you'll pass an explicit PR number).
- **Commit or stash unrelated working-tree changes first.** Codex inspects the repo via `git status` / `git diff`; stray edits and scratch files pollute the review scope and the reported SHA.

## Step 1 — run the review (non-interactive, read-only, no hang)

By default, run reviews with `gpt-5.5` and `xhigh` reasoning effort. The caller may choose different settings with `--model <model>` and `--reasoning-effort <effort>`; if they do not, pass these defaults explicitly. When invoked as a skill, parse those options from the user's request and assign the shell variables below; the environment variables are just an automation-friendly equivalent.

Global flags go **before** the `review` subcommand:

```bash
# Defaults unless the caller supplied --model / --reasoning-effort.
model="${CODEX_REVIEW_MODEL:-gpt-5.5}"
reasoning_effort="${CODEX_REVIEW_REASONING_EFFORT:-xhigh}"

# Resolve the PR's base branch (fallback to the repo's default branch).
base=$(gh pr view --json baseRefName -q .baseRefName 2>/dev/null \
       || git symbolic-ref --short refs/remotes/origin/HEAD 2>/dev/null | sed 's@^origin/@@' \
       || echo main)

# Per-repo + per-branch scratch dir. No fixed filenames, so concurrent reviews
# (another repo, PR, or session) never clash. Step 2 recomputes the SAME path
# from the same repo+branch, so nothing has to be threaded between the shells.
b=$(git rev-parse --abbrev-ref HEAD)
dir="${TMPDIR:-/tmp}/codex-review/$(git rev-parse --show-toplevel | cksum | cut -d' ' -f1)-${b//[^A-Za-z0-9._-]/_}"
mkdir -p "$dir"

# Time the run — duration/model/effort go in the posted comment.
start=$(date +%s)
codex -m "$model" -c model_reasoning_effort="$reasoning_effort" -s read-only -a never review --base "$base" \
  > "$dir/review.md" 2> "$dir/review.log"
rc=$?
echo "exit=$rc  review lines=$(wc -l < "$dir/review.md")  dir=$dir"
dur=$(( $(date +%s) - start ))
# Hand off run metadata to Step 2: line 1 duration, 2 model, 3 effort.
printf '%s\n%s\n%s\n' "$((dur/60))m$((dur%60))s" "$model" "$reasoning_effort" > "$dir/meta.txt"
# An empty review file is NOT a clean pass. codex can exit 0 with NO verdict when
# it is rate-limited (5h/weekly quota), unauthenticated, or errored mid-run — the
# cause is in the stderr log, never the (empty) stdout review. Always inspect:
if [ "$rc" -ne 0 ] || [ ! -s "$dir/review.md" ]; then
  echo "!! codex produced no review — scanning stderr for quota/auth/errors:"
  grep -iE 'rate.?limit|quota|usage|429|too many|reached|not logged in|unauthor|stream error|error:' "$dir/review.log" | tail -20
  echo "(full stderr at $dir/review.log; also try 'codex login status' and chatgpt.com/codex/settings/usage)"
fi
```

- `--model <model>` / `CODEX_REVIEW_MODEL` — Codex model to pass as `-m <model>`; default `gpt-5.5`.
- `--reasoning-effort <effort>` / `CODEX_REVIEW_REASONING_EFFORT` — reasoning effort to pass as `-c model_reasoning_effort=<effort>`; default `xhigh`.
- `-s read-only` — codex may read files and run git, but **never edits** the tree.
- `-a never` — never block on an approval prompt: the run **can't hang** and **won't auto-post** (codex's GitHub-posting tool is approval-gated and is cleanly skipped under `never`), so you stay in control of what gets posted.
- `--base <branch>` — review every commit on the branch since `<branch>`. For a **delta re-review** after addressing findings, pass the previously-reviewed SHA instead: `--base <prev-sha>` (reviews only the new commits — cheaper and focused).
- `--commit <sha>` — review a single commit instead.
- **A custom focus `[PROMPT]` is mutually exclusive with BOTH `--base` and `--commit`** in current codex: `codex review --base "$base" "Focus on X"` fails with `error: the argument '--base <BRANCH>' cannot be used with '[PROMPT]'` (same for `--commit`). A bare prompt (`codex review "Focus on X"`, or with `--uncommitted`) reviews the working-tree changes, NOT a branch/commit range. So for a PR/branch review you CANNOT steer with a focus prompt — run `--base "$base"` plain and rely on the diff itself (any findings docs, RFCs, or TODOs in the diff are visible to codex). If the skill was invoked with `[focus instructions]`, tell the user the focus can't be injected into a branch-scoped review and proceed with the plain `--base` review (or, only if the intent is the uncommitted working tree, drop `--base` and pass the prompt).

**Output split:** the **final review prose is on stdout**; the full trace — tool calls, the diff, the model's reasoning — is on **stderr** (large). Read stdout for the verdict; dip into stderr only to see what codex actually inspected.

A long review can exceed a foreground timeout — run it with `run_in_background: true` (or a generous `timeout`) and read the output file when it finishes.

## Step 2 — post it to the PR

Under `-a never` codex did **not** post; you do, so the review is on the record next to any other reviewers. Wrap the review in a header line (review duration, in the Claude-Code-review style) and a footer attribution:

```bash
# Recompute the SAME scratch dir Step 1 used (this runs in a separate shell).
b=$(git rev-parse --abbrev-ref HEAD)
dir="${TMPDIR:-/tmp}/codex-review/$(git rev-parse --show-toplevel | cksum | cut -d' ' -f1)-${b//[^A-Za-z0-9._-]/_}"

# Never post an empty review (codex errored, or wrote only whitespace) — abort instead.
grep -q '[^[:space:]]' "$dir/review.md" 2>/dev/null || { echo "review is empty — see $dir/review.log; not posting"; exit 1; }

pr=$(gh pr view --json number -q .number)            # current branch's PR
sha=$(git rev-parse --short HEAD)
[ -f "$dir/meta.txt" ] && { read -r duration; read -r model; read -r effort; } < "$dir/meta.txt"   # written in Step 1
dur_phrase=${duration:+ in $duration}                # " in 2m21s", or "" if unknown
model=${model:-gpt-5.5}; effort=${effort:-xhigh}     # fall back to the skill defaults
version=$(codex --version 2>/dev/null | grep -oE '[0-9]+(\.[0-9]+)+' | head -n1); version=${version:-unknown}  # bare semver, e.g. 0.55.0
skill_url="https://github.com/birdayz/skills/tree/master/codex-review"

gh pr comment "$pr" --body "$(printf '**Codex finished review%s** · `%s`\n\n---\n\n%s\n\n---\n\nGenerated by Codex %s on %s (%s) and [codex-review](%s)' \
  "$dur_phrase" "$sha" "$(cat "$dir/review.md")" "$version" "$model" "$effort" "$skill_url")"
```

The posted comment then reads:

```
**Codex finished review in 2m21s** · `a1b2c3d`

---

<the review prose>

---

Generated by Codex 0.139.0 on gpt-5.5 (xhigh) and [codex-review](https://github.com/birdayz/skills/tree/master/codex-review)
```

(`codex-review` renders as a clickable link to the skill folder.)

(`gh` infers the repo from the working directory — no hard-coded owner/repo. Pass `--repo owner/name` and an explicit `pr` only if you're outside the checkout.)

## Step 3 — classify and iterate

Read the review (`$dir/review.md`, the scratch dir Step 1 printed) and classify the verdict (it usually ends with an `ACK` / `NAK` / a findings list):

- **ACK / no blocking issues** → done; report to the user.
- **Findings** → treat each like any review finding: **reproduce it and judge its true severity first — don't trust a "minor / no bug" framing at face value.** Fix the root cause, add a regression test that pins it (verify the test fails *without* the fix), run the project's tests, commit. Then **re-review the delta**: `codex -m "$model" -c model_reasoning_effort="$reasoning_effort" -s read-only -a never review --base <prev-sha>` and post again. Iterate until ACK on the current HEAD.

A codex ACK is only valid for the **exact HEAD it reviewed** — after any new commit, re-review (same SHA discipline as any reviewer).

## One-shot recipe

```bash
model="${CODEX_REVIEW_MODEL:-gpt-5.5}"
reasoning_effort="${CODEX_REVIEW_REASONING_EFFORT:-xhigh}"
base=$(gh pr view --json baseRefName -q .baseRefName 2>/dev/null || echo main)
pr=$(gh pr view --json number -q .number)
skill_url="https://github.com/birdayz/skills/tree/master/codex-review"
dir=$(mktemp -d)                                     # unique per run — no fixed names, no clash
start=$(date +%s)
codex -m "$model" -c model_reasoning_effort="$reasoning_effort" -s read-only -a never review --base "$base" > "$dir/review.md" 2> "$dir/review.log"
grep -q '[^[:space:]]' "$dir/review.md" 2>/dev/null || { echo "review is empty — see $dir/review.log; not posting"; exit 1; }
dur=$(( $(date +%s) - start )); duration="$((dur/60))m$((dur%60))s"; dur_phrase=${duration:+ in $duration}   # self-contained: single shell, no handoff
sha=$(git rev-parse --short HEAD)
version=$(codex --version 2>/dev/null | grep -oE '[0-9]+(\.[0-9]+)+' | head -n1); version=${version:-unknown}
gh pr comment "$pr" --body "$(printf '**Codex finished review%s** · `%s`\n\n---\n\n%s\n\n---\n\nGenerated by Codex %s on %s (%s) and [codex-review](%s)' \
  "$dur_phrase" "$sha" "$(cat "$dir/review.md")" "$version" "$model" "$reasoning_effort" "$skill_url")"
sed -n '1,80p' "$dir/review.md"   # read the verdict
```

## Gotchas

- **Don't wait for what you didn't trigger** — codex is manual; if a review is "expected", run it.
- **Flag order**: `-m`, `-c model_reasoning_effort=...`, `-s`, and `-a` are top-level, *before* `review`; `--base`, `--commit`, and `--title` are review options after `review`. Placing global flags after `review` errors with "unexpected argument".
- **Custom `[PROMPT]` + `--base`/`--commit`** is rejected by the arg parser (`'--base <BRANCH>' cannot be used with '[PROMPT]'`). A focus prompt only works on the working-tree scope (bare `review` / `--uncommitted`), never on a branch or commit range — for a PR review, drop the prompt and run `--base` plain.
- **Need a *steered* analysis that `review` can't express** (e.g. "what test gaps remain on this branch?", a targeted audit — not a diff review)? Use `codex exec`, NOT `review`: `codex -s read-only -a never exec "PROMPT" < /dev/null > out 2> log`. The **`< /dev/null` is mandatory** in any non-interactive/background context — `codex exec` reads stdin to *append* to the prompt (per `--help`: "if stdin is piped and a prompt is also provided, stdin is appended as a `<stdin>` block"), so with stdin left open it prints `Reading additional input from stdin...` and **blocks forever, producing empty output**. (`-s`/`-a` are still top-level, before `exec`.)
- **Empty review ≠ clean pass — it's usually a quota/auth failure.** codex exits **0 with no verdict** when the 5h or weekly rate limit is exhausted (it prints the limit interactively but not always to non-interactive stderr), when unauthenticated, or when it errors mid-run. A zero-length `review.md` is a FAILED run, never an ACK — scan the stderr log (`grep -iE 'rate.?limit|quota|usage|429|not logged in' "$dir/review.log"`), confirm with `codex login status`, and re-run after the quota resets. NEVER report "codex found nothing" off an empty file. (Both stdout and stderr are always redirected to files precisely so this is inspectable.)
- **Clean tree first** — codex reads `git status`; stray edits widen or skew the review.
- **Heading varies between passes** — if you ever scan for codex's own posted comments, its heading changes across passes (e.g. an initial review vs a re-review); match broadly, not on a fixed prefix.
