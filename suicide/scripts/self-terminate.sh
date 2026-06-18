#!/usr/bin/env bash
# self-terminate.sh  (swaymsg-native)
#
# Terminate the *current* agent end to end on a wlroots/Sway desktop: close the
# Sway window hosting this terminal, which cascades to kill everything inside it
# (the shell, any launcher like `rpai run claude`, and the agent process).
#
# Reliable because it DISCOVERS the agent's own window instead of relying on
# focus (the focused window is usually some other app the user is looking at)
# or a hardcoded id (changes every session). The only durable link between
# "this agent process" and "a Sway window" is the process tree:
#   1. Walk the process ancestry from $$ up to PID 1 (the one part Sway can't
#      do — it doesn't know our parent processes).
#   2. For each ancestor PID, probe `swaymsg [pid=N] nop`: a criteria match exits
#      0, "No matching node." exits 2. The lowest ancestor that matches is the
#      terminal emulator hosting us — regardless of focus.
#   3. From a DETACHED (setsid), DELAYED subprocess: `swaymsg [pid=N] kill` to
#      close the window (cascades to the agent), with a SIGKILL fallback if the
#      terminal ignores the close. Detaching + the delay let the agent print a
#      final message first and let the killer survive the cascade.
#
# Usage:  bash self-terminate.sh [--delay SECONDS] [--dry-run]
# Exit:   0 armed/dry-run, 1 could not locate the window, 2 bad args.

set -u

DELAY=3
DRY=0
while [ $# -gt 0 ]; do
  case "$1" in
    --delay) DELAY="${2:?}"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    -h|--help) sed -n '2,30p' "$0"; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

command -v swaymsg >/dev/null 2>&1 || { echo "self-terminate: swaymsg not found (not a Sway session?)" >&2; exit 1; }

# 1. Ancestry PIDs, closest-first.
ancestors=""
pid=$$
for _ in $(seq 1 64); do
  ppid=$(ps -o ppid= -p "$pid" 2>/dev/null | tr -d ' ')
  [ -z "$ppid" ] && break
  ancestors="$ancestors $pid"
  [ "$ppid" -le 1 ] && break
  pid="$ppid"
done

# 2. Lowest ancestor that owns a Sway window = our terminal. Ask sway directly.
TERM_PID=""
for p in $ancestors; do
  if swaymsg "[pid=$p]" nop >/dev/null 2>&1; then
    TERM_PID="$p"; break
  fi
done

if [ -z "$TERM_PID" ]; then
  echo "self-terminate: no ancestor PID owns a Sway window (ancestry:$ancestors)" >&2
  exit 1
fi

echo "self-terminate: terminal pid=$TERM_PID; window closes in ~${DELAY}s"
[ "$DRY" = 1 ] && { echo "self-terminate: --dry-run, nothing killed"; exit 0; }

# 3. Detached, delayed killer: sway closes the window (cascades to the agent);
# SIGKILL the terminal as a fallback if it ignores the close.
setsid sh -c "
  sleep $DELAY
  swaymsg '[pid=$TERM_PID]' kill 2>/dev/null
  sleep 2
  kill -KILL $TERM_PID 2>/dev/null
" </dev/null >/dev/null 2>&1 &
