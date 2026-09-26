#!/bin/sh
# Build and install an Aeolus release on the manager host (0011).
#
# Run as root on the manager host:  update.sh [ref]
# ref defaults to the newest v* tag. The code is fetched with the host's
# read-only deploy key, built with the Go toolchain go.mod asks for, and
# installed; a running service is restarted. Only /var/lib/aeolus and
# /etc/aeolus hold state, so the host can always be rebuilt.
set -eu

SRC=/opt/aeolus/src
BIN=/opt/aeolus/bin
REPO=git@github.com:ChristopherGriffin/aeolus.git
export GIT_SSH_COMMAND="ssh -i /etc/aeolus/deploy_ed25519 -o IdentitiesOnly=yes -o UserKnownHostsFile=/etc/aeolus/github_known_hosts -o StrictHostKeyChecking=yes"

if [ ! -d "$SRC/.git" ]; then
	git clone -q "$REPO" "$SRC"
fi
git -C "$SRC" fetch -q --tags --prune origin

REF="${1:-$(git -C "$SRC" tag -l 'v*' --sort=-v:refname | head -n 1)}"
if [ -z "$REF" ]; then
	echo "update.sh: no release tag to install" >&2
	exit 1
fi
if git -C "$SRC" rev-parse -q --verify "refs/remotes/origin/$REF" >/dev/null; then
	REV="origin/$REF"
else
	REV="$REF"
fi
git -C "$SRC" -c advice.detachedHead=false checkout -q --detach "$REV"
COMMIT=$(git -C "$SRC" rev-parse --short HEAD)

cd "$SRC"
HOME=/root GOTOOLCHAIN=auto CGO_ENABLED=0 go build -trimpath \
	-ldflags "-s -w -X main.version=$REF-$COMMIT" -o "$BIN/aeolus.new" ./cmd/aeolus
"$BIN/aeolus.new" version
mv -f "$BIN/aeolus.new" "$BIN/aeolus"

install -m 0644 deploy/aeolus.service /etc/systemd/system/aeolus.service
if [ ! -f /etc/aeolus/serve.env ]; then
	printf 'AEOLUS_HOSTS=%s,%s\n' "$(hostname -f)" "$(hostname -I | awk '{print $1}')" >/etc/aeolus/serve.env
fi
systemctl daemon-reload
if systemctl is-active -q aeolus; then
	systemctl restart aeolus
fi
echo "installed $REF ($COMMIT)"
