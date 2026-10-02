#!/bin/sh
# Installs the Aeolus agent on an OpenWrt AP (0040). Run it on the AP from the
# directory it was unpacked in:
#
#	sh install.sh <manager URL> <uplink port> <manager certificate>
#
# for example
#
#	sh install.sh https://aeolus.symtus.com:8443 wan /tmp/manager.crt
#
# It needs nothing beyond OpenWrt's default image. Everything it adds is kept
# across a sysupgrade.
set -eu

[ $# -eq 3 ] || { sed -n '2,10p' "$0" >&2; exit 2; }
url=$1 uplink=$2 cert=$3
here=$(dirname "$0")

[ -f "$cert" ] || { echo "no certificate at $cert" >&2; exit 1; }
[ -d "/sys/class/net/$uplink" ] || { echo "no port named $uplink" >&2; exit 1; }

cp -R "$here/files/." /
chmod 0755 /usr/sbin/aeolus-agent /etc/init.d/aeolus
mkdir -p /etc/aeolus
chmod 0700 /etc/aeolus
cp "$cert" /etc/aeolus/manager.crt

[ -f /etc/config/aeolus ] || touch /etc/config/aeolus
uci -q get aeolus.agent >/dev/null || uci set aeolus.agent=agent
uci set aeolus.agent.url="$url"
uci set aeolus.agent.uplink="$uplink"
uci commit aeolus

/usr/sbin/aeolus-agent ping
/etc/init.d/aeolus enable
# Stopping a service that is not running yet only complains; say nothing.
/etc/init.d/aeolus stop 2>/dev/null || true
/etc/init.d/aeolus start
echo "The agent is running. Its log: logread -e aeolus"
