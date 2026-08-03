/**
 * Generates web/ao-bin-dumps/shopcategories.json - the localized category/subcategory
 * taxonomy that powers the Market page's cascading Category -> Sub-type filter.
 *
 * Source of truth: items.json's own `items.shopcategories` tree (the same file
 * update-ao-data.ts already pulls for item data - it separately carries the full shop
 * category/subcategory hierarchy with the exact @id values every item's own
 * @shopcategory/@shopsubcategory1 attribute references). Labels come from localization.json's
 * @MARKETPLACEGUI_ROLLOUT_SHOPCATEGORY_<ID>/SHOPSUBCATEGORY_<ID> tuids (EN-US) - confirmed by
 * grepping a real downloaded copy for known screen text (e.g. "Cloth Robes"), not guessed.
 *
 * Run via `npx tsx tools/generate-shop-categories.ts`. Not part of the regular
 * update-ao-data/refresh-assets pipeline (localization.json is ~90MB and this taxonomy changes
 * far less often than item stats) - re-run by hand only when a game patch adds/renames a shop
 * category.
 */
import fs from 'fs';
import path from 'path';
import {downloadFile} from './common';

const GITHUB_RAW_BASE = 'https://raw.githubusercontent.com/ao-data/ao-bin-dumps/refs/heads/master';
const OUTPUT_PATH = path.join('web/ao-bin-dumps', 'shopcategories.json');

interface ShopCategoryOut {
    label: string;
    subcategories: Record<string, string>;
}

function tuidLabel(localizationText: string, tuid: string): string | null {
    // Structural search rather than a full JSON.parse of a ~90MB file: find the tuid's own
    // "tu" block, then the EN-US "seg" inside it. Reliable because every tu block belongs to
    // exactly one tuid and EN-US always appears in this dataset.
    const tuidIndex = localizationText.indexOf(`"@tuid": "${tuid}"`);
    if (tuidIndex === -1) return null;
    const blockEnd = localizationText.indexOf('"@tuid"', tuidIndex + 1);
    const block = localizationText.slice(tuidIndex, blockEnd === -1 ? undefined : blockEnd);
    const match = block.match(/"@xml:lang": "EN-US",\s*\n\s*"seg": "([^"]*)"/);
    return match ? match[1] : null;
}

async function main() {
    console.log('📥 Downloading items.json (for the shopcategories tree)...');
    const itemsRes = await downloadFile(`${GITHUB_RAW_BASE}/items.json`);
    if (itemsRes.status !== 'success' || !itemsRes.buffer) {
        console.error('❌ Failed to download items.json:', itemsRes.message);
        process.exit(1);
    }
    const itemsData = JSON.parse(itemsRes.buffer.toString('utf-8'));
    const rawCategories = itemsData?.items?.shopcategories?.shopcategory;
    if (!rawCategories) {
        console.error('❌ items.json has no items.shopcategories.shopcategory tree');
        process.exit(1);
    }
    const categories = Array.isArray(rawCategories) ? rawCategories : [rawCategories];

    console.log('📥 Downloading localization.json (~90MB, this may take a moment)...');
    const locRes = await downloadFile(`${GITHUB_RAW_BASE}/localization.json`);
    if (locRes.status !== 'success' || !locRes.buffer) {
        console.error('❌ Failed to download localization.json:', locRes.message);
        process.exit(1);
    }
    const localizationText = locRes.buffer.toString('utf-8');

    const out: Record<string, ShopCategoryOut> = {};
    let missingLabels = 0;

    for (const cat of categories) {
        const catId: string | undefined = cat['@id'];
        if (!catId) continue;

        const catLabel = tuidLabel(localizationText, `@MARKETPLACEGUI_ROLLOUT_SHOPCATEGORY_${catId.toUpperCase()}`);
        if (!catLabel) {
            missingLabels++;
            console.warn(`⚠️  No EN-US label for category "${catId}"`);
        }

        const rawSub = cat.shopsubcategory;
        const subs = rawSub ? (Array.isArray(rawSub) ? rawSub : [rawSub]) : [];
        const subcategories: Record<string, string> = {};
        for (const sub of subs) {
            const subId: string | undefined = sub['@id'];
            if (!subId) continue;
            const subLabel = tuidLabel(localizationText, `@MARKETPLACEGUI_ROLLOUT_SHOPSUBCATEGORY_${subId.toUpperCase()}`);
            if (!subLabel) {
                missingLabels++;
                console.warn(`⚠️  No EN-US label for subcategory "${subId}" (category "${catId}")`);
                continue;
            }
            subcategories[subId] = subLabel;
        }

        out[catId] = {label: catLabel || catId, subcategories};
    }

    fs.writeFileSync(OUTPUT_PATH, JSON.stringify(out, null, 2) + '\n', 'utf-8');
    console.log(`✅ Wrote ${OUTPUT_PATH} (${Object.keys(out).length} categories, ${missingLabels} labels missing)`);
}

main().catch(err => {
    console.error('❌ Unexpected error:', err);
    process.exit(1);
});
