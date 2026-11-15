#!/bin/sh
# Mirror GitLab branches and tags to GitHub from SERVER 1.
#
# Required configuration (/etc/hackathon/mirror.env, mode 0600):
#   GITLAB_REPOSITORY   ssh://git@10.77.0.1:2222/hackathon/hackathon.git
#   GITLAB_SSH_KEY      /etc/hackathon/mirror/id_ed25519
#   GITHUB_REPOSITORY   niktar411/hakaton-14-11-2026
#   GITHUB_MIRROR_TOKEN fine-grained token with "Contents: read and write"
#
# The token is handed to git through GIT_ASKPASS, never through argv, so it
# does not appear in `ps` output or in the stored remote URL.
set -eu
umask 077

: "${GITLAB_REPOSITORY:?set GITLAB_REPOSITORY}"
: "${GITLAB_SSH_KEY:?set GITLAB_SSH_KEY}"
: "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
: "${GITHUB_MIRROR_TOKEN:?set GITHUB_MIRROR_TOKEN}"
MIRROR_ROOT=${MIRROR_ROOT:-/var/lib/hackathon-mirror}
ASKPASS=${GIT_ASKPASS:-/usr/local/bin/hackathon-git-askpass}
REPOSITORY=$MIRROR_ROOT/repository.git
LOCK=$MIRROR_ROOT/mirror.lock

case "$GITHUB_REPOSITORY" in
    */*) ;;
    *) echo "GITHUB_REPOSITORY must be owner/name" >&2; exit 1 ;;
esac
case "$GITHUB_REPOSITORY" in
    *[!A-Za-z0-9._/-]*) echo "GITHUB_REPOSITORY contains invalid characters" >&2; exit 1 ;;
esac
if [ "${#GITHUB_MIRROR_TOKEN}" -lt 20 ]; then
    echo "GITHUB_MIRROR_TOKEN looks truncated" >&2
    exit 1
fi
if [ ! -r "$GITLAB_SSH_KEY" ]; then
    echo "GITLAB_SSH_KEY is not readable" >&2
    exit 1
fi
if [ ! -x "$ASKPASS" ]; then
    echo "GIT_ASKPASS helper is missing" >&2
    exit 1
fi

mkdir -p "$MIRROR_ROOT"
chmod 0700 "$MIRROR_ROOT"

# One mirror run at a time; a slow GitHub push must not overlap the next timer.
exec 9>"$LOCK"
if ! flock -n 9; then
    echo "mirror is already running" >&2
    exit 0
fi

export GIT_TERMINAL_PROMPT=0
export GIT_ASKPASS="$ASKPASS"
export GIT_SSH_COMMAND="ssh -i $GITLAB_SSH_KEY -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -o BatchMode=yes"

if [ ! -d "$REPOSITORY" ]; then
    git clone --mirror "$GITLAB_REPOSITORY" "$REPOSITORY"
fi

cd "$REPOSITORY"
git remote set-url origin "$GITLAB_REPOSITORY"
git fetch --prune --tags origin '+refs/heads/*:refs/heads/*'
git push --prune --force "https://github.com/${GITHUB_REPOSITORY}.git" \
    'refs/heads/*:refs/heads/*' 'refs/tags/*:refs/tags/*'

echo "mirrored $(git rev-parse --short HEAD) to github.com/${GITHUB_REPOSITORY}"
