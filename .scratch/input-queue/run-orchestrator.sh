#!/usr/bin/env bash
cd /home/cioky/Projects/wick-simple || exit 1
exec wick-pig -role full "$(cat .scratch/input-queue/orchestrate-prompt.md)"
