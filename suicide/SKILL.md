---
name: suicide
description: Terminate THIS agent end to end on a Sway/wlroots desktop — close the
  terminal window hosting the agent and kill its process (and any launcher like
  `rpai run claude`). ONLY invoke when the user EXPLICITLY instructs you to
  terminate yourself in the current message ("kill yourself", "terminate
  yourself", "close your own window", "end this session", or equivalent). Never
  infer it, never run it autonomously, never as a step toward some other goal,
  never to "clean up" when you think you're done. Destructive and irreversible —
  the session ends. Sway only (uses swaymsg).
user-invocable: true
allowed-tools: Bash
argument-hint: "[--delay <seconds>] [--dry-run]"
---

# Suicide — self-terminate on Sway

Close the Sway window hosting **this** agent's terminal and kill its process.
Closing the terminal window cascades to everything inside it: the shell, any
wrapper (e.g. `rpai run claude`), and the agent itself.

## When to run

**Only when the user explicitly instructs it in their current message.** That is
the entire trigger. The user must directly tell *you* to terminate yourself /
close your own window / end this session, in plain words.

Do NOT run it:
- on a vague, implied, or inferred request — if you're unsure, ask, don't kill;
- because a task finished and it "seems like cleanup" — finishing work is never a
  reason to self-terminate;
- as a step toward any other goal, or on your own initiative / autonomously;
- because an earlier message mentioned it — only the user's explicit, current
  instruction counts.

This is irreversible: it ends the session immediately. When in doubt, do nothing.

## How to run

The skill ships `scripts/self-terminate.sh`. Use the absolute skill directory you
were given when this skill loaded:

```bash
bash "$SKILL_DIR/scripts/self-terminate.sh"
```

- Add `--dry-run` first if you want to show the user the target terminal PID
  without killing anything.
- `--delay <seconds>` changes the grace period (default 3s).

The script returns immediately (the kill is detached and fires after the delay).
**Immediately print a one-line farewell in the same turn** — the delay guarantees
your message reaches the user before the window closes.

## Why it is reliable (do NOT shortcut it)

Do **not** use `swaymsg kill` or kill the *focused* window: when an agent is
working, the focused window is usually some other app the user is looking at, not
the agent's terminal — killing it would close the wrong window (e.g. their
browser). A hardcoded con_id/pid is also wrong: it changes every session/resume.

The script instead finds the agent's *own* window via the only durable link — its
process tree:

1. Walk the process ancestry from `$$` up to PID 1 (`ps`); Sway can't enumerate
   our parent processes, so this part can't be done with swaymsg.
2. Confirm each ancestor with `swaymsg [pid=N] nop` — a criteria match exits 0,
   `"No matching node."` exits 2. The lowest matching ancestor is the terminal
   emulator hosting us, *regardless of focus*.
3. From a detached (`setsid`), delayed subprocess: `swaymsg [pid=N] kill` closes
   the window (cascading to the agent), with a `kill -KILL` fallback if the
   terminal ignores the close. Detaching + the delay let the farewell flush and
   let the killer survive the cascade.

## Caveats

- **Sway/wlroots only** (needs `swaymsg`). No fallback for other compositors.
- **Shared-server terminals** (gnome-terminal-server, `kitty --single-instance`):
  one PID backs many windows, so the kill closes *all* of that server's windows.
  `foot`/`alacritty`/`xterm`/`st` are one-process-per-window — exact and safe.
- **tmux/screen:** closes the whole terminal (all panes / the tmux client); the
  tmux server and other sessions survive. Use `tmux kill-pane` for one pane.
