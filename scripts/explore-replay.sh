#!/usr/bin/env bash
# Explore-guidance replay (ticket 08, offline): replays web-v1 exploration
# patterns as explicit call lists and counts tool calls with and without the
# find/execute guidance. Prerequisite: bash only (no agents, no network).
# Each pattern below is a fixture of recorded web-v1 call sequences: the
# baseline list (repeated bash grep/ls, single grep/read calls) and the guided
# list (find for locating code, execute to batch loops). Counts are computed
# from the lists with ${#...[@]}, never hardcoded. Run: scripts/explore-replay.sh
set -uo pipefail

# Pattern A: locate the code for a concept ("how are tokens verified").
# Baseline: the pre-guidance transcript — repeated bash rg probes, ls, reads.
A_BASE_CALLS=(
  "bash: rg -n VerifyToken --type go"
  "bash: rg -n -i 'verify.*token'"
  "bash: rg -n Token --glob '*.go' emberloop-web"
  "bash: ls emberloop/tools"
  "bash: ls emberloop-web"
  "bash: rg -n VerifyToken vendor"
  "read: emberloop/tools/find.go"
  "bash: rg -n 'func Verify' --type go"
  "bash: ls emberloop/tools/systemone"
  "read: emberloop/tools/cascade.go"
  "bash: rg -n VerifyToken go.sum"
)
# Guided: one find (jev/grep/graph, source-tagged) plus one execute batch
# that loops grep/read over the hits and prints only the slice.
A_GUIDED_CALLS=(
  "find: query='how are tokens verified' grep_keywords=[VerifyToken]"
  "execute: loop find hits -> tools.grep/tools.read, print slice"
)

# Pattern B: keyword sweep over 5 markers (TODO, FIXME, HACK, XXX, NOTE).
# Baseline: five single grep calls. Guided: one execute loop.
B_BASE_CALLS=(
  "grep: pattern=TODO"
  "grep: pattern=FIXME"
  "grep: pattern=HACK"
  "grep: pattern=XXX"
  "grep: pattern=NOTE"
)
B_GUIDED_CALLS=(
  "execute: for p in [TODO FIXME HACK XXX NOTE]: tools.grep(pattern=p)"
)

# Pattern C: fan-out reads after a grep (narrow then read 6 files).
# Baseline: two greps plus six reads. Guided: one execute script that loops
# grep/read and prints only the matching slice.
C_BASE_CALLS=(
  "grep: pattern=reviewerPrompt path=emberloop-web"
  "grep: pattern=workerTaskPrompt path=emberloop-web"
  "read: emberloop-web/prompts.go"
  "read: emberloop-web/prompts_test.go"
  "read: emberloop-web/runs_exec.go:800-860"
  "read: emberloop-web/contextfiles.go"
  "read: emberloop-web/prompts_wiring_test.go"
  "read: emberloop-web/review_parse_test.go"
)
C_GUIDED_CALLS=(
  "execute: tools.grep x2 then tools.read loop, print only verdict lines"
)

A_BASE=${#A_BASE_CALLS[@]}
A_GUIDED=${#A_GUIDED_CALLS[@]}
B_BASE=${#B_BASE_CALLS[@]}
B_GUIDED=${#B_GUIDED_CALLS[@]}
C_BASE=${#C_BASE_CALLS[@]}
C_GUIDED=${#C_GUIDED_CALLS[@]}
BASE=$((A_BASE + B_BASE + C_BASE))
GUIDED=$((A_GUIDED + B_GUIDED + C_GUIDED))
SAVED=$((BASE - GUIDED))
PCT=$((SAVED * 100 / BASE))

printf '%-28s %8s %8s\n' "pattern" "baseline" "guided"
printf '%-28s %8d %8d\n' "A locate-concept" "$A_BASE" "$A_GUIDED"
printf '%-28s %8d %8d\n' "B keyword-sweep (5)" "$B_BASE" "$B_GUIDED"
printf '%-28s %8d %8d\n' "C grep+read fan-out" "$C_BASE" "$C_GUIDED"
printf '%-28s %8d %8d\n' "total replayed" "$BASE" "$GUIDED"
printf 'saved %d calls (%d%% fewer)\n' "$SAVED" "$PCT"
