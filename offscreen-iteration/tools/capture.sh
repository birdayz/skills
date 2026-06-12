#!/usr/bin/env bash
# capture.sh — log-gated screenshot series from a virtual display.
# Usage: capture.sh [-d :99] [-l /tmp/app.log] [-g "ready pattern"] [-n 10] [-i 5] [-o /tmp/shot]
# Waits until the log matches -g (the no-sleeps rule), settles briefly, then grabs
# -n stills every -i seconds as <out>_<k>.png — Read them as images.
set -euo pipefail
DISP=":99"; LOG="/tmp/app.log"; GATE=""; N=10; IVL=5; OUT="/tmp/shot"
while getopts "d:l:g:n:i:o:" f; do case "$f" in
  d) DISP="$OPTARG";; l) LOG="$OPTARG";; g) GATE="$OPTARG";;
  n) N="$OPTARG";; i) IVL="$OPTARG";; o) OUT="$OPTARG";;
esac; done
if [ -n "$GATE" ]; then until grep -q "$GATE" "$LOG" 2>/dev/null; do sleep 3; done; sleep 8; fi
for k in $(seq -w 1 "$N"); do DISPLAY="$DISP" import -window root "${OUT}_${k}.png"; sleep "$IVL"; done
echo "captured $N frames: ${OUT}_*.png"
