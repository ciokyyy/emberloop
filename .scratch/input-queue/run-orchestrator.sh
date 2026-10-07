#!/usr/bin/env bash
cd /home/cioky/Projects/emberloop || exit 1
exec emberloop -role implement "$(cat .scratch/input-queue/orchestrate-prompt.md)"
