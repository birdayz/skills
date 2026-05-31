# birdayz-skills

Claude Code skills. Image generation, code review, that sort of thing.

## Install

### In Claude Code

```
/plugin marketplace add birdayz/skills
/plugin install image-generation@birdayz-skills
/plugin install codex-review@birdayz-skills
```

### CLI

```
claude plugin marketplace add https://github.com/birdayz/skills
claude plugin install image-generation@birdayz-skills
claude plugin install codex-review@birdayz-skills
```

## Skills

### generate-with-refs

Generate images with Google Gemini using reference images. Prevents hallucinations by grounding generation in actual visual references.

The key idea: generation is driven by a markdown spec file (`*-prompt.md`), not chat prompts. When the AI needs to change something, it edits the spec. This is 100x more effective than wildly prompting in chat — you get reproducible, iteratable results.

```
/generate-with-refs path/to/folder
```

The folder needs a `*-prompt.md` and reference images. See the [SKILL.md](generate-with-refs/SKILL.md) for details.

Requires `GEMINI_API_KEY` or `GOOGLE_API_KEY` env var and Go toolchain.

### codex-review

Kick off an independent code review by **OpenAI Codex** (the `codex` CLI) on the current branch/PR, post it as a PR comment, and drive the fix→re-review loop. Repo-agnostic — works on any git repo with a GitHub PR.

The key point: codex is **manual** — no bot or webhook runs it. The skill triggers it for you (`codex -s read-only -a never review --base <base>`), captures the review from stdout, and posts it with `gh` — so you never sit waiting for a review nobody started.

```
/codex-review [--base <branch>] [focus instructions]
```

Requires the `codex` CLI (logged in) and `gh`. See the [SKILL.md](codex-review/SKILL.md) for details.

## License

MIT
