# Minecraft mod-dev skills

Skills that let an LLM agent build, verify, and polish a Minecraft (NeoForge 26.1.x) mod
**end to end without a human in the loop** — written while shipping a full theme-park mod
unattended. The recurring problem they solve: the agent can't see a game window, can't press keys,
and a launch costs ~60s — so naive "edit, launch, ask the human to look" iteration is hopeless.
These skills replace the human's eyes and hands.

## [minecraft-playtest-loop](playtest-loop/SKILL.md)
The offscreen foundation the other two build on: unattended visual iteration for mod dev. Render the
dev client/server on a virtual display nobody sees (Xvfb + `llvmpipe` software GL), drive the game via
env-gated demo hooks compiled into the mod, gate captures on the MC log, prove gameplay with greppable
functional asserts, and gate looks with LLM-judge review loops. Bundled tools:
[`playtest-loop/tools`](playtest-loop/tools/) — `launch-offscreen.sh`, `capture.sh`, and a
dependency-free Go `keysend` uinput injector.

## [minecraft-mod-development](mod-development/SKILL.md)
The base loop for NeoForge mod work. Three rules that make agent-driven mod work fast and verifiable:

- **Logic lives in MC-free classes, iterated via JUnit** (seconds), never via game launches.
- **API truth comes from the decompiled jar on the build classpath** (`javap -p` / grep the sources
  jar), not from training data — MC renames half its API every version.
- **Visual/behavioral truth comes from a headless harness**: Xvfb + software GL + env-gated
  auto-demo hooks (vars that teleport cameras, drive rides, run scripted scenarios)
  + `import`/ffmpeg captures the agent can `Read` as images. Build → screenshot → fix, fully
  unattended, nothing ever appears on the user's screen.

Plus a pile of hard-won MC 26.1.x API gotchas: minecart client-sim & the armor-stand-seat trick,
the render-pipeline rework, programmatic world-building. Bundled helper:
[`mod-development/tools/keysend`](mod-development/tools/keysend/main.go) — a dependency-free Go
uinput keystroke injector for driving a real on-screen game session on Wayland/sway, where no
display-server input tooling exists.

## [minecraft-npc-design](npc-design/SKILL.md)
Offline-first custom entity/character pipeline: author geometry as JSON cube models, paint atlases
procedurally (Go), generate faces with an image model against reference photos, and **iterate in a
~1s offline previewer instead of the game**. In-game verification is reduced to one "photo studio"
launch: an env hook spawns the entity on a floating stage and auto-orbits the camera 360° while the
agent captures stills — every angle, zero camera math, one launch. Bundled reference tools under
[`npc-design/tools/art-pipeline`](npc-design/tools/art-pipeline/).
