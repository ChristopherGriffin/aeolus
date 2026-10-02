#!/bin/sh
# aeolus on the manager host, installed as /usr/local/bin/aeolus by
# aeolus-update. Run as root, it runs the manager as the service's user, so
# any file it writes stays the service's (0043):
#
#	systemctl stop aeolus
#	aeolus token -account griff -revoke-others
#	systemctl start aeolus
if [ "$(id -u)" -eq 0 ]; then
	exec runuser -u aeolus -- /opt/aeolus/bin/aeolus "$@"
fi
exec /opt/aeolus/bin/aeolus "$@"
