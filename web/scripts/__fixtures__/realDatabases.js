import {existsSync, readFileSync} from 'node:fs';
import {gunzipSync} from 'node:zlib';
import {fileURLToPath} from 'node:url';
import {dirname, join} from 'node:path';

import {HarvestablesDatabase} from '../data/HarvestablesDatabase.js';
import {MobsDatabase} from '../data/MobsDatabase.js';
import zonesDatabase, {ZonesDatabase} from '../data/ZonesDatabase.js';

const here = dirname(fileURLToPath(import.meta.url));
const dumps = join(here, '..', '..', 'ao-bin-dumps');

// Some ao-bin-dumps files only ship their pre-compressed .gz sibling committed (no plain copy -
// see tools/compress-game-data.ts), same reasoning as internal/server/http.go's readAsset()
// fallback: fall back to gunzipping it so tests reading straight off disk (bypassing the Go
// server's own .gz handling) still find the data.
export function readAoBinDumpJSON(absolutePath) {
    if (existsSync(absolutePath)) {
        return JSON.parse(readFileSync(absolutePath, 'utf8'));
    }
    const gzipped = readFileSync(absolutePath + '.gz');
    return JSON.parse(gunzipSync(gzipped).toString('utf8'));
}

function readJSON(name) {
    return readAoBinDumpJSON(join(dumps, name));
}

export function loadRealHarvestablesDatabase() {
    const db = new HarvestablesDatabase();
    db._parseHarvestables(readJSON('harvestables.min.json'));
    db.isLoaded = true;
    return db;
}

export function loadRealMobsDatabase() {
    const db = new MobsDatabase();
    db._parseMobs(readJSON('mobs.min.json'));
    db.isLoaded = true;
    return db;
}

export function loadRealZonesDatabase() {
    const db = new ZonesDatabase();
    db.zones = readJSON('zones.json');
    db.loaded = true;
    return db;
}

export function installRealDatabasesOnWindow() {
    window.harvestablesDatabase = loadRealHarvestablesDatabase();
    window.mobsDatabase = loadRealMobsDatabase();
    return {
        harvestablesDatabase: window.harvestablesDatabase,
        mobsDatabase: window.mobsDatabase,
    };
}

export {zonesDatabase as defaultZonesDatabase};
