# Minecraft mod-dev skills

Three skills that let an LLM agent build, verify, and polish a Minecraft (NeoForge 26.1.x) mod
**end to end without a human in the loop** — written while building `karoland`, a full theme-park
mod shipped as a kid's birthday present. The recurring problem they solve: the agent can't see a
game window, can't press keys, and a launch costs ~60s — so naive "edit, launch, ask the human to
look" iteration is hopeless. These skills replace the human's eyes and hands.

## [mod-development](mod-development/SKILL.md)
The base loop. Three rules that make agent-driven mod work fast and verifiable:

- **Logic lives in MC-free classes, iterated via JUnit** (seconds), never via game launches.
- **API truth comes from the decompiled jar on the build classpath** (`javap -p`), not from
  training data — MC renames half its API every version.
- **Visual/behavioral truth comes from a headless harness**: Xvfb + software GL + env-gated
  auto-demo hooks (`KAROLAND_*` vars that teleport cameras, drive rides, run scripted scenarios)
  + `import`/ffmpeg captures the agent can `Read` as images. Build → screenshot → fix, fully
  unattended, nothing ever appears on the user's screen — which also means no spoilers.

Bundled helper: [`mod-development/tools/keysend`](mod-development/tools/keysend/main.go) — a
dependency-free Go uinput keystroke injector (`keysend tap:t type:"/cmd" tap:enter`) for driving
a real on-screen game session on Wayland/sway, where no display-server input tooling exists.

## [npc-design](npc-design/SKILL.md)
Offline-first custom entity/character pipeline: author geometry as JSON cube models, paint
atlases procedurally (Go), generate faces with an image model against reference photos, and
**iterate in a ~1s offline previewer instead of the game**. In-game verification is reduced to
one "photo studio" launch: an env hook spawns the entity on a floating stage and auto-orbits the
camera 360° while the agent captures stills — every angle, zero camera math, one launch.

## [playtest](playtest/SKILL.md)
Unattended QA with two layers:

1. **Functional asserts**: env-gated test hooks inside the mod (`KAROLAND_RINGTEST=1` etc.) that
   drive a real headless client through a feature server-side and log one greppable `PASS`/`FAIL`
   line — slide carries a rider to the pool at full health, minigame round pays out, rocket
   launches and lands. CI-grade proof that gameplay *works*, not just compiles.
2. **LLM-judge review loops**: capture screenshots from the headless harness, hand them to a
   persona subagent with a rubric, and iterate until it scores 10/10. Two judges proved useful:
   a kid-experience judge (catches "the last-collectible sadness spiral", unreadable signage,
   drowning risk) and a domain-accuracy judge (scored a Falcon 9 replica 8/10 and demanded leg
   deploy, real stage separation, and an LZ-1 divert before giving 10/10). The judge's blockers
   become the next edit list; the loop terminates on a verifiable score.

The combination is the point: **asserts prove it works, captures prove it looks right, judges
prove it's good** — and all three run without a human, so the agent can vibe-code for hours and
only surface finished, reviewed features.
