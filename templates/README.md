# Ready-made AP templates

Each file here is an Aeolus AP template (decision 0090): the settings for one kind of AP, by the board OpenWrt names it. Aeolus doesn't create these by itself.

To use one:
1. Download the file (on GitHub, open it, then **Download raw file**).
2. In Aeolus, open the folder it should apply to, then **Templates › Import template…**.
3. Pick the file, check its settings, and **Review**.

It is made at that folder, offered there and below, and picked there for its boards unless you untick **Pick it here**. A template you've tuned in Aeolus downloads in the same format from its page in the Library (**Download**), ready to go back here.

| File | For | Holds |
|---|---|---|
| [arista-c360.json](arista-c360.json) | `arista,c360` | US; 6 GHz 160 MHz on preferred scanning channels, one a block; 5 GHz 80 MHz; 2.4 GHz 20 MHz; automatic channels and power; 802.11g and newer |

## The format

```json
{
	"aeolus_template": 1,
	"name": "Arista C-360",
	"about": "One line on what it is for (optional).",
	"boards": ["arista,c360"],
	"values": { "radio.6g.width": 160, "system.country": "US" }
}
```

`values` holds Locations fields by path, as the schema (`internal/schema/v1.json`) has them. A template sets radios, ports, system settings (not `system.agent`), radio resource management and power control. It never sets tunnels, names, service folders or secrets. CI imports every file here, so a file the schema would refuse fails the build.
