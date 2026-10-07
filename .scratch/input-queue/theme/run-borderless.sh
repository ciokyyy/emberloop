#!/bin/bash
# $1 harness, $2 rule variant
export EMBERLOOP_SETTINGS=/tmp/inprobe/settings-off.json EMBERLOOP_RULE=${2:-soft}
pal=(45475a f38ba8 a6e3a1 f9e2af 89b4fa f5c2e7 94e2d5 bac2de 585b70 f38ba8 a6e3a1 f9e2af 89b4fa f5c2e7 94e2d5 a6adc8)
printf '\e]11;#1e1e2e\a\e]10;#cdd6f4\a\e]12;#f5e0dc\a'
for i in "${!pal[@]}"; do printf '\e]4;%d;#%s\a' "$i" "${pal[$i]}"; done
trap 'printf "\e]111\a\e]110\a\e]112\a\e]104\a"' EXIT
W=/tmp/inprobe/emberloop-proto
if [ "$1" = pig ]; then $W wrap --agent prevb-$1 --type orchestrator --name orchestrator --harness pig -- pig --theme /tmp/inprobe/pigthemeb --use-theme emberloopb
else $W wrap --agent prevb-$1 --type worker --name worker --harness claude -- claude --settings '{"theme":"custom:emberloopb"}'; fi
