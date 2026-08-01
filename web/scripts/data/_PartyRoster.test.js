import {describe, test, expect, beforeEach, vi} from 'vitest';
import {PartyRoster} from './PartyRoster.js';

// Real bytes from a live capture (2026-08-01, see docs/technical/PROTOCOL18_PARAM_LAYOUTS.md's
// "Party events" section): two 16-byte character GUIDs, Rabhynoide then S3phir0th.
const GUID_RABHYNOIDE = [71, 70, 110, 125, 113, 214, 74, 67, 157, 214, 3, 7, 181, 82, 201, 122];
const GUID_S3PHIR0TH = [104, 187, 170, 253, 225, 251, 184, 68, 147, 179, 90, 103, 114, 134, 25, 156];

function partyJoinedParams(names, guids) {
    return {
        0: 71412,
        8: guids.flat(),
        9: names,
        252: 231,
    };
}

describe('PartyRoster', () => {
    let roster;

    beforeEach(() => {
        roster = new PartyRoster();
        window.logger = {info: vi.fn(), warn: vi.fn()};
    });

    describe('handlePartyJoined', () => {
        // @verified pcap-derived 2026-08-01: params[9] is the member name array, same order
        // as the concatenated 16-byte GUIDs in params[8].
        test('sets the roster from params[9]', () => {
            roster.handlePartyJoined(partyJoinedParams(['Rabhynoide', 'S3phir0th'], [GUID_RABHYNOIDE, GUID_S3PHIR0TH]));

            expect(roster.isPartyMember('Rabhynoide')).toBe(true);
            expect(roster.isPartyMember('S3phir0th')).toBe(true);
            expect(roster.isPartyMember('SomeoneElse')).toBe(false);
        });

        test('replaces (not merges with) a previous roster', () => {
            roster.handlePartyJoined(partyJoinedParams(['Rabhynoide', 'S3phir0th'], [GUID_RABHYNOIDE, GUID_S3PHIR0TH]));
            roster.handlePartyJoined(partyJoinedParams(['S3phir0th'], [GUID_S3PHIR0TH]));

            expect(roster.isPartyMember('Rabhynoide')).toBe(false);
            expect(roster.isPartyMember('S3phir0th')).toBe(true);
        });

        test('ignores a malformed payload (missing params[9]) without throwing', () => {
            expect(() => roster.handlePartyJoined({0: 71412, 252: 231})).not.toThrow();
            expect(roster.isPartyMember('Rabhynoide')).toBe(false);
        });

        test('does not build a GUID map when params[8] length does not match params[9]', () => {
            // Mismatched lengths (e.g. a future payload variant) - stay defensive rather than
            // guess a wrong GUID<->name pairing.
            roster.handlePartyJoined({0: 71412, 8: GUID_RABHYNOIDE, 9: ['Rabhynoide', 'S3phir0th'], 252: 231});

            roster.handlePartyPlayerLeft({0: 71412, 1: GUID_RABHYNOIDE, 252: 235});
            // Roster itself is still set from params[9]; only the GUID resolution is skipped.
            expect(roster.isPartyMember('Rabhynoide')).toBe(true);
        });
    });

    describe('handlePartyPlayerLeft', () => {
        // @verified pcap-derived: event 235 carries only params[1] (leaving member's GUID),
        // resolved against the map built from the most recent PartyJoined.
        test('removes the member matching the GUID', () => {
            roster.handlePartyJoined(partyJoinedParams(['Rabhynoide', 'S3phir0th'], [GUID_RABHYNOIDE, GUID_S3PHIR0TH]));

            roster.handlePartyPlayerLeft({0: 71412, 1: GUID_RABHYNOIDE, 252: 235});

            expect(roster.isPartyMember('Rabhynoide')).toBe(false);
            expect(roster.isPartyMember('S3phir0th')).toBe(true);
        });

        test('is a no-op for an unrecognized GUID', () => {
            roster.handlePartyJoined(partyJoinedParams(['Rabhynoide'], [GUID_RABHYNOIDE]));

            roster.handlePartyPlayerLeft({0: 71412, 1: GUID_S3PHIR0TH, 252: 235});

            expect(roster.isPartyMember('Rabhynoide')).toBe(true);
        });

        test('does not throw when no roster has ever been set', () => {
            expect(() => roster.handlePartyPlayerLeft({0: 71412, 1: GUID_RABHYNOIDE, 252: 235})).not.toThrow();
        });
    });

    describe('handlePartyDisbanded', () => {
        test('clears the entire roster', () => {
            roster.handlePartyJoined(partyJoinedParams(['Rabhynoide', 'S3phir0th'], [GUID_RABHYNOIDE, GUID_S3PHIR0TH]));

            roster.handlePartyDisbanded();

            expect(roster.isPartyMember('Rabhynoide')).toBe(false);
            expect(roster.isPartyMember('S3phir0th')).toBe(false);
        });
    });

    describe('reset', () => {
        // @verified: called from EventRouter.reset() (radar session teardown) - a stale roster
        // from a party that changed while the radar wasn't running must not persist.
        test('clears the roster like handlePartyDisbanded', () => {
            roster.handlePartyJoined(partyJoinedParams(['Rabhynoide'], [GUID_RABHYNOIDE]));

            roster.reset();

            expect(roster.isPartyMember('Rabhynoide')).toBe(false);
        });

        test('a fresh PartyJoined after reset works normally', () => {
            roster.handlePartyJoined(partyJoinedParams(['Rabhynoide'], [GUID_RABHYNOIDE]));
            roster.reset();

            roster.handlePartyJoined(partyJoinedParams(['S3phir0th'], [GUID_S3PHIR0TH]));

            expect(roster.isPartyMember('S3phir0th')).toBe(true);
        });
    });
});
