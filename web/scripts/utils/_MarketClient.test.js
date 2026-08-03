import {describe, test, expect, beforeEach, afterEach, vi} from 'vitest';
import {fetchPrices} from './MarketClient.js';

function jsonResponse(body, ok = true) {
    return Promise.resolve({
        ok,
        status: ok ? 200 : 500,
        json: () => Promise.resolve(body),
    });
}

describe('MarketClient.fetchPrices', () => {
    let originalFetch;

    beforeEach(() => {
        window.logger = {debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn()};
        originalFetch = globalThis.fetch;
        globalThis.fetch = vi.fn(() => jsonResponse([]));
    });

    afterEach(() => {
        globalThis.fetch = originalFetch;
    });

    test('builds the query string from items/cities/qualities', async () => {
        await fetchPrices(['T4_BAG', 'T5_BAG'], ['Caerleon', 'Martlock'], [1, 2]);

        expect(globalThis.fetch).toHaveBeenCalledWith(
            '/api/market/prices?items=T4_BAG%2CT5_BAG&locations=Caerleon%2CMartlock&qualities=1%2C2'
        );
    });

    test('omits locations/qualities from the query when not provided', async () => {
        await fetchPrices(['T4_BAG']);

        expect(globalThis.fetch).toHaveBeenCalledWith('/api/market/prices?items=T4_BAG');
    });

    test('returns the parsed JSON body on success', async () => {
        const entries = [{item_id: 'T4_BAG', city: 'Caerleon', quality: 1, sell_price_min: 100}];
        globalThis.fetch.mockReturnValue(jsonResponse(entries));

        const result = await fetchPrices(['T4_BAG']);

        expect(result).toEqual(entries);
    });

    test('returns an empty array without calling fetch when itemIds is empty', async () => {
        const result = await fetchPrices([]);

        expect(result).toEqual([]);
        expect(globalThis.fetch).not.toHaveBeenCalled();
    });

    test('returns an empty array on a non-ok response', async () => {
        globalThis.fetch.mockReturnValue(jsonResponse({}, false));

        const result = await fetchPrices(['T4_BAG']);

        expect(result).toEqual([]);
    });

    test('returns an empty array and logs on a network failure', async () => {
        globalThis.fetch.mockRejectedValue(new Error('network down'));

        const result = await fetchPrices(['T4_BAG']);

        expect(result).toEqual([]);
        expect(window.logger.warn).toHaveBeenCalled();
    });
});
