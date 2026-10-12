// An AP's own Wi-Fi networks, as its agent says them (0137):
// TestAgentOthers runs this with the real ucode and compares what it
// prints with others.out.
//
//	ucode -L 'agent/files/usr/share/ucode/*.uc' agent/test/others.uc

'use strict';

import * as others from 'aeolus.others';

// A package as uci's get_all() gives it: sections by name, each with its
// type, name and place.
function pkg(list) {
	let out = {}, i = 0;
	for (let s in list) {
		let anon = s[1] == null;
		let name = anon ? sprintf('cfg%02d%04x', i + 1, i * 4369) : s[1];
		out[name] = { '.type': s[0], '.name': name, '.anonymous': anon, '.index': i++, ...s[2] };
	}
	return out;
}

// An AP with two radios and a scan radio of airscan's. Of its own: an IoT
// network on both bands, on VLAN 10; a trusted one on both, on VLAN 20,
// with fast transition and a list of clients kept off; a hidden one that
// is off; one another AP hangs off (WDS); one with no name of its own, on the AP's own LAN, on two radios
// at once; the station it reaches its uplink by, which is no network; one
// on the scan radio; and Aeolus's own.
let wireless = pkg([
	['wifi-device', 'radio0', { type: 'mac80211', band: '2g', channel: 'auto' }],
	['wifi-iface', 'default_radio0', { device: 'radio0', network: 'wifi_iot', mode: 'ap', ssid: 'Sweet_Spot_IoT', encryption: 'psk2', key: 'not-the-real-one', macfilter: 'deny', maclist: ['F0:AD:4E:2E:3A:1F'] }],
	['wifi-device', 'radio1', { type: 'mac80211', band: '5g', channel: 'auto' }],
	['wifi-iface', 'default_radio1', { device: 'radio1', network: 'wifi_trusted', mode: 'ap', ssid: 'Sweet Spot', encryption: 'psk2', key: "it's a secret", ieee80211r: '1', ft_over_ds: '0' }],
	['wifi-iface', 'wifinet2', { device: 'radio0', mode: 'ap', ssid: 'Sweet Spot', encryption: 'psk2', key: "it's a secret", network: 'wifi_trusted', ieee80211r: '1' }],
	['wifi-iface', 'wifinet3', { device: 'radio1', mode: 'ap', ssid: 'Sweet_Spot_IoT', encryption: 'psk2', key: 'not-the-real-one', network: 'wifi_iot' }],
	['wifi-iface', 'quiet', { device: 'radio1', ssid: 'Back room', encryption: 'sae-mixed', key: 'another', network: 'named', hidden: '1', disabled: '1' }],
	['wifi-iface', 'backhaul', { device: 'radio1', mode: 'ap', wds: '1', ssid: 'Far barn', encryption: 'psk2', key: 'a-third', network: 'lan' }],
	['wifi-iface', null, { device: ['radio0', 'radio1'], ssid: 'Open house', encryption: 'none', network: 'lan' }],
	['wifi-iface', 'uplink', { device: 'radio1', mode: 'sta', ssid: 'Upstream', encryption: 'psk2', key: 'x', network: 'wwan' }],
	['wifi-device', 'radio2', { type: 'mac80211', band: '5g', airscan: '1' }],
	['wifi-iface', 'default_radio2', { device: 'radio2', mode: 'ap', ssid: 'OpenWrt', encryption: 'none', network: 'lan', disabled: '1' }],
	['wifi-iface', 'aeolus_lab_radio0', { device: 'radio0', mode: 'ap', ssid: 'Aeolus Lab', encryption: 'psk2', key: 'k', network: 'aeolus_n1' }],
	['wifi-iface', 'old', { device: 'radio9', ssid: 'On a radio that is gone', encryption: 'none', network: 'lan' }],
]);
let network = pkg([
	['device', null, { name: 'br-lan', type: 'bridge', ports: ['eth0'] }],
	['interface', 'lan', { device: 'br-lan', proto: 'dhcp' }],
	['interface', 'wifi_iot', { device: 'br-lan.10', proto: 'dhcp' }],
	['interface', 'wifi_trusted', { device: 'br-lan.20', proto: 'none' }],
	['device', null, { name: 'vl30', type: '8021q', ifname: 'br-lan', vid: '30' }],
	['interface', 'named', { device: 'vl30', proto: 'none' }],
]);
let on = { 'default_radio0 radio0': 13, 'default_radio1 radio1': 1, 'wifinet2 radio0': 0 };
let list = others.list(wireless, network, (section, radio) => on[section + ' ' + radio]);
printf('%d of its own:\n', length(list));
for (let e in list)
	printf('  %J\n', e);
printf('nothing of a key in what is said: %J\n', index(sprintf('%J', list), 'secret') < 0 && index(sprintf('%J', list), 'not-the-real') < 0);
printf('with nothing to count its clients by: %J\n', map(others.list(wireless, network, null), e => e.clients));
printf('an AP with no Wi-Fi config: %J\n', others.list(null, null, null));

// Which section one is, where it is asked for: by its name, and the SSID
// it is to have. Not Aeolus's, not a station's, and not one whose SSID is
// another by now.
for (let ask in [['wifinet2', 'Sweet Spot'], ['wifinet2', 'Sweet_Spot_IoT'], ['aeolus_lab_radio0', 'Aeolus Lab'], ['uplink', 'Upstream'], ['radio0', ''], ['nosuch', 'x'], ['quiet', 'Back room']])
	printf('find %s, %J: %J\n', ask[0], ask[1], others.find(wireless, ask[0], ask[1])?.['.name'] ?? null);

// The radios a section is on, however its config names them.
printf('radios: %J %J %J\n', others.radios(others.find(wireless, 'wifinet2', 'Sweet Spot')), others.radios(filter(values(wireless), s => s.ssid == 'Open house')[0]), others.radios({ device: ' radio0  radio1 ' }));

// As it is kept before it is taken out: its options and its lists, a quote
// in a value as uci writes one, under a line that says what the file is.
print(others.kept(others.find(wireless, 'default_radio1', 'Sweet Spot'), '2026-10-11 19:40:00', 'Taken out of /etc/config/wireless by Aeolus, asked by griff'));
print(others.kept(others.find(wireless, 'default_radio0', 'Sweet_Spot_IoT'), '2026-10-11 19:40:00', 'Taken out'));
let anon = filter(values(wireless), s => s.ssid == 'Open house')[0];
print(others.kept(anon, '2026-10-11 19:40:00', 'Taken out'));

// All there is to say of one, where a person asks for it to be made
// Aeolus's own: every option as it stands, its key among them, and its
// VLAN. Nothing of uci's own, the names that begin with a dot.
printf('told: %J\n', others.told(wireless, network, others.find(wireless, 'default_radio1', 'Sweet Spot')));
printf('told, of one with a list: %J\n', others.told(wireless, network, others.find(wireless, 'default_radio0', 'Sweet_Spot_IoT')));
// And how many sections hang on it: passphrases or VLANs of its devices'
// own, which are no part of it. Aeolus's own keys, on Aeolus's networks,
// are not counted.
let hung = { ...wireless,
	st1: { '.type': 'wifi-station', '.name': 'st1', '.index': 50, iface: 'default_radio0', mac: '02:00:00:00:00:09', key: 'a-device-key' },
	vl1: { '.type': 'wifi-vlan', '.name': 'vl1', '.index': 51, iface: 'default_radio0', vid: '30', network: 'named' },
	st2: { '.type': 'wifi-station', '.name': 'st2', '.index': 52, iface: 'wifinet2', key: 'another' },
	aeolus_key_1: { '.type': 'wifi-station', '.name': 'aeolus_key_1', '.index': 53, iface: 'default_radio0', key: 'aeolus-one' },
};
printf('told, with sections that hang on it: %J\n', others.told(hung, network, others.find(hung, 'default_radio0', 'Sweet_Spot_IoT')).stations);
// A WDS network says so, spelled 1 or true.
printf('wds, spelled true: %J\n', filter(others.list(pkg([['wifi-device', 'radio0', { band: '2g' }], ['wifi-iface', 'far', { device: 'radio0', ssid: 'Far', wds: 'true' }], ['wifi-iface', 'near', { device: 'radio0', ssid: 'Near', wds: 'on' }]]), {}, null), e => e.wds)[0]?.ssid);

// So many are said of an AP, and no more.
let many = [['wifi-device', 'radio0', { band: '2g' }]];
for (let i = 0; i < 40; i++)
	push(many, ['wifi-iface', 'n' + i, { device: 'radio0', ssid: 'Net ' + i, encryption: 'none' }]);
printf('of 40, said: %d\n', length(others.list(pkg(many), {}, null)));
