#!/bin/sh
# Installs the Aeolus agent on an OpenWrt AP from the manager that served
# this script, and joins the AP to it (0083). On the AP:
#
#	wget -qO- --no-check-certificate https://<manager>:8443/install | sh
#
# With options, after "sh -s --":
#
#	-u <port>         the uplink, the port carrying the VLANs (else the one the default route leaves by)
#	-f <fingerprint>  the SHA-256 the manager's certificate must have (aeolus fingerprint, on the manager)
#	-n                install only: join a manager later, from LuCI's Aeolus page or with aeolus-enroll
#
# The manager's certificate is fetched first and its fingerprint printed;
# everything after is fetched over TLS pinned to it, and each file checked
# against the manifest's SHA-256. Where LuCI is installed, its Aeolus page
# comes too. Files a package owns (aeolus-agent, luci-app-aeolus) are left
# to it. Then aeolus-setup joins the AP to the manager, which installs the
# packages Aeolus needs through the manager's cache of OpenWrt's feeds
# (0069), and the AP waits in Landing Zone for a person to adopt it.
set -eu

MANAGER='@MANAGER@'

# Run from a pipe, sh reads the script as it goes, so all of it is read
# into main before any of it runs: nothing it starts can eat the rest.
main() {
	die() {
		echo "aeolus install: $*" >&2
		exit 1
	}

	fp= uplink= join=1
	while getopts f:u:n o; do
		case $o in
		f) fp=$OPTARG ;;
		u) uplink=$OPTARG ;;
		n) join= ;;
		*) die "options: -u <uplink port>, -f <fingerprint>, -n to install only" ;;
		esac
	done

	[ -f /etc/openwrt_release ] || die "this is not OpenWrt"
	[ "$(id -u)" = 0 ] || die "run it as root"
	command -v ucode >/dev/null 2>&1 || die "this OpenWrt has no ucode, which the agent is written in"
	# The agent's ucode modules: uclient and digest first come in 24.10's
	# feeds (0083). A snapshot has them.
	release=$(. /etc/openwrt_release && echo "${DISTRIB_RELEASE:-}")
	case $release in
	1[0-9].* | 2[0-3].*) die "Aeolus needs OpenWrt 24.10 or later; this is $release, whose feeds lack ucode modules the agent needs (uclient, digest)" ;;
	esac
	[ -z "$uplink" ] || [ -d "/sys/class/net/$uplink" ] || die "no port named $uplink"

	if command -v apk >/dev/null 2>&1; then
		owned() { apk info -e "$1" >/dev/null 2>&1; }
	else
		owned() { opkg status "$1" 2>/dev/null | grep -q '^Status:.* installed'; }
	fi

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT

	# The manager's certificate, which nothing vouches for yet.
	uclient-fetch -q -T 20 --no-check-certificate -O "$tmp/manager.crt" "$MANAGER/install/manager.crt" ||
		die "no certificate from $MANAGER"
	b64=$(sed -n '/-----BEGIN CERTIFICATE-----/,/-----END CERTIFICATE-----/{p;/-----END/q;}' "$tmp/manager.crt" | sed '/-----/d' | tr -d '\r\n')
	echo "$b64" | grep -Eq '^[A-Za-z0-9+/=]+$' || die "$MANAGER sent no certificate"
	have=$(ucode -e "print(b64dec('$b64'))" | sha256sum | cut -d' ' -f1)
	echo "The manager's certificate, SHA-256: $(echo "$have" | tr a-f A-F | sed 's/../&:/g; s/:$//')"
	if [ -n "$fp" ]; then
		want=$(echo "$fp" | tr -d ': ' | tr A-F a-f)
		[ "$have" = "$want" ] || die "that is not $fp: not trusted, nothing installed"
		echo "It matches the fingerprint given."
	fi

	fetch() {
		uclient-fetch -q -T 30 --ca-certificate="$tmp/manager.crt" -O "$2" "$MANAGER$1" ||
			die "could not fetch $1 from $MANAGER over TLS pinned to its certificate (does the certificate name the host you gave?)"
	}

	fetch /install/manifest "$tmp/manifest.json"
	# jshn reads variables it never set, so it runs without set -u.
	set +u
	. /usr/share/libubox/jshn.sh
	json_load "$(cat "$tmp/manifest.json")" || die "the manifest is not JSON"
	json_get_var version version

	# list <part>: "path mode sha256" for each of the manifest's <part> files
	list() {
		json_select "$1"
		json_select files
		i=1
		while json_is_a $i object; do
			json_select $i
			json_get_vars path mode sha256
			echo "$path $mode $sha256"
			json_select ..
			i=$((i + 1))
		done
		json_select ..
		json_select ..
	}

	parts=
	owned aeolus-agent || parts=agent
	[ -d /www/luci-static/resources ] && ! owned luci-app-aeolus && parts="$parts luci"

	: >"$tmp/files"
	for part in $parts; do
		list $part >>"$tmp/files"
	done

	# Every file is fetched and checked before any is put in place.
	mkdir "$tmp/stage"
	while read -r p m sha; do
		case $p in
		/usr/sbin/aeolus-* | /usr/libexec/aeolus-* | /usr/share/ucode/aeolus/* | /etc/init.d/aeolus | /etc/hotplug.d/*/*aeolus* | /lib/upgrade/keep.d/*aeolus*) ;;
		/www/luci-static/resources/view/aeolus/* | /usr/share/luci/menu.d/luci-app-aeolus.json | /usr/share/rpcd/acl.d/luci-app-aeolus.json) ;;
		*) die "the manifest names $p, not a place of Aeolus's" ;;
		esac
		case $p in *..*) die "the manifest names $p" ;; esac
		echo "$sha" | grep -Eq '^[0-9a-f]{64}$' || die "the manifest's SHA-256 for $p is $sha"
		case $m in 0755 | 0644) ;; *) die "the manifest gives $p the mode $m" ;; esac
		fetch "/install/files/$sha" "$tmp/stage/$sha"
		[ "$(sha256sum "$tmp/stage/$sha" | cut -d' ' -f1)" = "$sha" ] || die "$p is not what the manifest says"
	done <"$tmp/files"

	while read -r p m sha; do
		mkdir -p "${p%/*}"
		cp "$tmp/stage/$sha" "$p.aeolus-new"
		chmod "$m" "$p.aeolus-new"
		mv "$p.aeolus-new" "$p"
	done <"$tmp/files"

	case " $parts " in
	*" agent "*) echo "Installed the Aeolus agent, $version." ;;
	*) echo "The aeolus-agent package is installed; its files are left to it, and the agent updates itself from the manager." ;;
	esac
	case " $parts " in *" luci "*)
		rm -rf /tmp/luci-indexcache* /tmp/luci-modulecache
		/etc/init.d/rpcd reload >/dev/null 2>&1 || true
		echo "LuCI has an Aeolus page, under Services."
		;;
	esac

	if [ -z "$join" ]; then
		echo "Join a manager from LuCI's Aeolus page, or: aeolus-enroll enroll <manager> <fingerprint> [uplink]"
		exit 0
	fi
	if [ -z "$uplink" ]; then
		uplink=$(/usr/libexec/aeolus-enroll status | jsonfilter -e '@.guess')
		[ -n "$uplink" ] || {
			echo "No one uplink port found: join from LuCI's Aeolus page, or name the port:"
			echo "  aeolus-enroll enroll $MANAGER $have <port>"
			exit 1
		}
	fi
	/usr/libexec/aeolus-enroll enroll "$MANAGER" "$have" "$uplink" </dev/null
}

main "$@"
