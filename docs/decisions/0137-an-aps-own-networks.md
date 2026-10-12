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
  - As each AP last reported: an AP reports every five minutes, and once its Wi-Fi is back after it takes one out.
  - One that carries another AP's uplink says so, there and where it is asked to be taken out: that AP would be cut off.
- **One with operator on an AP may have it take one out:** an action (0104), `remove-network`, asked of each AP that has the network, with its SSID and the sections it is, one a radio.
  - **This is the one way Aeolus removes what it did not make,** and only at a person's asking, of that AP, for that network. Every other time it leaves such a section as it is (0040).
  - The manager asks only for sections the AP last said it has, with that SSID. The AP takes out only a section that still has that SSID: a section with no name of its own is named for where it stands in its config, and another may stand there since.
  - **The AP keeps what it takes out, first,** as it was, key and all: a file a section in `/etc/aeolus/removed`, which only root reads, under a line that says what it is and how to have it back. Aeolus has no copy of the network or its passphrase and could not put it back. Nothing is taken out that cannot be kept.
  - The Wi-Fi then reloads. The network's clients are off it and cannot join it again; the others on its radios join again in a few seconds.
  - **The AP waits for its Wi-Fi to be back, as after an apply (0057), and then reports its state:** a report made while the radios reload has none of the AP's own BSSIDs in it, and its networks, heard by another AP, would be taken for a stranger's (0105). Where some do not come back, what the AP says came of it says how many.
  - It is asked before it is done, with how many clients are on the network, and logged with who asked, as every action is.
  - **Each network asked of an AP is an action of its own,** told from another by its sections: an AP's two networks may be taken out one after the other, without waiting for the first.
  - **What the AP says came of it has nothing of the SSID:** an SSID need not be plain words, and the manager takes no other for a result.

- **One of them may be made Aeolus's own: import.** On the same line as Remove, for one with operator on the AP who may change a Services folder that applies there.
  - **What the person says:** the Services folder the network goes in, its ID there, its bands, and its VLAN. The SSID, the security, the VLAN where the AP's config says it, and the bands it is on now are filled in from what the APs reported.
  - **What only the AP has, the passphrase above all, is read from the AP then:** an action, `import-network`, asked of the AP that has the most clients on the network. The AP answers with the section as it stands in its config, every option, and the manager makes the network of it at once: one change, under the name of whoever asked, with where it came from for its note.
  - **The passphrase is in the AP's answer and in the change, where it is sealed as any is (0027).** It is in no report, is not kept with the action, is not logged, and is never shown: the form says it is read, and nothing more.
  - **What has a place in Aeolus is carried:** SSID, security, passphrase, hidden, client isolation, fast transition, 802.11k and 802.11v, the clients kept off by MAC, the most clients, the DTIM period, multicast to unicast, whether it is on; with WPA Enterprise, the RADIUS servers, their ports and secrets, the NAS identifier and the disconnect client. An option that is on is on however UCI spells it. **What has none is named in what came of the action,** by its option's name alone, as many as there is room for and how many more: management frame protection set by hand, WPS, a value outside what Aeolus takes, and whatever else the section has.
  - **What cannot be Aeolus's is refused, and nothing made:** a security Aeolus has no name for, as WPA alone; a key of 64 digits, which is no passphrase; an ID a network has in that folder, above it or below it; a change that would have an AP's config held (0029).
  - **So is a network that does what Aeolus's would not, and that its clients, or another AP, may depend on:** it carries another AP's uplink (WDS); it lets on only the clients in a list, or asks a RADIUS server of each MAC its own way; its devices have passphrases of their own, by a file or from a server; it puts clients in VLANs by a file or as a server says. Aeolus's network would be offered without it, and the AP's own taken out. Said with why, and the network left as it is.
  - **The AP's answer is held to what was asked.** An AP's report is up to five minutes old, and a network may have changed since; and an AP is believed for its own network and no more. The answer must be of the section asked for, an access point's, and of the security the form showed: one asked as WPA2 and said as open makes nothing. The change is then tried again as it now is, the ID still free and no config held, before it is made.
  - **A network that waits to be imported is not asked another way:** a second asking into another folder, under another ID or on another VLAN is refused until the first is made or expires.
  - WPA2/WPA3 with WPA3 alone on 6 GHz is one network, as it is that by rule there (0086): the section off 6 GHz is the one asked for.
- **A network made so takes the place of the APs' own:** `replace_own`, a setting of the network, which import sets. It is a setting like any other, among a network's more settings, and may be set by hand for a network made by hand.
  - **On each AP that offers it, the AP's own network that is the same one over again is taken out as Aeolus's goes in:** the same SSID, the same security, and the same passphrase (with WPA Enterprise, the same RADIUS server), on radios Aeolus's is on. In the same apply, so the Wi-Fi restarts once, and an apply that is undone (0057) puts the AP's own back.
  - **The same passphrase, or it is another network.** An AP whose own network of that name has another passphrase keeps it, beside Aeolus's, and it stays in the list of what is not Aeolus's: its clients would be turned away by Aeolus's.
  - **And not one that does what Aeolus's would not:** a WDS uplink, a list of the only clients let on, passphrases or VLANs of its devices' own. It stays, whatever its name and key. The security is the same where the encryption is, and on a 6 GHz radio where the AP's own is WPA3 alone and Aeolus's WPA2/WPA3.
  - A section on two radios goes only where Aeolus's network is on both.
  - **The AP keeps what is taken out first,** as it does for Remove, and applies nothing where it cannot.
  - **This is the second way Aeolus removes what it did not make (0040),** and as narrow as the first: only for a network that says so, only a section that is that network, and never a station's, a mesh's, or one on a radio another service owns.
- **An AP's own network on bands Aeolus's is not on stays where Aeolus's is not.** One on 2.4 GHz alone, imported on 2.4 and 5 GHz, is on both as Aeolus's: the form says which APs gain a band.

## Consequences

- **A person sees every network their APs offer in one place,** whoever set it, and which are Aeolus's.
- **A network taken out is gone from Aeolus's sight at the AP's next report.** Its clients' history stays, by SSID.
- **An AP's passphrases do not leave it** by a report or a taking out: the report has none, and the file kept is on the AP. An import sends one network's, once, to the manager, at a person's asking, over the connection the AP's own config comes by.
- **The files an AP keeps go where its config goes:** `/etc/aeolus` is kept over a sysupgrade, and is in a backup of the AP's config, so the networks taken out are too, keys and all, until someone removes them from the AP. Nothing prunes them.
- **What an AP keeps of its config before each apply is root's alone to read** (`/etc/aeolus/backup`): it has the keys of the AP's own networks in it, and was written as any file is.

## Not done

- **An import is not seen before it is made:** the form shows what the APs reported, and what else the section has is read only when the change is made. What had no place is said after.
- **Per-device passphrases of an AP's own network (`wifi-station`) are not imported.**
- **The network an AP's own was on (its `interface`, its VLAN's bridge entry) is left,** though nothing may use it after.
- **A network on the AP's own LAN has no VLAN to fill in:** the person gives it.
- **Nothing says who set such a network, or when.**
- **A network on the AP's own LAN has no VLAN to show:** its config names none.
- **A network taken out is not put back where the Wi-Fi does not all come back after:** an apply is undone then (0057), but no config of Aeolus's changed here, and the AP only says so.
- **Changes someone made with `uci` and did not commit are committed with it,** where they are to the AP's Wi-Fi config.
- **Sections that go with one taken out are left:** a per-device passphrase of its own (`wifi-station`), or the network (`interface`) it was on, which may serve something else.
