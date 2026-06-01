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

codex -m "$model" -c model_reasoning_effort="$reasoning_effort" -s read-only -a never review --base "$base" \
  > /tmp/codex-review.md 2> /tmp/codex-review.log
echo "exit=$?  review lines=$(wc -l < /tmp/codex-review.md)"
```

- `--model <model>` / `CODEX_REVIEW_MODEL` — Codex model to pass as `-m <model>`; default `gpt-5.5`.
- `--reasoning-effort <effort>` / `CODEX_REVIEW_REASONING_EFFORT` — reasoning effort to pass as `-c model_reasoning_effort=<effort>`; default `xhigh`.
- `-s read-only` — codex may read files and run git, but **never edits** the tree.
- `-a never` — never block on an approval prompt: the run **can't hang** and **won't auto-post** (codex's GitHub-posting tool is approval-gated and is cleanly skipped under `never`), so you stay in control of what gets posted.
- `--base <branch>` — review every commit on the branch since `<branch>`. For a **delta re-review** after addressing findings, pass the previously-reviewed SHA instead: `--base <prev-sha>` (reviews only the new commits — cheaper and focused).
- `--commit <sha>` — review a single commit instead. NOTE: `--commit` **cannot** be combined with a custom `[PROMPT]`.
- Optional custom focus (allowed with `--base`, not `--commit`): append a prompt, e.g. `... review --base "$base" "Focus on concurrency and error handling."` (pass the skill's `[focus instructions]` argument here).

**Output split:** the **final review prose is on stdout**; the full trace — tool calls, the diff, the model's reasoning — is on **stderr** (large). Read stdout for the verdict; dip into stderr only to see what codex actually inspected.

A long review can exceed a foreground timeout — run it with `run_in_background: true` (or a generous `timeout`) and read the output file when it finishes.

## Step 2 — post it to the PR

Under `-a never` codex did **not** post; you do, so the review is on the record next to any other reviewers:

```bash
pr=$(gh pr view --json number -q .number)     # current branch's PR
sha=$(git rev-parse --short HEAD)
gh pr comment "$pr" --body "$(printf '## Codex review of %s\n\n%s' "$sha" "$(cat /tmp/codex-review.md)")"
```

(`gh` infers the repo from the working directory — no hard-coded owner/repo. Pass `--repo owner/name` and an explicit `pr` only if you're outside the checkout.)

## Step 3 — classify and iterate

Read `/tmp/codex-review.md` and classify the verdict (it usually ends with an `ACK` / `NAK` / a findings list):

- **ACK / no blocking issues** → done; report to the user.
- **Findings** → treat each like any review finding: **reproduce it and judge its true severity first — don't trust a "minor / no bug" framing at face value.** Fix the root cause, add a regression test that pins it (verify the test fails *without* the fix), run the project's tests, commit. Then **re-review the delta**: `codex -m "$model" -c model_reasoning_effort="$reasoning_effort" -s read-only -a never review --base <prev-sha>` and post again. Iterate until ACK on the current HEAD.

A codex ACK is only valid for the **exact HEAD it reviewed** — after any new commit, re-review (same SHA discipline as any reviewer).

## One-shot recipe

```bash
model="${CODEX_REVIEW_MODEL:-gpt-5.5}"
reasoning_effort="${CODEX_REVIEW_REASONING_EFFORT:-xhigh}"
base=$(gh pr view --json baseRefName -q .baseRefName 2>/dev/null || echo main)
pr=$(gh pr view --json number -q .number)
codex -m "$model" -c model_reasoning_effort="$reasoning_effort" -s read-only -a never review --base "$base" > /tmp/codex-review.md 2> /tmp/codex-review.log
gh pr comment "$pr" --body "$(printf '## Codex review of %s\n\n%s' "$(git rev-parse --short HEAD)" "$(cat /tmp/codex-review.md)")"
sed -n '1,80p' /tmp/codex-review.md   # read the verdict
```

## Gotchas

- **Don't wait for what you didn't trigger** — codex is manual; if a review is "expected", run it.
- **Flag order**: `-m`, `-c model_reasoning_effort=...`, `-s`, and `-a` are top-level, *before* `review`; `--base`, `--commit`, and `--title` are review options after `review`. Placing global flags after `review` errors with "unexpected argument".
- **`--commit` + custom prompt** is rejected by the arg parser — use `--base` if you need a prompt, or drop the prompt.
- **Clean tree first** — codex reads `git status`; stray edits widen or skew the review.
- **Heading varies between passes** — if you ever scan for codex's own posted comments, its heading changes across passes (e.g. an initial review vs a re-review); match broadly, not on a fixed prefix.
