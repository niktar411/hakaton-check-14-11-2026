#!/bin/sh
# Installs the OpenCode web UI as a separate container on the production host.
# Run as root from the repository root: sudo sh deploy/opencode/install.sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
target=/srv/opencode

mkdir -p "$target/config" "$target/data" "$target/workspace"

if [ ! -d "$target/workspace/.git" ]; then
  git clone --quiet "$repo_root" "$target/workspace"
  git -C "$target/workspace" remote remove origin
  git -C "$target/workspace" config user.name "OpenCode"
  git -C "$target/workspace" config user.email "opencode@localhost"
fi

cp "$repo_root/deploy/opencode/Dockerfile" "$target/Dockerfile"
cp "$repo_root/deploy/opencode/docker-compose.yml" "$target/docker-compose.yml"
cp "$repo_root/deploy/opencode/opencode.json" "$target/config/opencode.json"

if [ ! -f /etc/hackathon/opencode.env ]; then
  . /etc/hackathon/main.env
  test -n "${MODEL_API_KEY:-}"
  password=$(openssl rand -base64 24 | tr -d '\n')
  umask 077
  {
    printf 'OPENCODE_PASSWORD=%s\n' "$password"
    printf 'OPENCODE_SERVER_PASSWORD=%s\n' "$password"
    printf 'OPENCODE_API_KEY=%s\n' "$MODEL_API_KEY"
  } > /etc/hackathon/opencode.env
fi
chmod 600 /etc/hackathon/opencode.env

cd "$target"
docker compose up -d --build

if [ -f /etc/hackathon/opencode-users.txt ]; then
  sh "$repo_root/deploy/opencode/setup-auth.sh" /etc/hackathon/opencode-users.txt
fi

printf 'OpenCode web UI is starting on http://127.0.0.1:4096\n'
printf 'Internal password file: /etc/hackathon/opencode.env (0600)\n'
