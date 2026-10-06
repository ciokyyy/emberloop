#!/usr/bin/env bash
cd /home/cioky/Projects/wick-simple || exit 1
exec wick-pig -role implement "$(cat .scratch/context-compress/orchestrate-prompt.md)"
