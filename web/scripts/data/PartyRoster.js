import {CATEGORIES} from '../constants/LoggerConstants.js';

// Tracks the local player's current party membership by display name, learned live from
// Photon Party events (see docs/technical/PROTOCOL18_PARAM_LAYOUTS.md's "Party events"
// section - wire layout confirmed 2026-08-01 via a real capture, not guessed). Session-only
// by design, unlike the persisted Ignore List: a party roster shouldn't survive a page reload
// or outlive the party itself - see reset().
export class PartyRoster {
    constructor() {
        this.members = new Set(); // display names, includes the local player themselves
        this._guidToName = new Map(); // last-seen GUID(hex) -> name, for PartyPlayerLeft lookups
    }

    static _guidHex(byteArray) {
        return Array.from(byteArray).map((b) => b.toString(16).padStart(2, '0')).join('');
    }

    // Event 231 (PartyJoined): a full roster snapshot, sent whenever the local player's own
    // party membership state changes. params[8] is the concatenated 16-byte member GUIDs, in
    // the same order as params[9]'s names - zipped here into a GUID->name map so
    // handlePartyPlayerLeft (which only gets a GUID, no name) can resolve who left.
    handlePartyJoined(Parameters) {
        const names = Parameters[9];
        const guidBuffer = Parameters[8];
        if (!Array.isArray(names)) return;

        this.members = new Set(names);
        this._guidToName = new Map();
        if (guidBuffer && guidBuffer.length === names.length * 16) {
            for (let i = 0; i < names.length; i++) {
                const guid = PartyRoster._guidHex(guidBuffer.slice(i * 16, i * 16 + 16));
                this._guidToName.set(guid, names[i]);
            }
        }

        window.logger?.info(CATEGORIES.PLAYERS, 'PartyRosterUpdated', {members: [...this.members]});
    }

    // Event 235 (PartyPlayerLeft): only carries the leaving member's GUID, not their name.
    handlePartyPlayerLeft(Parameters) {
        const guidBuffer = Parameters[1];
        if (!guidBuffer) return;
        const guid = PartyRoster._guidHex(guidBuffer);
        const name = this._guidToName.get(guid);
        if (name) {
            this.members.delete(name);
            this._guidToName.delete(guid);
            window.logger?.info(CATEGORIES.PLAYERS, 'PartyMemberLeft', {name});
        }
    }

    // Event 232 (PartyDisbanded): the party is gone entirely.
    handlePartyDisbanded() {
        this.members.clear();
        this._guidToName.clear();
    }

    isPartyMember(nickname) {
        return this.members.has(nickname);
    }

    // Called from EventRouter.reset() (radar session teardown): a stale roster from a party
    // that changed or disbanded while the radar wasn't running would otherwise wrongly keep
    // excluding people on the next session.
    reset() {
        this.members.clear();
        this._guidToName.clear();
    }
}

const partyRoster = new PartyRoster();
export default partyRoster;
