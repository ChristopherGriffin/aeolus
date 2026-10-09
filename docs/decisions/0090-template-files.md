# 0090. Template files: ready-made templates in the repo, downloaded and imported

- Status: Proposed
- Date: 2026-10-09
- Proposed by: Griff. Aeolus shouldn't create ready-made templates by itself; they should be available, and downloadable, from the git repo. The C-360's should hold its radio plan, the country, the protocols and the power. Written up by Claude
- Refines: 0085

## Context

- **0085's templates start empty.** The manager makes one, with no settings, for each new kind of AP adopted. Anything in it is set by hand.
- **The lab has a proven radio plan for the C-360** (0087, 0089). Anyone else with C-360s would have to rebuild it field by field.

## Decision

- **A template file** is JSON:
  - `"aeolus_template": 1`;
  - `name`;
  - `boards`, as OpenWrt names them;
  - an optional `about`;
  - `values`, Locations fields by path, as the schema has them.

  Where the template lives isn't in the file: that is picked when it is imported.
- **`templates/` in the repo holds ready-made ones,** starting with `arista-c360.json`:
  - US;
  - 6 GHz at 160 MHz, automatic, on preferred scanning channels, one a block;
  - 5 GHz at 80 MHz, automatic;
  - 2.4 GHz at 20 MHz, automatic, which means 1, 6 and 11;
  - automatic power on every band;
  - 802.11g and newer on 2.4 GHz, everything on 5 GHz, 802.11ax on 6 GHz.

  Aeolus never creates these by itself. CI imports every file there, so one the schema would refuse fails the build.
- **Download,** on a template's page in the Library, saves it as a file in that format, named after its ID. Sealed values (0027) can't be read back, so they are left out, and the page says which.
- **Import template…,** on a folder's Templates tab, reads a file and shows its name, boards and settings. The name and boards can be changed there, and it is picked at the folder for its boards unless Pick it here is unticked. It is then made as one change: `add-template` now carries `values`, checked against the schema and sealed as any setting is. Its ID comes from the name, with -2, -3 and so on added where the library has that ID already.
- **The manager's own automatic templates stay empty:** an `add-template` in the manager's name with values is refused.

## Consequences

- A template moves between Aeolus installs, and back to the repo, as a file.
- A file with a field a template may not set, or a value the schema refuses, is refused whole, and nothing is made.
