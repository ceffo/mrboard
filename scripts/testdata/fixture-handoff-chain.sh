#!/usr/bin/env bash
# Permanent test fixture for the terminal handoff: shaped like a real review launcher chain.
# Usage: fixture-handoff-chain.sh <root>
# It builds a local bare "origin" under <root>, then (as a wrapper script would) clones it behind a
# prompt read from /dev/tty, fetches, adds a worktree, resets it, and finally execs a full-screen
# child (the alt-screen fixture). All git traffic is local; no network, no ssh.
set -eu

root=$1
here=$(cd "$(dirname "$0")" && pwd)
origin="$root/origin.git"
repo="$root/checkout"
worktree="$root/worktrees/feature"
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t

if [ ! -d "$origin" ]; then
	git init -q --bare -b main "$origin"
	seed="$root/seed"
	git clone -q "$origin" "$seed" 2>/dev/null
	git -C "$seed" checkout -q -b feature
	echo hi >"$seed/f"
	git -C "$seed" add f
	git -C "$seed" commit -q -m init
	git -C "$seed" push -q origin feature
fi

if [ ! -d "$repo" ]; then
	printf 'No local checkout at %s - clone it? [y/N] ' "$repo" >&2
	read -r answer </dev/tty
	[ "$answer" = "y" ] || exit 1
	git clone -q "file://$origin" "$repo" >&2
fi

git -C "$repo" fetch origin feature >&2
git -C "$repo" worktree prune >&2
if [ -d "$worktree" ]; then
	git -C "$worktree" checkout feature >&2
	git -C "$worktree" reset --hard origin/feature >&2
else
	mkdir -p "$(dirname "$worktree")"
	git -C "$repo" worktree add -B feature "$worktree" origin/feature >&2
fi

cd "$worktree"
exec "$here/fixture-altscreen-command.sh" "$@"
