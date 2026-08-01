#!/usr/bin/env bash
# Records demo/mrboard.gif.
#
#   scripts/record-demo.sh            # from the working tree — for iterating
#   scripts/record-demo.sh v0.10.0    # from a clean checkout of a ref — for publishing
#
# Why the second form exists: the version string is baked in at build time by
# `git describe` and is visible in the recorded footer. Building from the working
# tree stamps `-dirty` the moment anything is uncommitted — including the very
# GIF this command rewrites — and a clean tree past a tag still reads
# `v0.10.0-2-gabc1234`. A bare released version is only reachable by building
# from a separate checkout that sits exactly on the tag, which is what the
# throwaway worktree below is for.
#
# The binary comes from the ref; the TAPE comes from the working tree, so you can
# iterate on the recording script without having to tag first. That means the GIF
# is not reproducible from the ref alone — commit the tape you used.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
ref="${1:-}"
ldpath="github.com/ceffo/mrboard/internal/cmd/mrboard.Version"

command -v vhs >/dev/null 2>&1 || {
	echo "record-demo: vhs is not installed (brew install vhs)" >&2
	exit 1
}

if [[ -z $ref ]]; then
	cd "$root"
	version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
	echo "record-demo: recording from the working tree (version: $version)"
	go build -ldflags "-X $ldpath=$version" -o ./bin/mrboard ./cmd/mrboard/...
	exec vhs demo/mrboard.tape
fi

git -C "$root" rev-parse --verify "$ref^{commit}" >/dev/null 2>&1 || {
	echo "record-demo: unknown ref '$ref'" >&2
	exit 1
}

wt="$(mktemp -d)/mrboard"
cleanup() { git -C "$root" worktree remove --force "$wt" >/dev/null 2>&1 || true; }
trap cleanup EXIT
git -C "$root" worktree add --detach "$wt" "$ref" >/dev/null

if [[ ! -x $wt/scripts/demo-tui.sh ]]; then
	echo "record-demo: '$ref' has no scripts/demo-tui.sh — it predates demo mode" >&2
	exit 1
fi

cd "$wt"

# Resolve the version BEFORE staging the tape. The stamp describes the source
# the binary is compiled from, and the tape is a driver script that contributes
# nothing to the binary — but it does make the worktree dirty, which would
# otherwise poison `git describe` here exactly as it does in the working tree.
version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
case $version in
*-dirty)
	echo "record-demo: worktree at '$ref' is unexpectedly dirty ($version); aborting" >&2
	exit 1
	;;
esac

# Use the tape being edited now, not the one frozen at the ref, so the recording
# script can be iterated on without tagging first.
cp "$root/demo/mrboard.tape" "$wt/demo/mrboard.tape"

echo "record-demo: recording from a clean checkout of $ref (version: $version)"
go build -ldflags "-X $ldpath=$version" -o ./bin/mrboard ./cmd/mrboard/...
vhs demo/mrboard.tape
cp "$wt/demo/mrboard.gif" "$root/demo/mrboard.gif"
echo "record-demo: wrote demo/mrboard.gif stamped $version"
