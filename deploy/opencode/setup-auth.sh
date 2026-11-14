#!/bin/sh
# Front the OpenCode UI with nginx Basic auth for the team accounts.
# Input: a temporary file of "username:password" lines. It is deleted by the caller.
# Run as root: sudo sh deploy/opencode/setup-auth.sh /path/to/users.txt
set -eu

users_file=${1:-/etc/hackathon/opencode-users.txt}
test -f "$users_file"
. /etc/hackathon/opencode.env

: > /etc/hackathon/opencode.htpasswd
while IFS=: read -r user pass; do
  [ -n "$user" ] || continue
  hash=$(openssl passwd -apr1 "$pass")
  printf '%s:%s\n' "$user" "$hash" >> /etc/hackathon/opencode.htpasswd
done < "$users_file"
chmod 644 /etc/hackathon/opencode.htpasswd

internal_auth=$(printf 'opencode:%s' "$OPENCODE_PASSWORD" | base64 | tr -d '\n')
cat > /etc/hackathon/opencode-nginx-auth.conf <<EOF
auth_basic "OpenCode";
auth_basic_user_file /etc/nginx/opencode.htpasswd;
proxy_set_header Authorization "Basic $internal_auth";
EOF
chmod 644 /etc/hackathon/opencode-nginx-auth.conf

python3 - <<'PY'
from pathlib import Path
path = Path('/srv/hackathon/nginx.vhosts.conf.template')
text = path.read_text()
marker = 'server_name ai.cool-domain.online;'
include = '        include /etc/nginx/snippets/opencode-auth.conf;\n'
start = text.index(marker)
location = text.index('    location / {', start)
line = location + len('    location / {\n')
if include not in text[start:start + 2000]:
    text = text[:line] + include + text[line:]
path.write_text(text)
PY

if ! grep -q 'opencode.htpasswd' /srv/hackathon/docker-compose.override.yml; then
  sed -i '/nginx.vhosts.conf.template/a\      - /etc/hackathon/opencode.htpasswd:/etc/nginx/opencode.htpasswd:ro\n      - /etc/hackathon/opencode-nginx-auth.conf:/etc/nginx/snippets/opencode-auth.conf:ro' /srv/hackathon/docker-compose.override.yml
fi

cd /srv/hackathon
docker compose up -d --force-recreate web
