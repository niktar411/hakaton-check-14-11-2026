#!/bin/sh
set -eu
set +x
umask 077

server_name() {
    url=$1
    case "$url" in
        https://*) authority=${url#https://} ;;
        *) echo "Public origins must use HTTPS" >&2; exit 1 ;;
    esac
    case "$authority" in
        ""|*/*|*\?*|*\#*|*@*) echo "Public origins must not contain credentials or paths" >&2; exit 1 ;;
    esac
    case "$authority" in
        *:*) name=${authority%%:*}; port=${authority##*:} ;;
        *) name=$authority; port=443 ;;
    esac
    case "$name" in ""|*[!A-Za-z0-9.-]*) echo "Public host is invalid" >&2; exit 1 ;; esac
    case "$port" in ""|*[!0-9]*) echo "Public port is invalid" >&2; exit 1 ;; esac
    if [ "$port" -lt 1 ] || [ "$port" -gt 65535 ]; then exit 1; fi
    printf '%s\n' "$name"
}

PUBLIC_SERVER_NAME=$(server_name "$PUBLIC_BASE_URL")
PLANNER_SERVER_NAME=$(server_name "$PLANNER_BASE_URL")
if [ "$PUBLIC_SERVER_NAME" = "$PLANNER_SERVER_NAME" ]; then
    echo "Public and planner hosts must differ" >&2
    exit 1
fi
export PUBLIC_SERVER_NAME PLANNER_SERVER_NAME
exec /docker-entrypoint.sh nginx -g 'daemon off;'
