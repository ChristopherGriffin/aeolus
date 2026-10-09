#!/bin/sh
# Installs the Aeolus agent on an OpenWrt AP by copying its files, for an AP
# without the aeolus-agent package (0040, 0080). Run it on the AP from the
# directory it was unpacked in:
#
#	sh install.sh <manager URL> <uplink port> <manager certificate>
#
# for example
#
#	sh install.sh https://192.168.20.60:8443 wan /tmp/manager.crt
#
# It puts the agent's files in place, inert, and hands over to aeolus-setup,
# which joins the AP to the manager and installs the packages Aeolus needs
# through the manager's cache of OpenWrt's feeds (0069). With the package
# installed (apk add aeolus-agent), run aeolus-setup with the same arguments
# instead. The agent's files and settings are kept over a sysupgrade, and
# after one the packages it needs are installed again from the manager.
set -eu

[ $# -eq 3 ] || { sed -n '2,10p' "$0" >&2; exit 2; }
here=$(dirname "$0")

cp -R "$here/files/." /
chmod 0755 /usr/sbin/aeolus-agent /usr/sbin/aeolus-prober /usr/sbin/aeolus-rrm /usr/sbin/aeolus-setup /usr/libexec/aeolus-enroll /usr/libexec/aeolus-gi /usr/libexec/aeolus-packages /usr/libexec/aeolus-rollback /etc/init.d/aeolus

exec /usr/sbin/aeolus-setup "$@"
