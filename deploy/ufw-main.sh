#!/bin/sh
# SERVER 1 firewall (ufw alternative). Mirrors deploy/nftables-main.sh.
set -eu
: "${WAN_INTERFACE:?set WAN_INTERFACE}"
: "${ADMIN_SSH_CIDR:?set ADMIN_SSH_CIDR}"
: "${AWG_UDP_PORT:?set AWG_UDP_PORT}"
SSH_PORT=${SSH_PORT:-22}
MAIN_DOCKER_NETWORK=${MAIN_DOCKER_NETWORK:-172.30.0.0/24}
API_CONTAINER_IP=${API_CONTAINER_IP:-172.30.0.5}
ufw --force reset
ufw default deny incoming
ufw default allow outgoing
ufw default deny routed
ufw allow in on "$WAN_INTERFACE" proto tcp to any port 80,443
ufw allow in on "$WAN_INTERFACE" proto tcp from "$ADMIN_SSH_CIDR" to any port "$SSH_PORT"
ufw allow in on "$WAN_INTERFACE" proto udp to any port "$AWG_UDP_PORT"
ufw allow in on awg0 proto tcp from 10.77.0.2 to any port "$SSH_PORT"
ufw allow in on awg0 proto tcp from 10.77.0.0/24 to 10.77.0.1 port 8081,2222
ufw allow in on awg0 proto tcp from 10.77.0.10 to 10.77.0.1 port 9101
ufw allow in proto tcp from "$API_CONTAINER_IP" to 10.77.0.1 port 9101
# Routed traffic: VPN clients -> SERVER 2, Docker service-to-service,
# container egress and the published web ports after Docker DNAT.
ufw route allow from 10.77.0.0/24 to 10.77.0.10
ufw route allow from "$MAIN_DOCKER_NETWORK" to any
ufw route allow from "$MAIN_DOCKER_NETWORK" to "$MAIN_DOCKER_NETWORK"
ufw route allow proto tcp from 10.77.0.0/24 to any port 80
ufw route allow proto tcp from 10.77.0.0/24 to any port 443
ufw route allow proto tcp from 10.77.0.0/24 to any port 8081
ufw route allow proto tcp from 10.77.0.0/24 to any port 2222
ufw route allow proto tcp to any port 80
ufw route allow proto tcp to any port 443
ufw --force enable
