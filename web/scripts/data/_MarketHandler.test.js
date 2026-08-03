import {describe, test, expect, beforeEach, afterEach, vi} from 'vitest';

vi.mock('./ZonesDatabase.js', () => ({
    default: {
        getZoneName: vi.fn(),
    },
}));

const {MarketHandler} = await import('./MarketHandler.js');
const zonesDatabase = (await import('./ZonesDatabase.js')).default;

// Real bytes from a live capture (2026-08-03, see docs/technical/PROTOCOL18_PARAM_LAYOUTS.md's
// "Marketplace operations" section): Parameters[0] is an array of JSON-ENCODED STRINGS, one
// per listing - each needs its own JSON.parse, not a plain array of objects.
function sellListing(overrides = {}) {
    return JSON.stringify({
        Id: 3280526436, UnitPriceSilver: 40000, DistanceFee: 0, TotalPriceSilver: 440000,
        Amount: 11, Tier: 4, IsFinished: false, AuctionType: 'offer',
        HasBuyerFetched: false, HasSellerFetched: false,
        SellerCharacterId: 'adc7ad7e-6f00-4028-8f6e-3ebf869b99ef', SellerName: 'ABOCbxz',
        BuyerCharacterId: null, BuyerName: null,
        ItemTypeId: 'T4_ROCK_LEVEL1@1', ItemGroupTypeId: 'T4_ROCK_LEVEL1',
        EnchantmentLevel: 1, QualityLevel: 1,
        Expires: '2026-09-02T18:52:18.91182', ReferenceId: '67685abc-af77-46ec-93ff-ef101f0f9392',
        LocationId: null,
        ...overrides,
    });
}

function buyListing(overrides = {}) {
    return JSON.stringify({
        Id: 3186376512, UnitPriceSilver: 510000, DistanceFee: 0, TotalPriceSilver: 562020000,
        Amount: 1102, Tier: 1, IsFinished: false, AuctionType: 'request',
        HasBuyerFetched: false, HasSellerFetched: false,
        SellerCharacterId: null, SellerName: null,
        BuyerCharacterId: 'b1b94405-4ad9-40d0-9153-a705e8b2e2c4', BuyerName: 'Kolcifier',
        ItemTypeId: 'T1_2H_TOOL_SICKLE', ItemGroupTypeId: 'T1_2H_TOOL_SICKLE',
        EnchantmentLevel: 0, QualityLevel: 1,
        Expires: '2026-08-09T20:28:45.154349', ReferenceId: 'ad989fb7-1911-4f45-a2bf-da7504f21a5f',
        LocationId: null,
        ...overrides,
    });
}

describe('MarketHandler', () => {
    let handler;
    let originalFetch;

    beforeEach(() => {
        vi.clearAllMocks();
        window.logger = {debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn()};
        window.currentMapId = '4002'; // Fort Sterling Market, per zones.json
        zonesDatabase.getZoneName.mockReturnValue('Fort Sterling Market');
        originalFetch = globalThis.fetch;
        globalThis.fetch = vi.fn(() => Promise.resolve({ok: true}));
        handler = new MarketHandler();
    });

    afterEach(() => {
        globalThis.fetch = originalFetch;
    });

    describe('handleAuctionGetOffers (event 81, sell side)', () => {
        // @verified 2026-08-03: live capture shape, one item/quality group aggregated to
        // sell_price_min/max from the listings' UnitPriceSilver.
        test('aggregates listings into a sell-side observation and submits it', () => {
            handler.handleAuctionGetOffers({
                0: [sellListing({UnitPriceSilver: 40000}), sellListing({UnitPriceSilver: 38000, Id: 2})],
            });

            expect(globalThis.fetch).toHaveBeenCalledTimes(1);
            const [url, init] = globalThis.fetch.mock.calls[0];
            expect(url).toBe('/api/market/observations');
            const body = JSON.parse(init.body);
            expect(body.side).toBe('sell');
            expect(body.entries).toEqual([{
                item_id: 'T4_ROCK_LEVEL1@1', city: 'Fort Sterling', quality: 1,
                sell_price_min: 38000, sell_price_min_date: expect.any(String),
                sell_price_max: 40000, sell_price_max_date: expect.any(String),
            }]);
        });

        test('does not submit anything when the city cannot be resolved', () => {
            zonesDatabase.getZoneName.mockReturnValue('@MISTS@some-instance');

            handler.handleAuctionGetOffers({0: [sellListing()]});

            expect(globalThis.fetch).not.toHaveBeenCalled();
        });

        test('does not submit anything when zonesDatabase has no name for the current zone', () => {
            zonesDatabase.getZoneName.mockReturnValue(null);

            handler.handleAuctionGetOffers({0: [sellListing()]});

            expect(globalThis.fetch).not.toHaveBeenCalled();
        });

        test('excludes finished listings from the aggregation', () => {
            handler.handleAuctionGetOffers({
                0: [sellListing({UnitPriceSilver: 40000}), sellListing({UnitPriceSilver: 1, IsFinished: true, Id: 2})],
            });

            const body = JSON.parse(globalThis.fetch.mock.calls[0][1].body);
            expect(body.entries[0].sell_price_min).toBe(40000);
        });

        test('groups separately by item and by quality', () => {
            handler.handleAuctionGetOffers({
                0: [
                    sellListing({ItemTypeId: 'T4_ROCK_LEVEL1@1', QualityLevel: 1, UnitPriceSilver: 100}),
                    sellListing({ItemTypeId: 'T4_ROCK_LEVEL1@1', QualityLevel: 2, UnitPriceSilver: 200}),
                    sellListing({ItemTypeId: 'T5_ROCK_LEVEL1@1', QualityLevel: 1, UnitPriceSilver: 300}),
                ],
            });

            const body = JSON.parse(globalThis.fetch.mock.calls[0][1].body);
            expect(body.entries).toHaveLength(3);
        });

        test('does not submit anything for an empty listings array', () => {
            handler.handleAuctionGetOffers({0: []});

            expect(globalThis.fetch).not.toHaveBeenCalled();
        });

        test('does not throw and does not submit when Parameters[0] is missing', () => {
            expect(() => handler.handleAuctionGetOffers({})).not.toThrow();
            expect(globalThis.fetch).not.toHaveBeenCalled();
        });

        test('skips a malformed listing entry without throwing', () => {
            expect(() => handler.handleAuctionGetOffers({0: ['{not json', sellListing()]})).not.toThrow();

            expect(globalThis.fetch).toHaveBeenCalledTimes(1);
        });

        test('a backend submit failure does not throw', () => {
            globalThis.fetch.mockRejectedValue(new Error('network down'));

            expect(() => handler.handleAuctionGetOffers({0: [sellListing()]})).not.toThrow();
        });
    });

    describe('handleAuctionGetRequests (event 82, buy side)', () => {
        // @verified 2026-08-03: live capture shape - buy_price_min/max from AuctionType:"request".
        test('aggregates listings into a buy-side observation and submits it', () => {
            handler.handleAuctionGetRequests({
                0: [buyListing({UnitPriceSilver: 510000}), buyListing({UnitPriceSilver: 490000, Id: 2})],
            });

            expect(globalThis.fetch).toHaveBeenCalledTimes(1);
            const body = JSON.parse(globalThis.fetch.mock.calls[0][1].body);
            expect(body.side).toBe('buy');
            expect(body.entries).toEqual([{
                item_id: 'T1_2H_TOOL_SICKLE', city: 'Fort Sterling', quality: 1,
                buy_price_min: 490000, buy_price_min_date: expect.any(String),
                buy_price_max: 510000, buy_price_max_date: expect.any(String),
            }]);
        });
    });

    describe('city resolution', () => {
        // @verified 2026-08-03: the open-city zone itself (not just its market sub-zone) must
        // also resolve correctly - a player could conceivably be tracked there right as a
        // response arrives.
        test('resolves the city from the open-city zone name, not just the market sub-zone', () => {
            zonesDatabase.getZoneName.mockReturnValue('Fort Sterling');

            handler.handleAuctionGetOffers({0: [sellListing()]});

            const body = JSON.parse(globalThis.fetch.mock.calls[0][1].body);
            expect(body.entries[0].city).toBe('Fort Sterling');
        });

        test.each(['Caerleon', 'Bridgewatch', 'Lymhurst', 'Martlock', 'Thetford', 'Brecilien'])(
            'resolves %s from its "<City> Market" zone name',
            (city) => {
                zonesDatabase.getZoneName.mockReturnValue(`${city} Market`);

                handler.handleAuctionGetOffers({0: [sellListing()]});

                const body = JSON.parse(globalThis.fetch.mock.calls[0][1].body);
                expect(body.entries[0].city).toBe(city);
            }
        );
    });
});
