#!/bin/sh
# SERVER 2 firewall (ufw alternative). Mirrors deploy/nftables-worker.sh:
# no public ingress, SSH only from SERVER 1, egress only to SERVER 1.
set -eu
: "${WAN_INTERFACE:?set WAN_INTERFACE}"
: "${SERVER1_PUBLIC_IP:?set SERVER1_PUBLIC_IP}"
: "${AWG_UDP_PORT:?set AWG_UDP_PORT}"
SSH_PORT=${SSH_PORT:-22}
case "$SERVER1_PUBLIC_IP" in ""|*[!0-9.]*) exit 1 ;; esac
ufw --force reset
ufw default deny incoming
ufw default deny outgoing
ufw default deny routed
ufw allow in on awg0 proto tcp from 10.77.0.1 to 10.77.0.10 port "$SSH_PORT"
ufw allow in on awg0 proto tcp from 10.77.0.1 to 10.77.0.10 port 9102
ufw allow in on awg0 proto tcp from 10.77.0.0/24 to 10.77.0.10 port 8080
ufw allow out on awg0 to 10.77.0.1
ufw allow out on "$WAN_INTERFACE" proto udp to "$SERVER1_PUBLIC_IP" port "$AWG_UDP_PORT"
ufw allow out on "$WAN_INTERFACE" proto udp from any port 68 to any port 67
ufw allow in on "$WAN_INTERFACE" proto udp from any port 67 to any port 68
# Docker containers: internal traffic and egress to SERVER 1 only.
ufw route allow from 172.16.0.0/12 to 172.16.0.0/12
ufw route allow from 172.16.0.0/12 to 10.77.0.1
ufw route allow from 10.77.0.0/24 to 10.77.0.10 port 8080
ufw --force enable
