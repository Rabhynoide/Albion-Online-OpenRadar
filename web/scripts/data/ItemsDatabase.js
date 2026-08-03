/**
 * Items Database
 * Parses items.min.json and provides item lookup by sequential ID
 *
 * Minified format: [{ n: "uniquename", p: itempower, cat: "category" }, ...]
 * Index in array = sequential ID (0-based, add 1 for game ID)
 */

import {CATEGORIES} from '../constants/LoggerConstants.js';

export class ItemsDatabase {
    constructor() {
        /** @type {Map<number, {name: string, tier: number, itempower: number, enchant: number, category: string|null, subcategory: string|null}>} */
        this.items = new Map();
        this.isLoaded = false;
    }

    /**
     * Load and parse items.min.json
     * @param {string} jsonPath - Path to items.min.json file
     */
    async load(jsonPath) {
        try {
            window.logger?.info(CATEGORIES.SYSTEM, 'ItemsLoading', {path: jsonPath});

            const response = await fetch(jsonPath, {cache: 'no-cache'});
            if (!response.ok) {
                throw new Error(`Failed to fetch items.min.json: ${response.status}`);
            }

            const items = await response.json();

            if (!Array.isArray(items)) {
                throw new Error('Invalid items.min.json structure: expected array');
            }

            // Items are pre-filtered (itempower > 0) and sequential
            // Index 0 = game ID 1, Index 1 = game ID 2, etc.
            for (let i = 0; i < items.length; i++) {
                const item = items[i];
                const id = i + 1; // Game IDs start at 1

                // Parse enchant from name (e.g., "T4_2H_SWORD@2" -> enchant 2)
                let name = item.n;
                let enchant = 0;
                const atIndex = name.lastIndexOf('@');
                if (atIndex > 0) {
                    enchant = parseInt(name.substring(atIndex + 1)) || 0;
                }

                this.items.set(id, {
                    name: name,
                    tier: this._extractTier(name),
                    itempower: item.p,
                    enchant: enchant,
                    category: item.cat || null,
                    subcategory: item.sub || null
                });
            }

            this.isLoaded = true;
            window.logger?.info(CATEGORIES.SYSTEM, 'ItemsLoaded', {count: this.items.size});

        } catch (error) {
            window.logger?.error(CATEGORIES.SYSTEM, 'ItemsLoadError', {error: error.message});
            throw error;
        }
    }

    /**
     * Get item by sequential ID
     * @param {number} id - Sequential item ID (1, 2, 3...)
     * @returns {{name: string, tier: number, itempower: number, enchant: number} | undefined}
     */
    getItemById(id) {
        return this.items.get(id);
    }

    /**
     * Case-insensitive substring search over item UniqueNames (there is no localized
     * display-name link in this data - "SWORD" matches "T4_MAIN_SWORD", "T5_2H_SWORD", etc).
     * @param {string} query
     * @param {number} [limit=20]
     * @returns {Array<{id: number, name: string, tier: number, itempower: number, enchant: number}>}
     */
    searchByName(query, limit = 20) {
        if (!query) return [];
        const needle = query.toLowerCase();
        const results = [];
        for (const [id, item] of this.items) {
            if (item.name.toLowerCase().includes(needle)) {
                results.push({id, ...item});
                if (results.length >= limit) break;
            }
        }
        return results;
    }

    /**
     * Case-insensitive substring search combined with optional category/subcategory/tier/enchant
     * filters (issue: market filters - mirrors the in-game marketplace's own cascading
     * Category -> Sub-type browse controls, plus Niveau/Enchantement). Any filter left at its
     * default (empty string) is not applied - matching the in-game "All" option. `query` may be
     * empty when filtering by category/tier/enchant alone, unlike `searchByName` which requires
     * text.
     * @param {{query?: string, category?: string, subcategory?: string, tier?: number|string, enchant?: number|string}} filters
     * @param {number} [limit=20]
     * @returns {Array<{id: number, name: string, tier: number, itempower: number, enchant: number, category: string|null, subcategory: string|null}>}
     */
    searchItems({query = '', category = '', subcategory = '', tier = '', enchant = ''} = {}, limit = 20) {
        const needle = query.toLowerCase();
        const wantTier = tier === '' ? null : Number(tier);
        const wantEnchant = enchant === '' ? null : Number(enchant);
        const results = [];
        for (const [id, item] of this.items) {
            if (needle && !item.name.toLowerCase().includes(needle)) continue;
            if (category && item.category !== category) continue;
            if (subcategory && item.subcategory !== subcategory) continue;
            if (wantTier !== null && item.tier !== wantTier) continue;
            if (wantEnchant !== null && item.enchant !== wantEnchant) continue;
            results.push({id, ...item});
            if (results.length >= limit) break;
        }
        return results;
    }

    /**
     * Extract tier from item uniquename (e.g., "T4_2H_SWORD" → 4)
     * @param {string} uniqueName
     * @returns {number}
     * @private
     */
    _extractTier(uniqueName) {
        const match = uniqueName.match(/^T(\d+)_/);
        return match ? parseInt(match[1]) : 0;
    }
}
