#!/bin/sh
# GIT_ASKPASS helper for the GitHub mirror. Reads the token from the
# environment so it never lands in a command line or a remote URL.
set -eu
case "$1" in
    Username*|username*) printf '%s\n' "${GITHUB_MIRROR_USER:-x-access-token}" ;;
    Password*|password*) printf '%s\n' "${GITHUB_MIRROR_TOKEN:?missing token}" ;;
    *) exit 1 ;;
esac
