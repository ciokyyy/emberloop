#!/usr/bin/env bash
cd /home/cioky/Projects/el || exit 1
exec el -role implement "$(cat .scratch/archive/input-queue/orchestrate-prompt.md)"
