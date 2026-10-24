#!/bin/zsh
set -eu
cd -- "$(dirname -- "$0")"
exec python3 .agents/skills/proloader-maintenance/scripts/maintain.py --online "$@"
