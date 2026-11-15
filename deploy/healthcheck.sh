#!/bin/sh
set -eu
: "${PUBLIC_HEALTH_URL:?set PUBLIC_HEALTH_URL}"
: "${PLANNER_HEALTH_URL:?set PLANNER_HEALTH_URL}"
curl --fail --silent --show-error --max-time 5 --output /dev/null "$PUBLIC_HEALTH_URL"
curl --fail --silent --show-error --max-time 5 --output /dev/null "$PLANNER_HEALTH_URL"
