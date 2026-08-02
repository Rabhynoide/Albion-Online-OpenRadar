// synthetic: no pcap fixture file yet; parameter shapes below are taken verbatim from a real
// live capture (2026-07-30) decoded via a throwaway internal/photon dump script, not guessed.

import {describe, test, expect, beforeEach, vi} from 'vitest';

vi.mock('../utils/SettingsSync.js', () => ({
    default: {
        getBool: vi.fn(() => true),
    },
}));

const {LocalTreasuresHandler} = await import('./LocalTreasuresHandler.js');
const settingsSync = (await import('../utils/SettingsSync.js')).default;

describe('LocalTreasuresHandler', () => {
    let handler;

    beforeEach(() => {
        vi.clearAllMocks();
        settingsSync.getBool.mockReturnValue(true);
        window.logger = {debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn()};
        handler = new LocalTreasuresHandler();
    });

    describe('handleLocalTreasuresUpdate (event 285)', () => {
        // @verified 2026-07-30: live capture, settingLocalTreasures=true. Two entities in one
        // packet: a SPECIAL_EVENT_1 (excluded, see below) and a buried treasure chest
        // (endTicks=0, no closing time) - only the chest should end up in the list.
        test('live capture: parses parallel arrays into one Treasure per drawable entity', () => {
            handler.handleLocalTreasuresUpdate({
                4: [77706, 78042],
                5: [40, -310, 121, -141],
                6: [639210373950865300, 639210375691831200],
                7: [639211236063285500, 0],
                8: ['SPECIAL_EVENT_1', 'CHEST'],
            });

            expect(handler.treasuresList).toHaveLength(1);

            const chest = handler.treasuresList.find(t => t.id === 78042);
            expect(chest.posX).toBe(121);
            expect(chest.posY).toBe(-141);
            expect(chest.label).toBe('CHEST');
            expect(chest.endTicks).toBe(0);
        });

        // @verified 2026-07-30: live capture confirmed SPECIAL_EVENT_1 id 77706 is also a real
        // NewMob (MOB_EVENT_LEAD_UP_SPEARMAN_T7) - drawing it here would duplicate an encounter
        // already shown, with better threat info, by the existing mob detection.
        test('SPECIAL_EVENT_* labels are excluded (duplicate of existing mob detection)', () => {
            handler.handleLocalTreasuresUpdate({
                4: [77706, 11306], 5: [40, -310, -537, 220], 6: [1, 1], 7: [0, 0],
                8: ['SPECIAL_EVENT_1', 'SPECIAL_EVENT_3'],
            });

            expect(handler.treasuresList).toHaveLength(0);
        });

        // @verified 2026-07-30: ANNIVERSARY was checked against the same capture and has no
        // matching mob id, so unlike SPECIAL_EVENT_* it stays drawable.
        test('ANNIVERSARY label is not excluded', () => {
            handler.handleLocalTreasuresUpdate({
                4: [48499], 5: [0, 0], 6: [1], 7: [0], 8: ['ANNIVERSARY'],
            });

            expect(handler.treasuresList).toHaveLength(1);
            expect(handler.treasuresList[0].label).toBe('ANNIVERSARY');
        });

        // @verified 2026-07-30: settingLocalTreasures=false causes an early return; nothing is added.
        test('synthetic: settingLocalTreasures=false returns early', () => {
            settingsSync.getBool.mockReturnValue(false);

            handler.handleLocalTreasuresUpdate({
                4: [1], 5: [0, 0], 6: [1], 7: [0], 8: ['CHEST'],
            });

            expect(handler.treasuresList).toHaveLength(0);
        });

        // @verified 2026-07-30: Photon collapses single-element arrays to a bare scalar; a
        // one-treasure update must still parse correctly (defensive Array.isArray guard).
        test('synthetic: a single treasure collapsed to scalar parameters still parses', () => {
            handler.handleLocalTreasuresUpdate({
                4: 555, 5: [10, 20], 6: 100, 7: 0, 8: 'SMUGGLER_PILE',
            });

            expect(handler.treasuresList).toHaveLength(1);
            expect(handler.treasuresList[0]).toMatchObject({
                id: 555, posX: 10, posY: 20, label: 'SMUGGLER_PILE',
            });
        });

        // @verified 2026-07-30: a malformed entry missing its position pair is skipped, not crashed on.
        test('synthetic: an entity missing a position pair is skipped', () => {
            handler.handleLocalTreasuresUpdate({
                4: [1, 2], 5: [10, 20], 6: [1, 2], 7: [0, 0], 8: ['CHEST', 'RESOURCE_T6'],
            });

            expect(handler.treasuresList).toHaveLength(1);
            expect(handler.treasuresList[0].id).toBe(1);
        });

        // @verified 2026-07-30: second update with the same id calls touch() instead of duplicating.
        test('synthetic: dedup by id calls touch on existing treasure', () => {
            handler.handleLocalTreasuresUpdate({4: [9], 5: [0, 0], 6: [1], 7: [0], 8: ['CHEST']});
            const treasure = handler.treasuresList[0];
            treasure.lastUpdateTime -= 5000;
            const preTouchTime = treasure.lastUpdateTime;

            handler.handleLocalTreasuresUpdate({4: [9], 5: [99, 99], 6: [1], 7: [0], 8: ['CHEST']});

            expect(handler.treasuresList).toHaveLength(1);
            expect(handler.treasuresList[0].lastUpdateTime).toBeGreaterThan(preTouchTime);
        });
    });

    describe('removeTreasure (via Leave, event 1)', () => {
        // @verified 2026-07-30: live capture confirmed individual treasure ids receive a normal
        // single-id Leave event, same as every other entity type - no batch removal needed.
        test('synthetic: removeTreasure removes only the matching id', () => {
            handler.treasuresList.push({id: 1, posX: 0, posY: 0, label: 'CHEST', lastUpdateTime: Date.now(), touch() {}});
            handler.treasuresList.push({id: 2, posX: 0, posY: 0, label: 'CHEST', lastUpdateTime: Date.now(), touch() {}});

            handler.removeTreasure(1);

            expect(handler.treasuresList).toHaveLength(1);
            expect(handler.treasuresList[0].id).toBe(2);
        });

        test('synthetic: removeTreasure with unknown id is a no-op', () => {
            handler.treasuresList.push({id: 1, posX: 0, posY: 0, label: 'CHEST', lastUpdateTime: Date.now(), touch() {}});

            handler.removeTreasure(9999);

            expect(handler.treasuresList).toHaveLength(1);
        });
    });

    describe('Clear', () => {
        test('synthetic: Clear empties the treasures list', () => {
            handler.treasuresList.push({id: 1, posX: 0, posY: 0, label: 'CHEST', lastUpdateTime: Date.now(), touch() {}});
            handler.treasuresList.push({id: 2, posX: 0, posY: 0, label: 'CHEST', lastUpdateTime: Date.now(), touch() {}});

            handler.Clear();

            expect(handler.treasuresList).toHaveLength(0);
        });
    });

    describe('cleanupStaleEntities', () => {
        test('synthetic: cleanupStaleEntities removes treasures past maxAgeMs, keeps fresh ones', () => {
            const now = Date.now();
            handler.treasuresList.push({id: 1, lastUpdateTime: now - 200000, posX: 0, posY: 0, touch() {}});
            handler.treasuresList.push({id: 2, lastUpdateTime: now - 10, posX: 0, posY: 0, touch() {}});

            const removed = handler.cleanupStaleEntities(120000);

            expect(removed).toBe(1);
            expect(handler.treasuresList).toHaveLength(1);
            expect(handler.treasuresList[0].id).toBe(2);
        });
    });
});
