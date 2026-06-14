# birdayz-skills

Claude Code skills. Image generation, code review, and Minecraft NeoForge mod development.

## Install

### In Claude Code

```
/plugin marketplace add birdayz/skills
/plugin install image-generation@birdayz-skills
/plugin install codex-review@birdayz-skills
/plugin install minecraft-playtest-loop@birdayz-skills
/plugin install minecraft-mod-development@birdayz-skills
/plugin install minecraft-npc-design@birdayz-skills
```

### CLI

```
claude plugin marketplace add https://github.com/birdayz/skills
claude plugin install image-generation@birdayz-skills
claude plugin install codex-review@birdayz-skills
claude plugin install minecraft-playtest-loop@birdayz-skills
claude plugin install minecraft-mod-development@birdayz-skills
claude plugin install minecraft-npc-design@birdayz-skills
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

The key point: codex is **manual** — no bot or webhook runs it. The skill triggers it for you (`codex -m gpt-5.5 -c model_reasoning_effort=xhigh -s read-only -a never review --base <base>` by default), captures the review from stdout, and posts it with `gh` — so you never sit waiting for a review nobody started.

```
/codex-review [--base <branch>] [--model <model>] [--reasoning-effort <effort>] [focus instructions]
```

Requires the `codex` CLI (logged in) and `gh`. See the [SKILL.md](codex-review/SKILL.md) for details.

### Minecraft mod dev (`minecraft-playtest-loop`, `minecraft-mod-development`, `minecraft-npc-design`)

Agent-driven Minecraft **NeoForge (26.1.x)** mod development — build, verify, and polish a mod end to end with no human watching the screen.

- **`minecraft-playtest-loop`** — the offscreen foundation the other two build on: render the dev client/server on a virtual display nobody sees, capture pixels the agent `Read`s, drive the game via env-gated demo hooks in the mod, prove behavior with greppable functional asserts, and gate looks with LLM-judge loops. Bundled tools: `launch-offscreen.sh`, `capture.sh`, and a dependency-free Go `keysend` uinput injector. Requires `Xvfb`, `ffmpeg`, ImageMagick, and Mesa software GL (`llvmpipe`).
- **`minecraft-mod-development`** — scaffold a mod from scratch, the decompiled-API lookup discipline, unit-test-first strategy, headless offscreen capture, driving the live game on Wayland/sway, and a pile of hard-won MC 26.1.x API gotchas.
- **`minecraft-npc-design`** — offline-first custom-entity pipeline: JSON cube models, procedural atlas painting, a ~1s previewer, a persona-reviewer loop, and an auto-orbit in-game photo studio.

See [minecraft/README.md](minecraft/README.md) and the per-skill SKILL.md files.

## License

MIT
