#!/bin/sh
# PROTOTYPE (Bulma look): serve a copy of your Loom DB on :7338 with the
# variant switcher. Flip looks with the floating bar or the ← → keys.
set -e
home=${TMPDIR:-/tmp}/loom-prototype-bulma
mkdir -p "$home" && rm -f "$home"/loom.db*
sqlite3 "${LOOM_HOME:-$HOME/.loom}/loom.db" ".backup $home/loom.db"
printf 'port = 7338\n' > "$home/config.toml"
echo "open http://127.0.0.1:7338/loom?variant=a"
LOOM_HOME="$home" exec go run . serve
