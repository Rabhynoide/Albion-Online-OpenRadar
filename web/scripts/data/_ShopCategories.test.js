import {describe, test, expect, beforeEach, afterEach, vi} from 'vitest';

function jsonResponse(body, ok = true) {
    return Promise.resolve({
        ok,
        status: ok ? 200 : 500,
        json: () => Promise.resolve(body),
    });
}

const FIXTURE = {
    weapons: {
        label: 'Weapons',
        subcategories: {sword: 'Sword', axe: 'Axe'},
    },
    armors: {
        label: 'Chest Armor',
        subcategories: {cloth_armor: 'Cloth Robes', plate_armor: 'Plate Armor'},
    },
};

describe('ShopCategories', () => {
    let originalFetch;
    let shopCategories;

    beforeEach(async () => {
        vi.resetModules();
        window.logger = {debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn()};
        originalFetch = globalThis.fetch;
        globalThis.fetch = vi.fn(() => jsonResponse(FIXTURE));
        shopCategories = await import('./ShopCategories.js');
    });

    afterEach(() => {
        globalThis.fetch = originalFetch;
    });

    test('getCategories/getSubcategories return nothing before load() resolves', () => {
        expect(shopCategories.getCategories()).toEqual([]);
        expect(shopCategories.getSubcategories('weapons')).toEqual([]);
    });

    test('load() fetches shopcategories.json from the default path', async () => {
        await shopCategories.load();

        expect(globalThis.fetch).toHaveBeenCalledWith('/ao-bin-dumps/shopcategories.json', {cache: 'no-cache'});
    });

    test('getCategories() returns top-level categories in taxonomy order after load', async () => {
        await shopCategories.load();

        expect(shopCategories.getCategories()).toEqual([
            {id: 'weapons', label: 'Weapons'},
            {id: 'armors', label: 'Chest Armor'},
        ]);
    });

    test('getSubcategories() returns the sub-types for one category', async () => {
        await shopCategories.load();

        expect(shopCategories.getSubcategories('armors')).toEqual([
            {id: 'cloth_armor', label: 'Cloth Robes'},
            {id: 'plate_armor', label: 'Plate Armor'},
        ]);
    });

    test('getSubcategories() returns an empty array for an unknown category', async () => {
        await shopCategories.load();

        expect(shopCategories.getSubcategories('not_a_category')).toEqual([]);
    });

    test('load() only fetches once, subsequent calls return the cached data', async () => {
        await shopCategories.load();
        await shopCategories.load();

        expect(globalThis.fetch).toHaveBeenCalledTimes(1);
    });

    test('load() rejects on a non-ok response', async () => {
        globalThis.fetch.mockReturnValue(jsonResponse({}, false));

        await expect(shopCategories.load()).rejects.toThrow('Failed to fetch shopcategories.json: 500');
    });
});
