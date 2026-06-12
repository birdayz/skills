#!/usr/bin/env bash
# launch-offscreen.sh — run any GUI command on a hidden virtual display.
# Usage: launch-offscreen.sh [-d :99] [-r 1280x720] [-l /tmp/app.log] -- <command...>
# Starts Xvfb if needed, scrubs Wayland, forces software GL, launches detached.
set -euo pipefail
DISP=":99"; RES="1280x720"; LOG="/tmp/app.log"
while [ $# -gt 0 ]; do case "$1" in
  -d) DISP="$2"; shift 2;; -r) RES="$2"; shift 2;; -l) LOG="$2"; shift 2;;
  --) shift; break;; *) echo "unknown arg $1" >&2; exit 2;;
esac; done
[ $# -gt 0 ] || { echo "usage: $0 [-d :99] [-r WxH] [-l log] -- <command...>" >&2; exit 2; }
pgrep -f "Xvfb ${DISP}" >/dev/null || { Xvfb "$DISP" -screen 0 "${RES}x24" & sleep 1; }
rm -f "$LOG"                                  # stale-log rule: a previous run must not satisfy your greps
DISPLAY="$DISP" WAYLAND_DISPLAY= LIBGL_ALWAYS_SOFTWARE=1 GALLIUM_DRIVER=llvmpipe \
  nohup "$@" > "$LOG" 2>&1 &
echo "launched on $DISP, log: $LOG (pid $!)"
