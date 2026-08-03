import {describe, test, expect, beforeEach} from 'vitest';
import {ItemsDatabase} from './ItemsDatabase.js';

function seed(db, entries) {
    for (const [id, item] of entries) {
        db.items.set(id, item);
    }
}

describe('ItemsDatabase.searchByName', () => {
    let db;

    beforeEach(() => {
        db = new ItemsDatabase();
        seed(db, [
            [1, {name: 'T4_MAIN_SWORD', tier: 4, itempower: 700, enchant: 0}],
            [2, {name: 'T5_2H_SWORD', tier: 5, itempower: 800, enchant: 0}],
            [3, {name: 'T4_BAG', tier: 4, itempower: 100, enchant: 0}],
            [4, {name: 'T5_BAG@1', tier: 5, itempower: 100, enchant: 1}],
        ]);
    });

    test('matches a case-insensitive substring', () => {
        const results = db.searchByName('sword');

        expect(results).toHaveLength(2);
        expect(results.map(r => r.name)).toEqual(['T4_MAIN_SWORD', 'T5_2H_SWORD']);
    });

    test('matches regardless of query case', () => {
        const results = db.searchByName('BAG');

        expect(results.map(r => r.name)).toEqual(['T4_BAG', 'T5_BAG@1']);
    });

    test('includes the item id alongside its fields', () => {
        const results = db.searchByName('T4_BAG');

        expect(results).toEqual([{id: 3, name: 'T4_BAG', tier: 4, itempower: 100, enchant: 0}]);
    });

    test('returns an empty array for no matches', () => {
        expect(db.searchByName('NOT_AN_ITEM')).toEqual([]);
    });

    test('returns an empty array for an empty query', () => {
        expect(db.searchByName('')).toEqual([]);
    });

    test('respects the limit parameter', () => {
        seed(db, [
            [5, {name: 'T4_2H_SWORD', tier: 4, itempower: 700, enchant: 0}],
            [6, {name: 'T6_2H_SWORD', tier: 6, itempower: 900, enchant: 0}],
        ]);

        const results = db.searchByName('sword', 2);

        expect(results).toHaveLength(2);
    });

    test('defaults to a limit of 20', () => {
        for (let i = 100; i < 125; i++) {
            db.items.set(i, {name: `T4_ITEM_${i}`, tier: 4, itempower: 100, enchant: 0});
        }

        expect(db.searchByName('T4_ITEM_')).toHaveLength(20);
    });
});
