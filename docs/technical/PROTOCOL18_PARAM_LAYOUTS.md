# Protocol18 Event Parameter Layouts (observed)

Status: snapshot from a single live capture (T5 zone + harvest + zone transit,
~130s, 3020 packets, 4954 events decoded). Use as baseline for regression tests
and frontend compatibility checks. Expect shifts when Albion patches gameplay
payload layouts.

## Convention

- **Dispatch byte** (`EventData.Code`): the 1-byte event selector read from the
  wire. Under Protocol18 Albion only uses two dispatch bytes: `3` (Move, hot
  path) and `1` (generic, real code in `params[252]`).
- **Real code** (`params[252]` as `int16`): the authoritative Albion event ID.
  74 distinct codes observed in this capture.
- **Frontend truth**: `web/scripts/core/EventRouter.js:117` routes on
  `params[252]`. Backend logs follow the same convention
  (`cmd/radar/main.go:onPhotonEvent`).

## Timestamp encoding: .NET `DateTime.Ticks`

Confirmed (2026-07-30, live capture cross-check, not guessed): timestamp-shaped fields on the
wire are **.NET ticks** — 100ns intervals since `0001-01-01`. Conversion:

```
ticks = (unixSeconds + 62135596800) * 10_000_000
```

Verified by computing "now" in ticks from a screenshot's wall-clock estimate and matching it
against real `Move` request `Parameters[0]` values (dispatch byte 3, see below) to within ~1
minute. JS can do this math with plain `Number` (not `BigInt`) despite ticks values exceeding
`Number.MAX_SAFE_INTEGER` — the float64 rounding error at this magnitude is on the order of
100 ticks (~10μs), utterly negligible for anything at second/minute granularity like a
countdown display.

## Dispatch byte 3: Move

```
params[  0] int64            entity id
params[  1] ByteArray        raw 30-byte (mode=3) or 22-byte (mode=4) blob
params[  4] float32          posX, injected by PostProcessEvent (offset 9 of params[1])
params[  5] float32          posY, injected by PostProcessEvent (offset 13 of params[1])
params[252] byte(3)          fallback-injected by PostProcessEvent (server omits)
```

**Notes:**
- Mobs/resources: `params[1][0] == 3`, positions at offsets 9/13 are unencrypted.
- Players: `params[1][0] == 3` too, but positions at offsets 9/13 are XOR-encrypted
  with the KeySync `XorCode`, unreadable without MITM (see `PLAYER_POSITIONS_MITM.md`).
- Mode 4 (22-byte) never has positions; the `len(raw) < 17` guard handles it.

## Dispatch byte 1: Generic (real code in `params[252]`)

Representative layouts observed:

| Real code | Purpose (guess from context) | Notable params |
|-----------|------------------------------|----------------|
| 1         | Leave                        | `params[0]` int64 entity id only |
| 6         | HealthUpdate (single)        | `params[2..3]` float32 HP, `params[6]` int64 attacker |
| 8         | HealthUpdate (alt)           | `params[2..3]` float32, similar shape |
| 11        | HealthUpdates (bulk)         | `params[1..10]` mixed arrays + ByteArrays |
| 14        | ?                            | `params[2]` `[]float32`, `params[8..9]` uint8 (flags) |
| 19        | ?                            | `params[2]` `[]float32`, `params[7]` int16 |
| 22        | ?                            | `params[1]` `[]int32`, `params[3..4]` ByteArray |
| 29        | NewCharacter (player spawn)  | `params[1]` string (name), `params[5..7,16,17]` ByteArray, `params[19..37]` float32 stats |
| 30        | ?                            | `params[5]` string, `params[8]` ByteArray, `params[9]` `[]int16`/ByteArray |
| 39        | NewHarvestableObjectList     | `params[0]` `[]int16` batch ids, `params[3]` `[]float32` batch positions |
| 40        | NewMob (probable)            | `params[8]` **`[]float32`** packed X/Y, `params[9]` `float32` scalar (rotation?) |
| 91        | ?                            | `params[2..3,5]` float32, `params[6]` int64 |
| 285       | LocalTreasuresUpdate         | Parallel arrays, one entry per active local treasure - see full layout below |
| 325       | NewRandomDungeonExit         | Dungeon/road portal creation - see full layout below |

**Frontend layout note (real code 40):**

`web/scripts/handlers/MobsHandler.js` reads positions from `parameters[8]` as a
`[]float32` array of length 2 (packed X and Y), not as scalars. The current code
unpacks `loc[0]` and `loc[1]` into `posX` and `posY` after a `normalizeNumber`
guard. Upstream `ao-data/albiondata-client` and
`Triky313/AlbionOnline-StatisticsAnalysis` decode the same wire shape.

### Event 325 (`NewRandomDungeonExit`) full layout

Covers both static dungeon entrances *and* Avalon Road portal entrances (e.g. type
`T5_PORTAL_ROYAL_SOLO`). Confirmed 2026-07-30 via live capture, fully characterized:

```
params[ 0] int          entity id
params[ 1] []float32    position [x, y]
params[ 2] float32      heading/rotation - only present on some variants (e.g. corrupted), always 90 when present
params[ 3] string       type (T5_PORTAL_ROYAL_SOLO, CORRUPTED_SOLO_NONLETHAL, T5_MORGANA, T6_HERETIC, ...)
params[ 5] string       file/asset name
params[ 6] int          fixed constant PER ASSET FILE (e.g. PORTAL_SOLO_A->228, PORTAL_SOLO_C->230) -
                         confirmed NOT a timer: two different physical portals with the same file
                         always show the same value here.
params[ 7] bool         always observed true
params[ 8] int          enchant level
params[11] bool         present only on corrupted variants
params[14] int          always 0
params[16] int          -1 on dungeon/road variants; see Mists exception below
params[17] int          1 or 2, meaning not determined
params[19] int          always 0
```

**Confirmed absent** for dungeon/road variants: destination zone name/id (never a literal
string on the wire - resolved client-side, same as this radar resolves names from
`zones.json`) and any expiration/remaining-lifetime field. See
`docs/technical/AVALON_ROADS_GPS.md` for the full portal-duration investigation and why the
GPS feature doesn't show a countdown.

**Mists wisp exception**: for type `MISTS_SOLO_YELLOW` / file `SHARED_MIST_WISP_PORTAL_MOB`
(the "feu follet" sign preceding a Mists portal), the same event carries two extra fields not
present on dungeon/road variants:
```
params[15] string   repeats the type (or eventual portal type)
params[16] int       entity id of an associated NewMob (event 123) created in the same packet
                      burst at the same position - not -1 here. Possibly relevant to a future
                      "Mist bosses" feature (issue #127).
params[18] int64     real .NET-ticks timestamp - confirmed ~30s in the future relative to
                      surrounding Move-request ticks. Almost certainly "when the portal spawns",
                      matching the documented feu-follet-before-portal mechanic. Not read
                      anywhere in the current codebase.
```

### Event 285 (`LocalTreasuresUpdate`) full layout

A **periodic full resync** of every active local treasure in the zone, delivered as parallel
arrays (one entry per treasure) rather than one event per entity - the only detection type in
this codebase shaped this way. Confirmed 2026-07-30:

```
params[4] []int        entity ids
params[5] []float32     flat position array [x0,y0,x1,y1,...] - 2 floats per entity, same
                         order as params[4]
params[6] []int64       start tick (.NET ticks) per entity
params[7] []int64       end tick (.NET ticks) per entity - 0 means "no closing time yet"
params[8] []string      type label per entity: CHEST, RESOURCE_T6, SMUGGLER_PILE,
                         SPECIAL_EVENT_1, SPECIAL_EVENT_3, ANNIVERSARY (observed values)
```

Removal is **not** batched: individual entity ids from this event later receive a normal
single-id `Leave` (event 1), same as every other entity type - confirmed by tracking specific
ids across a capture.

`SPECIAL_EVENT_1`/`SPECIAL_EVENT_3` share their entity id with a real `NewMob` (event 123) -
e.g. id `77706` was simultaneously a `SPECIAL_EVENT_1` entry here and a `NewMob` with
`params[31] = "MOB_EVENT_LEAD_UP_SPEARMAN_T7"`. `web/scripts/handlers/LocalTreasuresHandler.js`
excludes these labels for exactly this reason - drawing them would duplicate an encounter
already shown, with better threat info, by the existing mob detection. `ANNIVERSARY` was
checked against the same capture and has no matching mob id, so it stays drawable. `CHEST`
entries observed so far always have `endTicks = 0` (no expiration - a buried treasure just
sits there until dug up); `SPECIAL_EVENT_1`/`ANNIVERSARY` entries carry a real ~24h
start/end window, matching this event's status as the reference pattern for any future
"does this entity have a real lifetime" investigation.

### Party events (231, 232, 235, 229)

Confirmed 2026-08-01 via a live capture of a 2-person party being formed, disbanded, and
re-formed (invite, accept, leave). Motivated by issue #3 - auto-excluding party members from
hostile-player alerts, same mechanism as the manual Ignore List (see
`web/scripts/data/PartyRoster.js` and `PlayersHandler.js`).

**Event 231 (`PartyJoined`)** - a full party roster snapshot, sent to the client whenever
their own party's membership state is (re)established (initial join, and again after
disband+reform in this capture - not confirmed whether it also fires for every individual
join/leave of an *existing* party you're already in, since this capture never exercised a
3+-person party):

```
params[ 0] int          party/group instance id (changes each time a party is formed)
params[ 4] byte[16]     GUID of one member (the party leader, in every observed sample)
params[ 8] byte[]       concatenated 16-byte member GUIDs, one per member, same order as [9]
params[ 9] string[]     member display names, same order as the GUIDs in [8] - the field
                        that actually matters for auto-whitelisting
params[ 2] int          always 1 in this capture - meaning not determined
params[10-13] byte[N]   per-member arrays (N = member count) - meaning not determined
params[14] int[]        per-member array (e.g. [-1,-1]) - meaning not determined
params[15] int[]        per-member array (e.g. [6,-1]) - meaning not determined
params[16] bool[]       per-member array (e.g. [true,true]) - plausibly "online", not confirmed
```

**Event 232 (`PartyDisbanded`)**: `params[1]` int, the party id being disbanded (matches a
prior `PartyJoined`'s `params[0]`).

**Event 235 (`PartyPlayerLeft`)**: `params[0]` int party id, `params[1]` byte[16] GUID of the
member who left - **no name field**, must be resolved against a GUID→name map built from the
most recent `PartyJoined` for that party (`PartyRoster.js` does this).

**Event 229 (`PartyInvitation`, incoming invite, not yet accepted)**: also carries names
directly - `params[0]` byte[16] inviter GUID, `params[1]` string inviter name, `params[6]`
string inviter's guild name, `params[7]` int the party id you'd be joining, `params[13]`
string[] the current roster at invite time. Not currently consumed (the party isn't real until
`PartyJoined` fires), documented here since it independently confirms the params[0]-is-party-id
and name-array patterns above.

**Confirmed absent**: `PartyPlayerJoined` (233), the event whose name most directly suggests
"someone joined", never appeared in this capture at all - `PartyJoined` (231) covered every
membership change observed. Not yet confirmed whether 233 exists for scenarios this capture
didn't exercise (3+ person party, joining an already-formed party mid-session while other
members are online).

## Gaps in this snapshot

- No combat events (Cast*, Damage*): single idle plus harvest scenario.
- No JoinResponse (zone transit happened once; only real code 0 response
  observed, 1 sample). Covered by `move_map_change.pcap` (see below).
- Only 2 fragments in the capture; fragment reassembly is exercised in
  `TestPhotonParser_Fragment_*` unit tests instead.
- `msg_type 132` (53 occurrences) and `130` (3) silently dropped, not event
  / request / response. Proximity to `msgEncrypted=131` suggests encrypted
  variants. Worth investigating when more captures arrive.

## Router-contract fixture: `move_map_change.pcap`

A 584-packet capture exercising the three operation codes the frontend
router dispatches on. Under Protocol18 the wire `OperationCode` byte is
always 1; the real code lives in `Parameters[253]` as `int16`. These are
the counts observed across the fixture:

| Kind     | Real code | Name            | Count | Key params                                   |
|----------|-----------|-----------------|-------|----------------------------------------------|
| request  | 22        | Move            | 79    | `[0]` int64 entity id, `[1]` `[]float32` src pos (len 2), `[3]` `[]float32` tgt pos |
| response | 2         | JoinFinished    | 2     | `[8]` string mapId, `[9]` `[]float32` local player pos |
| response | 41        | ChangeCluster   | 1     | `[0]` string new mapId                       |

`TestLivePcap_RouterContract` in `internal/photon/live_pcap_test.go` pins
these as minimum expectations. Drop new router-visible codes into the
`cases` table there; add new fixtures with `tools/anonymize-pcap` before
committing.

## How to regenerate this document

Feed a new pcap through the analyzer pattern documented in the PR description
for issue #49. Compare the resulting layout table with this snapshot to detect
protocol drift across patches.
