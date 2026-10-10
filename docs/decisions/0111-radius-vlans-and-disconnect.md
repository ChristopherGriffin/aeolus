# 0111. VLANs from RADIUS, and disconnects from a NAC

- Status: Proposed
- Date: 2026-10-10
- Proposed by: Griff, "a radius server and maybe opennac to get this thing up to enterprise snuff". Written up by Claude
- Refines: 0070, 0082, 0098

## Context

- **WPA Enterprise (0098) was first tested end to end on 2026-10-10.** The test server was FreeRADIUS 3.2 on Zephyrus, with a lab CA, and the network was Aeolus-Enterprise on VLAN 20, on every lab AP. A client made on the C-360's spare radio joined:
  - by PEAP-MSCHAPv2 on the C-360 and on PumphouseAP;
  - by EAP-TLS on OfficeOpenWrt;
  - each time with a DHCP lease on VLAN 20, and accounting at the server.
- **A NAC decides where a client belongs, and the AP enforces it.** The NAC might be OpenNAC, PacketFence, ISE or ClearPass. The enforcement is standard RADIUS:
  - a VLAN in the Access-Accept (Tunnel-Private-Group-ID);
  - a Disconnect-Request (RFC 5176) when the NAC changes its mind, so the client signs in again and lands where it now belongs.
  - Aeolus did neither.
- **OpenWrt 25.12's wifi scripts already carry both** (ap.uc, read on OfficeOpenWrt):
  - `dynamic_vlan` on a WPA Enterprise wifi-iface, with the VLANs its wifi-vlan sections list as the allowed ones;
  - `radius_das_client` and `radius_das_secret`, with `radius_das_port`, 3799 unless set.
- **The full wpad has them** (0088). Per-user keys already put clients in VLANs the same way (0070), each on an AP/VLAN interface hostapd makes. A driver without such interfaces cannot do it (0082).

## Decision

- **A WPA Enterprise network may offer VLANs to its RADIUS server: `radius.vlans`.**
  - Each VLAN is carried as a per-user key's VLAN is (0070): tagged on the uplink, with an interface and a wifi-vlan on the network's Wi-Fi.
  - The wifi-vlan is named `r<vlan>`, so hostapd makes `<bss>-r<vlan>`. The prober reads a client's VLAN from that interface, as it does from `<bss>-k<vlan>`, and the Clients tab says the RADIUS server put it there.
  - A VLAN the server names that the network does not offer is refused.
  - `radius.vlan_required` refuses a client the server names no VLAN for (`dynamic_vlan=2`). Unset, such a client is on the network's own transport (`dynamic_vlan=1`).
  - A radio whose driver has no AP/VLAN interfaces is refused, as for keys (0082).
- **A WPA Enterprise network may let one server disconnect its clients: `radius.das`.**
  - `client` is the address the Disconnect-Requests come from. `secret` is that server's; unset, it is the sign-in one. `port` is where the AP listens, 3799 unless set.
  - The secret is sealed, as the others are (0027), and blanked in the UCI the AP sends back (0041).
- **Both are for WPA Enterprise alone.** On another network, compose refuses them, as nothing there signs in against the server. `radius.das` without a client is refused too. So is `radius.vlan_required` with no VLANs offered: nothing would be rendered, and every client would be let in after all.
- **The lab server is FreeRADIUS on Zephyrus.** Its config comes from a script, `/root/aeolus-radius/setup.sh`, which:
  - makes the lab CA and its certificates;
  - points EAP at them, PEAP unless the client asks for another;
  - copies PEAP's inner identity, and its inner reply, to the outer Access-Accept, so the APs account the user, not "anonymous", and a VLAN set for a user reaches the AP;
  - lists the APs' subnets as clients.
  - `/root/aeolus-radius/test.sh` tests PEAP and EAP-TLS with eapol_test.

## Consequences

- **A NAC can now steer clients by VLAN** (registration, quarantine, staff, guests) using the RADIUS every one of them speaks. Picking one, OpenNAC or PacketFence, is for when this has been tried against it.
- **Change of Authorization (CoA-Request), which changes a session without a disconnect, is not offered.** hostapd's DAS answers Disconnect-Requests, and a NAC that wants a change disconnects the client to apply it.
- **Still to come:**
  - MAC authentication for devices without 802.1X (`macaddr_acl=2` on an open or PSK network);
  - per-user keys looked up over RADIUS (0078);
  - 802.11r on WPA Enterprise (0098);
  - the user a client signed in as, shown on the Clients tab.
