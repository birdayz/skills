---
name: playtest-loop
description: Unattended visual iteration for Minecraft mod development — run the dev client/server on a virtual display the user never sees, capture screenshots/video the agent reads as images, drive the game via env-gated demo hooks in the mod, verify with greppable functional asserts, and gate quality with LLM-judge review loops. The offscreen foundation the mod-development and npc-design skills build on. Use whenever you need to build/see/fix mod UI, world, or rendering work without a human watching the screen.
---

# Offscreen Minecraft iteration (unattended visual mod-dev)

The problem: an agent can't see the Minecraft window, can't reliably press keys into it, and every
`runClient` costs ~60–120s. Asking the user to "launch it and tell me what you see" caps iteration
speed at human patience — and spoils a surprise build. The fix is a harness where **the game renders
on a display nobody sees, the agent captures pixels and reads them as images, and the mod drives
itself**. This is the base loop the `mod-development` and `npc-design` skills build on.

## 1. The virtual display

Use the bundled launcher (`tools/` is relative to THIS skill's directory):

```bash
<skill-dir>/tools/launch-offscreen.sh -r 1280x720 -l /tmp/mc.log -- ./gradlew :<mod>:runClient --console=plain
```

It starts Xvfb if needed, scrubs `WAYLAND_DISPLAY` (so GLFW can't grab the user's real session — it
physically cannot render to the real desktop), forces software GL (Mesa `llvmpipe` gives OpenGL 4.6,
no GPU needed), deletes the stale log, and launches detached. Manual equivalent, if you need to deviate:

```bash
Xvfb :99 -screen 0 1280x720x24 &
DISPLAY=:99 WAYLAND_DISPLAY= LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe \
  ./gradlew --no-daemon :<mod>:runClient > /tmp/mc.log 2>&1 &
```

- Capture stills: `DISPLAY=:99 import -window root /tmp/shot.png` (ImageMagick), then `Read` the PNG —
  or use the bundled gated series capturer (section 3).
- Capture video: `DISPLAY=:99 ffmpeg -f x11grab -i :99 …` → mp4/GIF for humans to review asynchronously.
- Nothing ever appears on the user's screen — essential when the mod is a gift/surprise; worst case is
  a crash, never a Minecraft window popping up where it can be seen.

## 2. Drive the game WITHOUT input injection

Keyboard/mouse injection into a headless client is hopeless (keysend/uinput goes to the real
compositor's focused window, not Xvfb; xdotool isn't installed). Invert it: **teach the mod to drive
itself** via env-gated hooks compiled into dev builds:

- `MYMOD_DEMO=1` → run a scripted scenario on boot (build the structure, seat the player, start the ride).
- `MYMOD_CAM="x,y,z,yaw,pitch;…"` → cycle camera vantages on a timer (teleport a SPECTATOR player).
- `MYMOD_SCENARIO=foo` → put the world into the exact state you need photographed.

Hooks are ~10 lines each inside the mod and turn "simulate a play session" into "set an env var".
When real input is unavoidable (driving a session on the user's ACTUAL screen, not Xvfb), inject at
the kernel level, not the display server — bundled in this skill:

```bash
cd <skill-dir>/tools/keysend && go build -o keysend .     # once; no dependencies
sudo ./keysend tap:slash type:"mymod build" tap:enter     # /dev/uinput; goes to the FOCUSED window
```

It creates a virtual kernel keyboard, so it works on Wayland/sway where xdotool cannot. (See the
`mod-development` skill's "Driving the game on Wayland/sway" for focus/screenshot details.)

## 3. Sequencing without sleeps

Polling `sleep N` race-fails constantly. Gate every phase on the MC **log**. The bundled capturer does
gate + settle + series in one call:

```bash
<skill-dir>/tools/capture.sh -l /tmp/mc.log -g "Sound engine started" -n 12 -i 5 -o /tmp/review
# → waits for the log line, settles, grabs /tmp/review_01.png … _12.png; Read them as images
```

Reliable MC log gates (dogfood-verified on 26.1.2): `GL info: llvmpipe … GL version 4.6` (software GL
up), `Backend library: LWJGL` (window created), **`Sound engine started`** (title screen reached).
For in-world captures, gate on your demo's OWN marker (e.g. `log("vantage cycle start")`), not a
generic line. Manual gate when you need custom phasing:

```bash
until grep -q "Riders ready" /tmp/mc.log; do sleep 3; done   # then capture
```

Pitfalls that will bite you exactly once each:
- `rm` the log before every launch — a stale log from the previous run (each `runClient` overwrites it)
  satisfies your grep and a dying instance's lines mislead. The launcher does this for you.
- Kill leftover game instances first (world file locks, port binds, half-dead windows). `pkill -f`
  often misses the long java cmdline — kill by PID from `ps -eo pid,cmd | grep -iE "DevLaunch|fml.modFold"`.
- Cap the client heap (`-Xmx4G`); N parallel headless clients + llvmpipe summon the OOM killer.

## 4. Functional asserts (CI-grade "it works")

Visual review can't prove behavior. Add env-gated **assert hooks** in the mod: a flag like
`MYMOD_TEST_RIDE=1` drives the real feature end-to-end in-process (server-side) and emits ONE greppable line:

```
MYMOD RIDE-TEST: rider=seated arrived=pool health=20 -> PASS
```

Rules: position/state-based assertions over flaky proxies; assert the OUTCOME the player cares about
(arrived at the pool at full health, money paid, structure built), not "reached step 3"; one line per
test, grep for `-> FAIL`. These run headless in the same harness — they are your gameplay integration CI.

## 5. LLM-judge review loops (iterate to a score)

Pixels in hand, close the quality loop without the user:

1. Capture a representative set (overview orbit + ground-tour close-ups + the player's literal POV).
2. Spawn a judge subagent with a **persona + rubric** and the image paths; it `Read`s them.
3. Demand a structured verdict: `SCORE: n/10`, `BLOCKERS: (each with a concrete fix)`.
4. Apply blockers, re-capture, re-judge. Terminate at the target score — never on "looks fine".

What makes judges effective:
- **Personas target failure modes**: a kid/end-user judge catches readability and frustration
  (unreadable signage, the last-collectible sadness spiral); a domain-expert judge catches accuracy
  (scored a Falcon 9 replica 8/10, demanded real stage separation before 10/10). Same images, different blockers.
- Feed judges the REAL artifact (screenshots from the actual renderer), never mockups — offline previews
  and in-engine truth diverge (lighting, orientation, scale). The software (llvmpipe) renderer also
  fogs/washes-out distant geometry, so get the camera CLOSE for color/detail judgments.
- Judges with file-system access can verify claims in code, not just pixels — let them.
- Keep the verdict format fixed so the loop is mechanical: blockers in → edits out → re-score.

## 6. The cost model (why this is fast)

A `runClient` is the expensive unit (~1 min). Spend it well:
- Iterate **logic** in JUnit (ms) and **assets** in offline previewers (~1s, see `npc-design`) — a
  launch is only for integration truth.
- Batch: one launch serves many vantages (camera-cycling demo hook) + several functional asserts.
- Run launches in the background; do other work; gate on log lines, not waiting.

Worked example: this harness shipped a complete Minecraft mod fully unattended — gameplay verified by
functional asserts, looks reviewed by two judge personas (a kid judge and a domain-accuracy judge)
iterated to 10/10 — with the user only ever seeing the finished gift.
