#!/bin/sh
# Copies the fixtures for a test run and builds a small git repository the
# Changes view can read.
set -eu
rm -rf .e2e-fixtures
cp -RL ../testdata/fixtures/desk .e2e-fixtures
repo=.e2e-fixtures/repo
mkdir -p "$repo"
g() { git -C "$repo" -c user.name=fixture -c user.email=fixture@example.com -c init.defaultBranch=main "$@"; }
g init -q
printf 'one\ntwo\nthree\n' > "$repo/notes.md"
g add notes.md
g commit -q -m base
printf 'one\nTWO\nthree\nfour\n' > "$repo/notes.md"
