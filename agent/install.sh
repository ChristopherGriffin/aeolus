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
# It also installs usteer, for band steering (0050), with steering off until
# Aeolus turns it on for a network. That needs the AP to reach OpenWrt's
# package feeds; without it, everything else works, and a network that asks
# for band steering is refused until usteer is installed. The agent's files
# and settings are kept across a sysupgrade; usteer must be installed again.
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

# Band steering (0050). Installing a package starts its service, and usteer's
# own default steers every SSID, so steering is turned off at once: Aeolus
# owns band_steering_interval and ssid_list from here on.
if apk info -e usteer >/dev/null 2>&1 || { apk update >/dev/null && apk add usteer; }; then
	uci set usteer.@usteer[0].band_steering_interval=0
	uci -q delete usteer.@usteer[0].ssid_list || true
	uci commit usteer
	/etc/init.d/usteer reload
else
	echo "usteer could not be installed: band steering will be refused on this AP until it is (apk add usteer)" >&2
fi

/usr/sbin/aeolus-agent ping
/etc/init.d/aeolus enable
# Stopping a service that is not running yet only complains; say nothing.
/etc/init.d/aeolus stop 2>/dev/null || true
/etc/init.d/aeolus start
echo "The agent is running. Its log: logread -e aeolus"
