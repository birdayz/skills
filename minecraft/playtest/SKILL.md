---
name: playtest
description: How to playtest + QA the karoland park (or any feature) WITHOUT spoiling the surprise — the offscreen capture harness, automated "proven-fun" functional tests, the screenshot/video capture pipeline, and the Aphmau reviewer loop that iterates a zone/activity to a 10/10. Use whenever asked to playtest, QA, "prove it's fun", record a video, or review/iterate a karoland zone or activity.
---

# Playtest & Assess Loop (karoland)

The goal: prove every activity actually **works and is fun**, and iterate each zone to a **10/10**
from the Aphmau reviewer — all **offscreen** so the birthday surprise is never shown on a real screen.

## The one rule: stay offscreen when she's around
The recipient (Karolina) must not see the gift early. NEVER launch the visible play client
(`/tmp/launch_play.sh`, real display) when she could see it. The offscreen harness renders to a
headless Xvfb display (`:99`) with `WAYLAND_DISPLAY` unset, so GLFW can't reach the real monitor —
it is invisible. ALL playtest/QA/video work uses the offscreen path. Helpers:
- `/tmp/rebuild.sh` — kill MC, WIPE the faire region, relaunch the offscreen demo (full fresh build).
- `/tmp/redemo.sh` — kill MC, relaunch the offscreen demo WITHOUT wiping (demo/vantage iteration; the park persists via its region sentinel).
- `/tmp/launch_demo.sh` — the offscreen demo itself: `DISPLAY=:99`, `WAYLAND_DISPLAY` unset, software GL (`LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe`), `KAROLAND_DEMO=1`, `--no-daemon`.

## The loop (per zone / activity)
1. **Build/iterate** the feature in `ParkBuilder` / `KarolandEvents` (unit-testable logic in `RaceLogic`).
2. **Prove it's FUN (functional QA)** — see below. Don't trust the eye; assert behaviour from logs.
3. **Capture** stills/video of it (offscreen).
4. **Aphmau review** the capture → score + concrete blockers.
5. If < 10/10: implement the exact blockers, `rebuild`, re-capture, re-review. Repeat.
6. Commit each pass (no "claude" co-author).

## Proven-fun = functional assertions, not vibes
For every INTERACTIVE activity, add a one-shot test in `KarolandEvents.runDemo` that teleports the
demo player onto it and LOGS the outcome over a few ticks. Read the log to assert PASS. Examples we
already use (search `KAROLAND ELEVTEST`/`BALLOONTEST`/`LAUNCHTEST`):
- **Launchers / elevator / bounce** (barrel blast, warp star, drop tower, bubble elevator, gumdrop
  slime): tp the player on, then log `player.getY()` every N ticks. PASS = Y climbs to the expected
  peak then descends (e.g. barrel Y100→114; elevator Y104→139). A flat/low Y = blocked (canopy hit,
  no headroom, water not flowing) → fix the obstruction.
- **Held items** (balloon): `setItemInHand` the item, log Y — PASS = gentle rise (levitation). Clear
  the hand / remove LEVITATION before unrelated tests or they contaminate each other.
- **Fireworks lever / jingle**: call the show, grep the log for the trigger + the absence of a
  `hiccup` warning.
- **Chests** (treasure, build sandbox, dig pit, maze prize): they're filled at build time; a missing
  `fillChest failed` in the log = PASS. Verify the goodies list in code.
- Gate each test behind a demo `phase ==` so they run in sequence each loop without colliding.

Pattern: `else if (phase == N) { p.setGameMode(SURVIVAL); clear hand/effects; teleportTo(pad); }
else if (phase in (N, N+45] && phase%5==0) { LOGGER.info("Karoland XTEST: Y={}", (int)p.getY()); }`

## Capture pipeline (offscreen)
- **Still:** `DISPLAY=:99 import -window root /tmp/shot.png` then Read it.
- **Video:** `ffmpeg -f x11grab -draw_mouse 0 -video_size 1280x720 -framerate 30 -i :99 -t <sec> -pix_fmt yuv420p -movflags +faststart /tmp/out.mp4`. Make a GIF with `-vf "fps=10,scale=480:-1:flags=lanczos"`.
- **Sync captures to the demo**: the demo logs `Karoland demo: vantage cycle start` at phase 10 each
  loop. Wait for that marker, then `sleep` into the window you want (orbit 10–149, ground TOUR
  150–329, race 330+). The TOUR is an array of close-up vantages in `KarolandEvents.TOUR` — put the
  thing you're reviewing as TOUR[0] for a reliable, repeatable capture (later vantages compress when
  the first heavy build lags the server, so capture on a 2nd cycle or use TOUR[0]).
- llvmpipe fogs distant geometry — capture CLOSE/ground-level for review; don't judge color/detail
  from a far orbit shot. Interiors render dark (atmospheric) — judge design, not brightness.

## The Aphmau reviewer
Spawn a general-purpose subagent role-playing **Aphmau** (warm, kid-focused, but concrete +
buildable feedback). Give it the capture path(s) + a list of what's built, and ask for: first
impression, a **/10 score**, and the **exact ranked blockers** to a 10 (or "it's a 10 and why").
Feed her blockers straight back into the build. She reliably catches: empty grass (needs density),
weak/unreadable focal points (castle/temple/emblem must be the clear icon), generic glow vs a
readable branded payoff, off-palette blocks, and "nothing to DO" (every zone needs a real activity).
Her jungle arc went 4.5 → 8.5 → 9 → 9.5 → 10 across these exact fixes.

## High-value polish bar (what 10/10 needs)
- **Density**: no empty lawn — themed ground (podzol/moss for jungle, candy terracotta for Kirby),
  packed props, trees on a jittered grid right up to the path.
- **A clear lit focal point** per zone, with a **readable branded payoff** (a generated poster/emblem
  on a dark backing, haloed by light — not a generic glow).
- **A real thing to DO** (a launcher, a climb+prize, a maze, a dig/build, a bounce) — verified fun.
- **Custom assets** via `generate-with-refs` (posters/signs) — see mod-development skill.
- **Connect everything with paths**; light it; sign it.

## Don't
- Don't launch the visible client while she's nearby. Don't poll for the build by spamming short
  sleeps — wait on the `park built` / `vantage cycle start` log lines. Don't trust a single far orbit
  shot. Don't mark an activity "fun" without a functional log assertion.
