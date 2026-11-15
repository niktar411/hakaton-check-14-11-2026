#!/bin/sh
# SERVER 2 firewall (nftables). No public ports. SSH and the control API are
# reachable only from SERVER 1 over the VPN; review links only over the VPN.
set -eu
: "${WAN_INTERFACE:?set WAN_INTERFACE}"
: "${SERVER1_PUBLIC_IP:?set SERVER1_PUBLIC_IP}"
: "${AWG_UDP_PORT:?set AWG_UDP_PORT}"
SSH_PORT=${SSH_PORT:-22}
case "$WAN_INTERFACE" in *[\ \;\|\&\`\$]*) exit 1 ;; esac
case "$SERVER1_PUBLIC_IP" in ""|*[!0-9.]*) exit 1 ;; esac
case "$AWG_UDP_PORT$SSH_PORT" in *[!0-9]*) exit 1 ;; esac
nft -f - <<EOF
destroy table inet hackathon
table inet hackathon {
    chain input {
        type filter hook input priority filter; policy drop;
        ct state established,related accept
        iifname "lo" accept
        # DHCP replies from the provider network (no other WAN ingress).
        iifname "$WAN_INTERFACE" udp sport 67 udp dport 68 accept
        # SSH and the worker API only from SERVER 1.
        iifname "awg0" ip saddr 10.77.0.1 ip daddr 10.77.0.10 tcp dport { $SSH_PORT, 9102 } accept
        # Review artifacts only over the team VPN.
        iifname "awg0" ip saddr 10.77.0.0/24 ip daddr 10.77.0.10 tcp dport 8080 accept
    }
    chain output {
        type filter hook output priority filter; policy drop;
        ct state established,related accept
        iifname "lo" accept
        # Egress only to SERVER 1 (VPN and its WireGuard endpoint).
        oifname "awg0" ip daddr 10.77.0.1 accept
        oifname "$WAN_INTERFACE" ip daddr $SERVER1_PUBLIC_IP udp dport $AWG_UDP_PORT accept
        oifname "$WAN_INTERFACE" udp sport 68 udp dport 67 accept
    }
    chain forward {
        type filter hook forward priority filter; policy drop;
        ct state established,related accept
        # Containers reach SERVER 1 (GitLab, DNS, package mirrors) only.
        iifname "br-*" oifname "awg0" ip daddr 10.77.0.1 accept
        # Check containers, temporary databases and review containers talk.
        iifname "br-*" oifname "br-*" accept
        # Review links are published only on the VPN address.
        iifname "awg0" oifname "br-*" ip saddr 10.77.0.0/24 tcp dport 8080 accept
    }
}
EOF
