---
name: offscreen-iteration
description: Unattended visual iteration for GUI software — run the app on a virtual display the user never sees, capture screenshots/video the agent reads as images, drive scenarios via env-gated hooks, verify with greppable functional asserts, and gate quality with LLM-judge review loops. Use whenever you need to build/see/fix UI, game, or rendering work without a human watching the screen.
---

# Offscreen LLM iteration (unattended visual dev)

The problem: an agent can't see a window, can't reliably press keys, and every app launch costs
30–120s. Asking the user to "launch it and tell me what you see" caps iteration speed at human
patience. The fix is a harness where **the app renders on a display nobody sees, the agent
captures pixels and reads them as images, and scenarios drive themselves**.

## 1. The virtual display

Use the bundled launcher (`tools/` is relative to THIS skill's directory):

```bash
<skill-dir>/tools/launch-offscreen.sh -r 1280x720 -l /tmp/app.log -- ./gradlew runClient
```

It starts Xvfb if needed, scrubs `WAYLAND_DISPLAY` (the app must not grab the user's real
session), forces software GL (`llvmpipe` — no GPU needed), deletes the stale log, and launches
detached. Manual equivalent, if you need to deviate:

```bash
Xvfb :99 -screen 0 1280x720x24 &
DISPLAY=:99 WAYLAND_DISPLAY= LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe \
  <app> > /tmp/app.log 2>&1 &
```

- Capture stills: `import -window root /tmp/shot.png` (ImageMagick), then `Read` the PNG —
  or use the bundled gated series capturer (section 3).
- Capture video: `ffmpeg -f x11grab -i :99 …` → mp4/GIF for humans to review asynchronously.
- Side effect worth knowing: nothing ever appears on the user's screen — useful when the work
  itself is a surprise (gift, demo, unannounced feature).

## 2. Drive the app WITHOUT input injection

Keyboard/mouse injection into a headless app is flaky (focus, timing, no compositor). Invert it:
**teach the app to drive itself** via env-gated hooks compiled into dev builds:

- `APP_DEMO=1` → run a scripted scenario on boot.
- `APP_CAM="x,y,z,yaw,pitch;…"` → cycle camera/viewport vantages on a timer.
- `APP_SCENARIO=foo` → put the program into the exact state you need photographed.

Hooks are ~10 lines each inside the app and turn "simulate a user session" into "set an env var".
When real input is unavoidable (driving a session on the user's actual screen), inject at the
kernel level, not the display server — bundled in this skill:

```bash
cd <skill-dir>/tools/keysend && go build -o keysend .     # once; no dependencies
sudo ./keysend tap:t type:"/teleport 0 80 0" tap:enter    # /dev/uinput needs root or input group
```

It creates a virtual kernel keyboard, so it works on Wayland/sway where xdotool cannot.

## 3. Sequencing without sleeps

Polling `sleep N` race-fails constantly. Gate every phase on the app's **log**. The bundled
capturer does gate + settle + series in one call:

```bash
<skill-dir>/tools/capture.sh -l /tmp/app.log -g "world loaded" -n 12 -i 5 -o /tmp/review
# → waits for the log line, settles, grabs /tmp/review_01.png … _12.png; Read them as images
```

Manual gate, when you need custom phasing:

```bash
until grep -q "world loaded" /tmp/app.log; do sleep 3; done   # then capture
```

Pitfalls that will bite you exactly once each:
- `rm` the log before every launch — a stale log from the previous run satisfies your grep.
- Kill leftover app instances first (file locks, port binds, half-dead windows).
- Cap the app's memory; N parallel headless instances + software GL summon the OOM killer.

## 4. Functional asserts (CI-grade "it works")

Visual review can't prove behavior. Add env-gated **assert hooks** in the app: a flag like
`APP_TEST_CHECKOUT=1` drives the real feature end-to-end in-process and emits ONE greppable line:

```
MYAPP CHECKOUT-TEST: cartTotal=ok paymentFlow=ok receipt=ok -> PASS
```

Rules: position/state-based assertions over flaky proxies; assert the OUTCOME the user cares
about (full health, money paid, file written), not "reached step 3"; one line per test, grep for
`-> FAIL`. These run headless in the same harness — they are your integration CI.

## 5. LLM-judge review loops (iterate to a score)

Pixels in hand, close the quality loop without the user:

1. Capture a representative set (overview + close-ups + the user's literal POV).
2. Spawn a judge subagent with a **persona + rubric** and the image paths; it `Read`s them.
3. Demand a structured verdict: `SCORE: n/10`, `BLOCKERS: (each with a concrete fix)`.
4. Apply blockers, re-capture, re-judge. Terminate at the target score — never on "looks fine".

What makes judges effective:
- **Personas target failure modes**: an end-user judge (a child, a novice) catches readability
  and frustration; a domain-expert judge (an engineer, a pilot) catches accuracy. Same images,
  different blockers.
- Judges with file-system access can verify claims in code, not just pixels — let them.
- Feed judges the REAL artifact (screenshots from the actual renderer), never mockups; offline
  previews and in-engine truth diverge (lighting, orientation, scale).
- Keep verdict format fixed so the loop is mechanical: blockers in → edits out → re-score.

## 6. The cost model (why this is fast)

A launch is the expensive unit (~1 min). Spend it well:
- Iterate **logic** in unit tests (ms) and **assets** in offline previewers (~1s) — a launch is
  only for integration truth.
- Batch: one launch serves many vantages (camera-cycling hook) + several asserts.
- Run launches in the background; do other work; gate on log lines, not waiting.

Worked example: the [the sibling skills](../README.md) skills used this harness to ship a
complete game mod — features verified by asserts, reviewed by two judge personas to 10/10 —
with the user only ever seeing finished results.
