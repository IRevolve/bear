#!/bin/sh
# Exit 0 only for one known bot commit changing exactly the example lock file.
# Unknown history, mixed commits, and merge commits must run CI.
set -eu
base=${1:-}
bot_email=${2:-}
[ -n "$base" ] && [ -n "$bot_email" ] || exit 1
git rev-parse --verify "${base}^{commit}" >/dev/null 2>&1 || exit 1
[ "$(git rev-list --count "$base..HEAD")" = 1 ] || exit 1
[ "$(git rev-list --parents -n 1 HEAD | wc -w | tr -d ' ')" = 2 ] || exit 1
[ "$(git log -1 --format=%ae)" = "$bot_email" ] || exit 1
[ "$(git log -1 --format=%ce)" = "$bot_email" ] || exit 1
[ "$(git log -1 --format=%s)" = 'chore(bear): update lock file [skip ci]' ] || exit 1
[ "$(git diff --name-only "$base" HEAD)" = 'examples/bear.lock.yml' ]
