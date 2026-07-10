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
# `gh auth token -u niemst` itself raced once (2026-07-10: returned the other
# account's token mid-rewrite), so verify the token's identity and retry.
PUSH_TOKEN=""
for attempt in 1 2 3; do
    CANDIDATE=$(gh auth token -u niemst || true)
    LOGIN=$(curl -fsS -H "Authorization: Bearer ${CANDIDATE}" https://api.github.com/user \
        | python3 -c 'import json,sys; print(json.load(sys.stdin)["login"])' || true)
    if [ "$LOGIN" = "niemst" ]; then
        PUSH_TOKEN="$CANDIDATE"
        break
    fi
    echo "$LOG_PREFIX token identity check failed (got '${LOGIN:-none}', attempt $attempt) — retrying" >&2
    sleep 10
done
if [ -z "$PUSH_TOKEN" ]; then
    echo "$LOG_PREFIX no token verifying as niemst after 3 attempts — aborting push" >&2
    exit 75  # EX_TEMPFAIL: transient shared-gh-state race, next scheduled run retries
fi
git push "https://x-access-token:${PUSH_TOKEN}@github.com/niemst/stash.git" main --force-with-lease

echo "$LOG_PREFIX $(date -u +%Y-%m-%dT%H:%M:%SZ) synced $(git rev-parse --short HEAD)"
