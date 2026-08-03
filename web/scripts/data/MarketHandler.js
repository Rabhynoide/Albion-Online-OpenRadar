import {CATEGORIES} from '../constants/LoggerConstants.js';
import zonesDatabase from './ZonesDatabase.js';

// Issue #23 (Market Prices, Part B): the marketplace never sends a city/location on the wire -
// confirmed 2026-08-03 via a live capture, LocationId is always null on every listing and no
// request/response parameter carries one either (see PROTOCOL18_PARAM_LAYOUTS.md's
// "Marketplace operations" section). The server infers it from which market building the
// character is physically inside. Zone names for market buildings follow a clean
// "<City> Market" convention, so a substring match against the known royal city list resolves
// the city from the radar's own already-tracked current zone name. Black Market isn't in this
// list - it doesn't appear under that name anywhere in zones.json, so observations made there
// are silently skipped rather than mis-attributed to the wrong city.
const KNOWN_CITIES = ['Caerleon', 'Bridgewatch', 'Fort Sterling', 'Lymhurst', 'Martlock', 'Thetford', 'Brecilien'];

function resolveCurrentCity(mapId) {
    const zoneName = zonesDatabase.getZoneName(mapId);
    if (!zoneName) return null;
    return KNOWN_CITIES.find(city => zoneName.includes(city)) || null;
}

// Matches the public Data Project API's own *_date format (no timezone, e.g.
// "2026-08-03T13:10:00" - see internal/adp.PriceEntry's doc comment) so a live observation's
// date fields look identical to ones sourced from the public API.
function nowAsPriceDate() {
    return new Date().toISOString().replace(/\.\d+Z$/, '');
}

// Groups active (non-finished) listings by (ItemTypeId, QualityLevel) and returns the
// min/max UnitPriceSilver per group - this project's own interpretation of "current sell/buy
// price" (the public API's exact aggregation methodology isn't documented), matching what the
// field names literally mean: the range of currently-listed offers, not historical sales.
function groupByItemAndQuality(listings) {
    const groups = new Map();
    for (const listing of listings) {
        if (listing.IsFinished) continue;
        const key = listing.ItemTypeId + '|' + listing.QualityLevel;
        if (!groups.has(key)) {
            groups.set(key, {itemId: listing.ItemTypeId, quality: listing.QualityLevel, prices: []});
        }
        groups.get(key).prices.push(listing.UnitPriceSilver);
    }
    return [...groups.values()];
}

export class MarketHandler {
    // Event 81 (AuctionGetOffers response) - sell-side listings only (AuctionType:"offer" on
    // every record observed in the reference capture).
    handleAuctionGetOffers(Parameters) {
        this._handleAuctionResponse(Parameters, 'sell');
    }

    // Event 82 (AuctionGetRequests response) - buy-side listings only (AuctionType:"request").
    handleAuctionGetRequests(Parameters) {
        this._handleAuctionResponse(Parameters, 'buy');
    }

    _handleAuctionResponse(Parameters, side) {
        const raw = Parameters[0];
        if (!Array.isArray(raw) || raw.length === 0) return;

        const city = resolveCurrentCity(window.currentMapId);
        if (!city) {
            window.logger?.debug(CATEGORIES.SYSTEM, 'MarketObservationSkipped_UnknownCity', {mapId: window.currentMapId});
            return;
        }

        // Parameters[0] is an array of JSON-encoded strings, not nested objects - each
        // listing needs its own JSON.parse (confirmed via the reference capture).
        const listings = [];
        for (const entry of raw) {
            try {
                listings.push(JSON.parse(entry));
            } catch (error) {
                window.logger?.debug(CATEGORIES.SYSTEM, 'MarketListingParseFailed', {error: error?.message || error});
            }
        }
        if (listings.length === 0) return;

        const groups = groupByItemAndQuality(listings);
        if (groups.length === 0) return;

        const now = nowAsPriceDate();
        const entries = groups.map(({itemId, quality, prices}) => {
            const min = Math.min(...prices);
            const max = Math.max(...prices);
            const entry = {item_id: itemId, city, quality};
            if (side === 'sell') {
                entry.sell_price_min = min;
                entry.sell_price_min_date = now;
                entry.sell_price_max = max;
                entry.sell_price_max_date = now;
            } else {
                entry.buy_price_min = min;
                entry.buy_price_min_date = now;
                entry.buy_price_max = max;
                entry.buy_price_max_date = now;
            }
            return entry;
        });

        this._submit(side, entries);
    }

    // Best-effort, fire-and-forget - mirrors ZoneGraph.reportTransition's fetch guard (try/catch
    // around the call itself, since fetch can throw synchronously in addition to rejecting).
    _submit(side, entries) {
        try {
            Promise.resolve(
                fetch('/api/market/observations', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({side, entries}),
                })
            ).catch((error) => {
                window.logger?.debug(CATEGORIES.SYSTEM, 'MarketObservationSubmitFailed', {error: error?.message || error});
            });
        } catch (error) {
            window.logger?.debug(CATEGORIES.SYSTEM, 'MarketObservationSubmitFailed', {error: error?.message || error});
        }
    }
}

const marketHandler = new MarketHandler();
export default marketHandler;
