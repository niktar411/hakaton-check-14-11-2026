#!/bin/sh
set -eu
set +x
umask 077
: "${GITLAB_URL:?set GITLAB_URL}"
: "${GITLAB_RUNNER_TOKEN:?set GITLAB_RUNNER_TOKEN}"
: "${RUNNER_DEFAULT_IMAGE:?set RUNNER_DEFAULT_IMAGE}"
mkdir -p /builds /etc/gitlab-runner
chmod 0700 /root/.ssh /etc/gitlab-runner
if [ ! -s /etc/gitlab-runner/config.toml ]; then
    gitlab-runner register --non-interactive \
        --url "$GITLAB_URL" \
        --token "$GITLAB_RUNNER_TOKEN" \
        --name "server2-isolated-docker" \
        --executor docker \
        --docker-image "$RUNNER_DEFAULT_IMAGE" \
        --docker-pull-policy if-not-present \
        --docker-cap-drop ALL \
        --docker-security-opt no-new-privileges \
        --builds-dir /builds \
        --cache-dir /cache
fi
exec gitlab-runner run --working-directory /builds
