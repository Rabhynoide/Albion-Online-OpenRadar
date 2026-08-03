import {CATEGORIES} from '../constants/LoggerConstants.js';

// Thin fetch wrapper for the radar backend's /api/market/prices endpoint. The backend decides
// Hub-vs-direct-public-API itself (see internal/server/market_api.go) - this module only
// builds the query string and parses the response, so the market page doesn't need to know
// anything about that split.
export async function fetchPrices(itemIds, cities = [], qualities = []) {
    if (!itemIds || itemIds.length === 0) return [];

    const params = new URLSearchParams();
    params.set('items', itemIds.join(','));
    if (cities.length > 0) params.set('locations', cities.join(','));
    if (qualities.length > 0) params.set('qualities', qualities.join(','));

    try {
        const response = await fetch(`/api/market/prices?${params.toString()}`);
        if (!response.ok) {
            window.logger?.warn(CATEGORIES.SYSTEM, 'MarketPricesFetchFailed', {status: response.status});
            return [];
        }
        return await response.json();
    } catch (error) {
        window.logger?.warn(CATEGORIES.SYSTEM, 'MarketPricesFetchError', {error: error?.message || error});
        return [];
    }
}
