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

describe('ItemsDatabase.searchItems', () => {
    let db;

    beforeEach(() => {
        db = new ItemsDatabase();
        seed(db, [
            [1, {name: 'T4_MAIN_SWORD', tier: 4, itempower: 700, enchant: 0, category: 'weapons'}],
            [2, {name: 'T4_MAIN_SWORD@1', tier: 4, itempower: 700, enchant: 1, category: 'weapons'}],
            [3, {name: 'T5_2H_SWORD', tier: 5, itempower: 800, enchant: 0, category: 'weapons'}],
            [4, {name: 'T4_BAG', tier: 4, itempower: 100, enchant: 0, category: 'bags'}],
            [5, {name: 'T4_OFF_SHIELD', tier: 4, itempower: 100, enchant: 0, category: 'offhands'}],
        ]);
    });

    // @verified 2026-08-03 (issue: market filters): mirrors the in-game marketplace's
    // Category/Niveau/Enchantement browse controls, which work with or without free text.
    test('with no filters at all, returns everything up to the limit', () => {
        expect(db.searchItems()).toHaveLength(5);
    });

    test('filters by category alone, no text query needed', () => {
        const results = db.searchItems({category: 'weapons'});

        expect(results.map(r => r.name)).toEqual(['T4_MAIN_SWORD', 'T4_MAIN_SWORD@1', 'T5_2H_SWORD']);
    });

    test('filters by tier alone', () => {
        const results = db.searchItems({tier: 4});

        expect(results.map(r => r.name)).toEqual(['T4_MAIN_SWORD', 'T4_MAIN_SWORD@1', 'T4_BAG', 'T4_OFF_SHIELD']);
    });

    test('accepts tier as a string (matches a <select> element\'s value type)', () => {
        const results = db.searchItems({tier: '5'});

        expect(results.map(r => r.name)).toEqual(['T5_2H_SWORD']);
    });

    test('filters by enchant alone, distinguishing enchant 0 from unset', () => {
        const results = db.searchItems({enchant: 0});

        expect(results.map(r => r.name)).toEqual(['T4_MAIN_SWORD', 'T5_2H_SWORD', 'T4_BAG', 'T4_OFF_SHIELD']);
    });

    test('combines text query with category/tier/enchant filters', () => {
        const results = db.searchItems({query: 'sword', category: 'weapons', tier: 4});

        expect(results.map(r => r.name)).toEqual(['T4_MAIN_SWORD', 'T4_MAIN_SWORD@1']);
    });

    test('returns nothing when a filter matches no item', () => {
        expect(db.searchItems({category: 'mounts'})).toEqual([]);
    });

    test('respects the limit parameter', () => {
        expect(db.searchItems({category: 'weapons'}, 2)).toHaveLength(2);
    });
});

describe('ItemsDatabase.searchItems subcategory filter', () => {
    let db;

    beforeEach(() => {
        db = new ItemsDatabase();
        seed(db, [
            [1, {name: 'T4_HEAD_CLOTH_SET1', tier: 4, itempower: 700, enchant: 0, category: 'head', subcategory: 'cloth_armor'}],
            [2, {name: 'T4_HEAD_PLATE_SET1', tier: 4, itempower: 700, enchant: 0, category: 'head', subcategory: 'plate_armor'}],
            [3, {name: 'T5_ARMOR_CLOTH_SET1', tier: 5, itempower: 800, enchant: 0, category: 'armors', subcategory: 'cloth_armor'}],
            [4, {name: 'T4_BAG', tier: 4, itempower: 100, enchant: 0, category: 'bags', subcategory: null}],
        ]);
    });

    // @verified 2026-08-03 (issue: cascading market filters): mirrors the in-game marketplace's
    // Category -> Sub-type flyout, where Sub-type narrows within an already-chosen Category.
    test('filters by subcategory alone', () => {
        const results = db.searchItems({subcategory: 'cloth_armor'});

        expect(results.map(r => r.name)).toEqual(['T4_HEAD_CLOTH_SET1', 'T5_ARMOR_CLOTH_SET1']);
    });

    test('combines category and subcategory filters', () => {
        const results = db.searchItems({category: 'head', subcategory: 'cloth_armor'});

        expect(results.map(r => r.name)).toEqual(['T4_HEAD_CLOTH_SET1']);
    });

    test('items with no subcategory are excluded when a subcategory filter is set', () => {
        expect(db.searchItems({subcategory: 'plate_armor'})).toHaveLength(1);
    });

    test('returns nothing when the subcategory matches no item', () => {
        expect(db.searchItems({subcategory: 'shieldtype'})).toEqual([]);
    });
});
