# The Aeolus agent as an OpenWrt package (0080). It holds exactly the files
# under agent/files, the same ones a release's bundle carries (0079), so a
# package and a bundle of one release are the same agent. The agent is pure
# ucode, so one package serves every target (PKGARCH:=all).
#
# Build it in an OpenWrt buildroot or SDK with this directory as a feed:
#
#	echo "src-link aeolus /path/to/aeolus/agent/openwrt" >> feeds.conf
#	./scripts/feeds update aeolus && ./scripts/feeds install aeolus-agent
#	make package/aeolus-agent/compile
#
# Installed, the agent is inert; aeolus-setup joins the AP to a manager.

include $(TOPDIR)/rules.mk

PKG_NAME:=aeolus-agent
AEOLUS_DIR:=$(abspath $(CURDIR)/../../..)
# The release it comes from, as apk spells a version: v0.49.2 is 0.49.2, and
# three commits past it is 0.49.2_p3.
PKG_VERSION:=$(shell git -C $(AEOLUS_DIR) describe --tags --match 'v[0-9]*' 2>/dev/null | \
	sed -e 's/^v//' -e 's/-\([0-9]*\)-g[0-9a-f]*$$/_p\1/' | grep . || echo 0.0.0)
PKG_RELEASE:=1

include $(INCLUDE_DIR)/package.mk

define Package/aeolus-agent
  SECTION:=net
  CATEGORY:=Network
  TITLE:=Aeolus agent: runs this AP from an Aeolus manager
  PKGARCH:=all
  DEPENDS:=+ucode +ucode-mod-fs +ucode-mod-uci +ucode-mod-ubus +ucode-mod-uloop \
	+ucode-mod-uclient +ucode-mod-math +ucode-mod-log +ucode-mod-digest
endef

define Package/aeolus-agent/description
 The Aeolus agent enrolls the AP with an Aeolus manager, polls it for its
 config, renders it into UCI, has the manager check the result, applies it
 with an automatic revert, and reports the AP's state. It is installed
 inert: run aeolus-setup <manager URL> <uplink port> <manager certificate>
 to join the AP to a manager. It updates itself from the manager afterwards.
endef

define Build/Compile
endef

# Modes are set here, not taken from the checkout: one on a Windows drive
# (WSL) shows every file as 0777. The executables are those 0079's bundle
# makes 0755.
define Package/aeolus-agent/install
	$(CP) $(AEOLUS_DIR)/agent/files/. $(1)/
	find $(1) -type d -exec chmod 0755 {} +
	find $(1) -type f -exec chmod 0644 {} +
	chmod 0755 $(1)/usr/sbin/aeolus-agent $(1)/usr/sbin/aeolus-prober \
		$(1)/usr/sbin/aeolus-rrm $(1)/usr/sbin/aeolus-setup \
		$(1)/usr/libexec/aeolus-packages $(1)/usr/libexec/aeolus-rollback \
		$(1)/etc/init.d/aeolus
endef

# Removal takes the manager's URL out, once the service has stopped, and
# keeps the rest. A package added again starts nothing until aeolus-setup,
# like a first install: OpenWrt starts a package's init script when it is
# installed, and the URL is what the init script starts the agent on. apk
# runs this on removal only, not on an upgrade; opkg says an upgrade with
# PKG_UPGRADE.
define Package/aeolus-agent/prerm
#!/bin/sh
[ -z "$${IPKG_INSTROOT}" ] && [ "$${PKG_UPGRADE}" != 1 ] || exit 0
if [ -n "$$(uci -q get aeolus.agent.url)" ]; then
	uci -q delete aeolus.agent.url
	uci commit aeolus
	echo "aeolus-agent: the manager's URL is cleared; aeolus-setup joins the AP to a manager again"
fi
exit 0
endef

$(eval $(call BuildPackage,aeolus-agent))
