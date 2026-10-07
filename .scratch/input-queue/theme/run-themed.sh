#!/bin/bash
# Mocha palette via OSC; reset on exit
pal=(45475a f38ba8 a6e3a1 f9e2af 89b4fa f5c2e7 94e2d5 bac2de 585b70 f38ba8 a6e3a1 f9e2af 89b4fa f5c2e7 94e2d5 a6adc8)
printf '\e]11;#1e1e2e\a\e]10;#cdd6f4\a\e]12;#f5e0dc\a'
for i in "${!pal[@]}"; do printf '\e]4;%d;#%s\a' "$i" "${pal[$i]}"; done
reset(){ printf '\e]111\a\e]110\a\e]112\a\e]104\a'; }
trap reset EXIT
if [ "$1" = pig ]; then pig --theme /tmp/inprobe/pigtheme --use-theme emberloop
else claude --settings '{"theme":"custom:emberloop"}'; fi
