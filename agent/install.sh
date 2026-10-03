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
# Aeolus turns it on for a network; snmpd, for SNMP (0052), off until Aeolus
# turns it on; vxlan and kmod-nft-bridge, for VXLAN tunnels and their MSS
# clamp (0054); and ucode-mod-socket, for the prober, which probes the
# tunnels and guards tunnel ports against loops (0059). That needs the AP to
# reach OpenWrt's package feeds;
# without them, everything else works, and a setting that needs a missing
# package is refused until it is installed. The agent's files and settings
# are kept across a sysupgrade; the packages must be installed again.
set -eu

[ $# -eq 3 ] || { sed -n '2,10p' "$0" >&2; exit 2; }
url=$1 uplink=$2 cert=$3
here=$(dirname "$0")

[ -f "$cert" ] || { echo "no certificate at $cert" >&2; exit 1; }
[ -d "/sys/class/net/$uplink" ] || { echo "no port named $uplink" >&2; exit 1; }

cp -R "$here/files/." /
chmod 0755 /usr/sbin/aeolus-agent /usr/sbin/aeolus-prober /etc/init.d/aeolus
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

# SNMP (0052). snmpd starts when it is installed, answering OpenWrt's
# default communities (public, and private from the AP itself), so it is
# turned off and those communities deleted at once: Aeolus owns all of
# snmpd's config from here on. The SSL build is needed for v3's AES.
if apk info -e snmpd-ssl >/dev/null 2>&1 || { apk update >/dev/null && apk add snmpd-ssl; }; then
	uci set snmpd.general.enabled=0
	for s in public private public6 private6; do uci -q delete snmpd.$s || true; done
	uci commit snmpd
	/etc/init.d/snmpd restart
else
	echo "snmpd could not be installed: SNMP will be refused on this AP until it is (apk add snmpd-ssl)" >&2
fi

# VXLAN tunnels (0054): netifd's vxlan handler, and the bridge family of
# nftables for the MSS clamp. Neither does anything until Aeolus renders a
# tunnel. netifd loads its protocols only when it starts, so installing vxlan
# means restarting the network, at the end (0057).
restart_network=0
for p in vxlan kmod-nft-bridge; do
	if apk info -e $p >/dev/null 2>&1; then
		continue
	fi
	if { apk update >/dev/null && apk add $p; }; then
		[ $p = vxlan ] && restart_network=1
	else
		echo "$p could not be installed: VXLAN tunnels will be refused on this AP until it is (apk add $p)" >&2
	fi
done

# The prober (0059) needs ucode's socket module. Without it the agent runs
# alone, and reports that its tunnels go unprobed.
if ! apk info -e ucode-mod-socket >/dev/null 2>&1 && ! { apk update >/dev/null && apk add ucode-mod-socket; }; then
	echo "ucode-mod-socket could not be installed: the tunnels go unprobed, and tunnel ports unguarded, until it is (apk add ucode-mod-socket)" >&2
fi

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
