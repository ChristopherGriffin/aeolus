# 0048. Editing networks where they are shown

- Status: Accepted
- Date: 2026-10-02
- Proposed by: Griff, choosing to edit SSIDs in place on the Networks tab (0047); written up by Claude and accepted with the merge
- Refines: 0013, 0027, 0042, 0045, 0046, 0047

## Decision

- **Each network on a Locations folder's or AP's Networks tab has an Edit button.**
  - It opens a form with every field the schema has for a network.
  - The changes are made in the service folder the network comes from, in one change (0045). The Services tree stays, so one network can serve many locations (0013).
  - The preview lists each change, names the service folder it changes, and lists every AP that gets a new config.
  - A field locked above that service folder cannot be changed there.
- **A network's fallback may be None** (Griff, 2026-10-11). The fallback's type offered VLAN and VXLAN, and a dash only while the network had no fallback yet: one given could not be taken away. None is in the list always. Picked for a network that has a fallback, it unsets the fallback's settings in the folder that sets them, as a change of its own: fields are unset in one change and set in another, so what else the form would change is said, and left for a change after. A fallback that comes from a folder above is taken away there, and None says so.
- **A network can be added from the Networks tab.**
  - It goes into one of the service folders that apply there and that the person may change.
  - Its name comes from its SSID, made to fit the schema's pattern and unlike any network already here.
  - Only the fields the person fills in are set.
- **A network can be removed from where it is defined,** that is, from the service folder that sets its SSID. What that folder sets for it is unset in one change (0046), and the preview says that every location using the folder stops offering it.
- **The manager describes the fields it accepts:** `GET /v1/schema`.
  - It returns each settable field as JSON Schema, with the tree it is set in, plus the patterns names must match.
  - The UI builds its inputs from that, so a field added to the schema can be edited without new UI code:
    - a yes/no as a checkbox;
    - a choice as a menu;
    - a list of choices as checkboxes;
    - a whole number as a number box;
    - anything else as text.
  - A secret gets a passphrase box that never shows the value; left empty, the value is kept (0027).
  - A field's `default` in the schema says what it means when unset. For example, an unset network broadcasts.
  - The server still checks every value before anything is recorded.
- **An open form or preview stops the page redrawing.** Anything open for editing carries a `data-editing` marker, and the page is not refreshed while one is there. This replaces a counter that some paths left unbalanced, which kept a page from refreshing.

## Consequences

- Changing a network from one location changes it at every location its service folder applies to. The preview says which APs that reaches before anything is recorded.
- A new network setting appears in the form, under More settings, as soon as it is in the schema. The agent and the render check must still learn it before APs can apply it.
