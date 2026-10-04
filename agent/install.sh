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
# It also installs the packages Aeolus needs (aeolus-packages says which),
# through the manager's cache of OpenWrt's feeds (0069): the AP's feeds are
# pointed at the manager, which fetches each package from OpenWrt once for
# all its APs. Without them, everything else works, and a setting that needs
# a missing package is refused until it is installed. The agent's files and
# settings are kept over a sysupgrade, and after one the agent installs the
# packages again from the manager.
set -eu

[ $# -eq 3 ] || { sed -n '2,10p' "$0" >&2; exit 2; }
url=$1 uplink=$2 cert=$3
here=$(dirname "$0")

[ -f "$cert" ] || { echo "no certificate at $cert" >&2; exit 1; }
[ -d "/sys/class/net/$uplink" ] || { echo "no port named $uplink" >&2; exit 1; }

cp -R "$here/files/." /
chmod 0755 /usr/sbin/aeolus-agent /usr/sbin/aeolus-prober /usr/libexec/aeolus-packages /etc/init.d/aeolus
mkdir -p /etc/aeolus
chmod 0700 /etc/aeolus
cp "$cert" /etc/aeolus/manager.crt

[ -f /etc/config/aeolus ] || touch /etc/config/aeolus
uci -q get aeolus.agent >/dev/null || uci set aeolus.agent=agent
uci set aeolus.agent.url="$url"
uci set aeolus.agent.uplink="$uplink"
uci commit aeolus

# The packages, through the manager (0069). Installing vxlan means
# restarting the network, at the end, so netifd loads it (0057).
restart_network=0
rc=0
/usr/libexec/aeolus-packages install || rc=$?
[ $rc = 3 ] && restart_network=1

/usr/sbin/aeolus-agent ping
/etc/init.d/aeolus enable
# Stopping a service that is not running yet only complains; say nothing.
/etc/init.d/aeolus stop 2>/dev/null || true
/etc/init.d/aeolus start
echo "The agent is running. Its log: logread -e aeolus"

if [ $restart_network = 1 ]; then
	echo "Restarting the network so netifd loads vxlan; this SSH session may drop for a few seconds."
	( trap '' HUP; sleep 2; /etc/init.d/network restart ) </dev/null >/dev/null 2>&1 &
fi
