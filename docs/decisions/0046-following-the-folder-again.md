# 0046. Following the folder again

- Status: Proposed
- Date: 2026-10-02
- Proposed by: Griff, after customizing OpenWrtnight's width: "we need a way to revert back to the upper folder's configuration after customizing the AP individually"; written up by Claude
- Refines: 0012, 0026, 0042, 0044

## Decision

- **Every value a node sets for itself can follow the folder above again.**
  - In the UI, each value marked "Set here" has a **Follow ‹folder›** button.
  - At the root, where nothing is above, the button is **Stop setting it here**.
  - Following unsets the value, so the node inherits it again (0012).
- **An AP can revert to its folder in one step.**
  - Its page offers **Revert to ‹folder›**, which unsets everything the AP sets for itself.
  - Folders get no such button: a folder's settings reach every AP below it, so they are reverted one value at a time.
- **In the Hardware panel, a band follows its folder as one setting.** If the channel is set on the same node as the width, it goes back too. That way a width set with an automatic channel (0045) leaves as it came.
- **One change can unset several fields of one node.**
  - An unset takes either one `path` or several `paths`.
  - Every path must be set at the node, or none is unset.
  - It is one log entry with one reason (0026), and the APs get one new config version.
- **A preview says what each field becomes.**
  - The preview returns `resolved`: each field the change touches, as it would resolve at the node afterwards, with its value and where it comes from.
  - A field nothing would set any more is null.
  - The UI shows each field's value now, what it becomes, and where that comes from, before asking for a reason.

## Consequences

- Undoing a custom setting no longer means knowing which field to unset, or using the API.
- Locks and breaks are unchanged. A value locked here is unlocked with unlock, not followed away (0005). A break's baseline is the branch's own, so it is not offered (0005).
