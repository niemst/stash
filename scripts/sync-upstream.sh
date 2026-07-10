#!/bin/bash
set -euo pipefail

REPO="$HOME/prv/repos/stash"
LOG_PREFIX="[stash-sync]"

cd "$REPO"

echo "$LOG_PREFIX $(date -u +%Y-%m-%dT%H:%M:%SZ) starting sync"

git fetch upstream
git fetch origin

LOCAL=$(git rev-parse HEAD)
UPSTREAM=$(git rev-parse upstream/main)

if [ "$LOCAL" = "$UPSTREAM" ]; then
    echo "$LOG_PREFIX already up to date"
    exit 0
fi

BRANCH=$(git rev-parse --abbrev-ref HEAD)

if [ "$BRANCH" != "main" ]; then
    echo "$LOG_PREFIX not on main (on $BRANCH), skipping auto-merge"
    exit 0
fi

# Rebase our changes on top of upstream (--autostash tolerates a dirty tree)
git rebase --autostash upstream/main

# Pin niemst's token on the push so a shared-gh-account race (another process
# flipping the active account) can't push as the wrong user and 403.
PUSH_TOKEN=$(gh auth token -u niemst)
git push "https://x-access-token:${PUSH_TOKEN}@github.com/niemst/stash.git" main --force-with-lease

echo "$LOG_PREFIX $(date -u +%Y-%m-%dT%H:%M:%SZ) synced $(git rev-parse --short HEAD)"
