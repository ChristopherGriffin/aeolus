# 0137. An AP's own networks

- Status: Proposed
- Date: 2026-10-11
- Proposed by: Griff. Two of his APs have networks on them that are no part of Aeolus: should Aeolus pull those in, read only, where the networks are shown, dull to say they are not Aeolus's, with a delete so Aeolus can take one out, and an import that makes one Aeolus's own. Shown from the folder, My House, and not an AP at a time. Written up by Claude
- Refines: 0040, 0104

## Context

- **Aeolus touches only what it owns (0040).** A Wi-Fi network someone set on an AP itself, in LuCI or by hand, runs beside Aeolus's and is never edited or removed. That is as it should be, and it left Aeolus saying nothing of such a network at all.
- **They showed all the same, as loose ends.** A client on one is a client of its AP, with an SSID and no network (0066). Each time it joins is recorded (0118), and its roams are counted (0136). Whoever looked for the network those belong to found none.
- **Griff's two APs each have two:** the house's own network, on both bands, with fast transition, on VLAN 20; and one for its devices, on VLAN 10, on both bands of the one AP and on 2.4 GHz of the other. Sixteen clients between them. Aeolus's networks are beside them, on other VLANs.
- **A network set so is the same network on every AP that has it only by its SSID.** Each AP has a section of its own config for it, one a radio, under whatever name it was given, or none.

## Decision

- **An AP says the Wi-Fi networks it has of its own,** in its state report (`others`): each wifi-iface of its config that is an access point's and is not Aeolus's (0039), a radio of it at a time.
  - What is said of one: the section it is, its radio and band, its SSID, its security as OpenWrt names the encryption, the VLAN it is on where its network's device says (a VLAN device, or one named as `br-lan.20`), whether it is hidden, off, offers fast transition, or carries another AP's uplink (WDS), and how many clients are on it now.
  - **Nothing of its key,** or of anything else that is a secret.
  - Not a station's section, a mesh's or a monitor's: none is a network clients join, and one may be what the AP itself is reached by. Nothing on a radio another service owns (0081).
  - 32 at the most. The manager keeps 64, and leaves out one that is not well said rather than refuse the report: what someone typed on an AP does not cost the AP its report.
- **A folder's Networks tab shows them under Aeolus's own,** and an AP's does: "On the APs, not Aeolus's".
  - A network a line, by its SSID, whatever APs and radios it is on, with its security, its VLAN, each AP and its bands, and its clients.
  - **Dull:** no card, a dashed edge, grey words. Nothing in it is Aeolus's to change, and nothing there can be edited.
  - As each AP last reported: an AP reports every five minutes, and at once after it takes one out.
  - One that carries another AP's uplink says so, there and where it is asked to be taken out: that AP would be cut off.
- **One with operator on an AP may have it take one out:** an action (0104), `remove-network`, asked of each AP that has the network, with its SSID and the sections it is, one a radio.
  - **This is the one way Aeolus removes what it did not make,** and only at a person's asking, of that AP, for that network. Every other time it leaves such a section as it is (0040).
  - The manager asks only for sections the AP last said it has, with that SSID. The AP takes out only a section that still has that SSID: a section with no name of its own is named for where it stands in its config, and another may stand there since.
  - **The AP keeps what it takes out, first,** as it was, key and all: a file a section in `/etc/aeolus/removed`, which only root reads, under a line that says what it is and how to have it back. Aeolus has no copy of the network or its passphrase and could not put it back. Nothing is taken out that cannot be kept.
  - The Wi-Fi then reloads. The network's clients are off it and cannot join it again; the others on its radios join again in a few seconds.
  - It is asked before it is done, with how many clients are on the network, and logged with who asked, as every action is.
  - **Each network asked of an AP is an action of its own,** told from another by its sections: an AP's two networks may be taken out one after the other, without waiting for the first.
  - **What the AP says came of it has nothing of the SSID:** an SSID need not be plain words, and the manager takes no other for a result.

## Consequences

- **A person sees every network their APs offer in one place,** whoever set it, and which are Aeolus's.
- **A network taken out is gone from Aeolus's sight at the AP's next report.** Its clients' history stays, by SSID.
- **An AP's passphrases do not leave it** by this: the report has none, and the file kept is on the AP.
- **The files an AP keeps go where its config goes:** `/etc/aeolus` is kept over a sysupgrade, and is in a backup of the AP's config, so the networks taken out are too, keys and all, until someone removes them from the AP. Nothing prunes them.
- **What an AP keeps of its config before each apply is root's alone to read** (`/etc/aeolus/backup`): it has the keys of the AP's own networks in it, and was written as any file is.

## Not done

- **Import:** making one of these Aeolus's own, with its settings. Next, and written here when it is built.
- **Nothing says who set such a network, or when.**
- **A network on the AP's own LAN has no VLAN to show:** its config names none.
- **Nothing checks that the Wi-Fi came back after a network is taken out,** as an apply does (0057): no config of Aeolus's changed, and there is nothing to go back to but the file kept.
- **Changes someone made with `uci` and did not commit are committed with it,** where they are to the AP's Wi-Fi config.
- **Sections that go with one taken out are left:** a per-device passphrase of its own (`wifi-station`), or the network (`interface`) it was on, which may serve something else.
