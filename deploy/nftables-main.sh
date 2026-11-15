#!/bin/sh
# SERVER 1 firewall (nftables). Public: 80/443, admin SSH, WireGuard.
# The Docker bridge may reach only the control agent and outbound services.
set -eu
: "${WAN_INTERFACE:?set WAN_INTERFACE}"
: "${ADMIN_SSH_CIDR:?set ADMIN_SSH_CIDR}"
: "${AWG_UDP_PORT:?set AWG_UDP_PORT}"
SSH_PORT=${SSH_PORT:-22}
MAIN_DOCKER_NETWORK=${MAIN_DOCKER_NETWORK:-172.30.0.0/24}
API_CONTAINER_IP=${API_CONTAINER_IP:-172.30.0.5}
case "$WAN_INTERFACE$ADMIN_SSH_CIDR" in *[\ \;\|\&\`\$]*) exit 1 ;; esac
case "$AWG_UDP_PORT$SSH_PORT" in *[!0-9]*) exit 1 ;; esac
case "$MAIN_DOCKER_NETWORK$API_CONTAINER_IP" in *[!0-9./]*) exit 1 ;; esac
nft -f - <<EOF
destroy table inet hackathon
table inet hackathon {
    chain input {
        type filter hook input priority filter; policy drop;
        ct state established,related accept
        iifname "lo" accept
        iifname "$WAN_INTERFACE" tcp dport { 80, 443 } accept
        iifname "$WAN_INTERFACE" ip saddr $ADMIN_SSH_CIDR tcp dport $SSH_PORT accept
        iifname "$WAN_INTERFACE" udp dport $AWG_UDP_PORT accept
        iifname "awg0" ip saddr 10.77.0.2 tcp dport $SSH_PORT accept
        iifname "awg0" ip saddr 10.77.0.0/24 ip daddr 10.77.0.1 tcp dport { 8081, 2222 } accept
        iifname "awg0" ip saddr 10.77.0.10 ip daddr 10.77.0.1 tcp dport 9101 accept
        # The API container talks to the host control agent from a fixed IP.
        iifname "br-*" ip saddr $API_CONTAINER_IP ip daddr 10.77.0.1 tcp dport 9101 accept
    }
    chain forward {
        type filter hook forward priority filter; policy drop;
        ct state established,related accept
        # VPN clients reach SERVER 2 only; SERVER 2 never reaches the VPN
        # beyond its own established replies.
        iifname "awg0" oifname "awg0" ip saddr 10.77.0.0/24 ip daddr 10.77.0.10 \
            tcp dport { $SSH_PORT, 8080, 9102 } accept
        # VPN clients reach published containers (site and GitLab).
        iifname "awg0" oifname "br-*" tcp dport { 80, 443, 8081, 2222 } accept
        # Compose service-to-service traffic.
        iifname "br-*" oifname "br-*" accept
        # Orchestrator/GitLab runner -> SERVER 2 over the VPN.
        iifname "br-*" oifname "awg0" ip daddr 10.77.0.10 accept
        # Containers need the Cloud.ru model API and CI downloads.
        iifname "br-*" oifname "$WAN_INTERFACE" accept
        # Published web ports after Docker DNAT.
        iifname "$WAN_INTERFACE" oifname "br-*" tcp dport { 80, 443 } accept
    }
}
EOF
