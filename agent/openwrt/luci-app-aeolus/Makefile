# LuCI's Aeolus page as an OpenWrt package (0083): joins the AP to a
# manager named by hand, through the agent's aeolus-enroll. It holds exactly
# the files under agent/luci, the ones the manager's installer puts beside
# the agent where LuCI is installed. Build it with aeolus-agent, from the
# same feed (see its Makefile):
#
#	./scripts/feeds install luci-app-aeolus
#	make package/luci-app-aeolus/compile

include $(TOPDIR)/rules.mk

PKG_NAME:=luci-app-aeolus
AEOLUS_DIR:=$(abspath $(CURDIR)/../../..)
# The release it comes from, as aeolus-agent spells it.
PKG_VERSION:=$(shell git -C $(AEOLUS_DIR) describe --tags --match 'v[0-9]*' 2>/dev/null | \
	sed -e 's/^v//' -e 's/-\([0-9]*\)-g[0-9a-f]*$$/_p\1/' | grep . || echo 0.0.0)
PKG_RELEASE:=1

include $(INCLUDE_DIR)/package.mk

define Package/luci-app-aeolus
  SECTION:=luci
  CATEGORY:=LuCI
  SUBMENU:=3. Applications
  TITLE:=LuCI page for joining this AP to an Aeolus manager
  PKGARCH:=all
  DEPENDS:=+aeolus-agent +luci-base
endef

define Package/luci-app-aeolus/description
 An Aeolus page under Services: give the manager's DNS name or IP address,
 check its certificate's fingerprint, and join the AP to it, for an AP that
 found no manager by itself.
endef

define Build/Compile
endef

define Package/luci-app-aeolus/install
	$(CP) $(AEOLUS_DIR)/agent/luci/. $(1)/
	find $(1) -type d -exec chmod 0755 {} +
	find $(1) -type f -exec chmod 0644 {} +
endef

# LuCI caches its menu, and rpcd reads ACLs when it starts.
define Package/luci-app-aeolus/postinst
#!/bin/sh
[ -n "$${IPKG_INSTROOT}" ] && exit 0
rm -rf /tmp/luci-indexcache* /tmp/luci-modulecache
/etc/init.d/rpcd reload >/dev/null 2>&1
exit 0
endef

$(eval $(call BuildPackage,luci-app-aeolus))
