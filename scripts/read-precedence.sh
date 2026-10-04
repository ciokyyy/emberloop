#!/usr/bin/env bash
# Which `read` runs when wick is loaded: ours, or pig's builtin?
# Runs the identical prompt twice — once plain pig, once with wick — and diffs
# what came back. The read outputs differ in shape, so the diff names the winner.
set -uo pipefail
export PATH="$HOME/go/bin:$PATH"
WICK=/home/cioky/Projects/wick-simple/wick
DIR=/tmp/wick-verify
FILE=$DIR/app.txt

rm -rf "$DIR"; mkdir -p "$DIR"
printf 'alpha\nbeta\ngamma\n' > "$FILE"
git -C "$DIR" init -q 2>/dev/null
git -C "$DIR" add -A >/dev/null 2>&1

PROMPT='Call the read tool with path "/tmp/wick-verify/app.txt". Then reply with the exact tool result verbatim between >>> and <<< markers, and nothing else.'

probe() {  # probe <label> <extra pig args...>
  local label=$1; shift
  ( cd "$DIR" && timeout 180 pig --no-skills "$@" -p "$PROMPT" 2>&1 ) \
    | python3 -c "
import sys,re
t=sys.stdin.read()
m=re.search(r'>>>(.*?)<<<', t, re.S)
print(m.group(1).strip() if m else t.strip()[-600:])
" | sed "s/^/[$label] /"
}

echo '=== builtin pig (no wick) ==='
probe builtin
echo
echo '=== wick loaded ==='
probe wick -e "$WICK"

herdr workspace list >/dev/null 2>&1
rm -rf "$DIR"