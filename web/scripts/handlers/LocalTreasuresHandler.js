import {CATEGORIES} from "../constants/LoggerConstants.js";
import settingsSync from "../utils/SettingsSync.js";

class Treasure {
    constructor(id, posX, posY, label, startTicks, endTicks) {
        this.id = id;
        this.posX = posX;
        this.posY = posY;
        this.label = label;
        this.startTicks = startTicks;
        this.endTicks = endTicks;
        this.hX = 0;
        this.hY = 0;
        this.lastUpdateTime = Date.now();
    }

    touch() {
        this.lastUpdateTime = Date.now();
    }
}

// LocalTreasuresUpdate delivers a full resync of every active local treasure in the zone
// (buried treasure chests, temporary rich resource nodes, smuggler piles, timed special/
// anniversary events) as parallel arrays, unlike every other detection type in this codebase
// which is one event per entity. Removal still arrives individually via the normal Leave event.
//
// "SPECIAL_EVENT_*" labels are drawn UNLESS a mob with the same id is currently tracked by
// MobsHandler: pcap-confirmed (2026-07-30 live capture) that a SPECIAL_EVENT_1 entry can share
// its entity id with a real NewMob (MOB_EVENT_LEAD_UP_SPEARMAN_T7) - drawing it here would
// duplicate an encounter already shown, with better threat info, by the existing mob detection.
// A blanket label-prefix exclusion originally shipped for this, but issue #164/#163 pcap-
// confirmed (2026-08-08) a second SPECIAL_EVENT_1 case - a buried-treasure decor with no
// matching mob id anywhere in the capture (not as NewMob, not even as a Leave) - that the
// blanket exclusion silently hid from the radar. The wire label alone doesn't distinguish the
// two cases; only a live id cross-check against the mob list does. ANNIVERSARY was checked
// against the original capture and has no matching mob id, so it was never affected either way.
const EXCLUDED_LABEL_PREFIXES = ['SPECIAL_EVENT'];

function isDrawableLabel(label, id, mobsHandler) {
    if (typeof label !== 'string') return false;
    if (!EXCLUDED_LABEL_PREFIXES.some(prefix => label.startsWith(prefix))) return true;
    return !mobsHandler?.hasMob(id);
}

export class LocalTreasuresHandler {
    constructor() {
        this.treasuresList = [];
    }

    handleLocalTreasuresUpdate(Parameters, mobsHandler) {
        if (!settingsSync.getBool('settingLocalTreasures')) return;

        const toArray = (v) => (v === undefined || v === null) ? [] : (Array.isArray(v) ? v : [v]);
        const ids = toArray(Parameters[4]);
        const positions = toArray(Parameters[5]);
        const startTicks = toArray(Parameters[6]);
        const endTicks = toArray(Parameters[7]);
        const labels = toArray(Parameters[8]);

        ids.forEach((id, i) => {
            const label = labels[i];
            if (!isDrawableLabel(label, id, mobsHandler)) return;
            const posX = positions[i * 2];
            const posY = positions[i * 2 + 1];
            if (posX === undefined || posY === undefined) return;
            this.addTreasure(id, posX, posY, label, startTicks[i], endTicks[i]);
        });
    }

    addTreasure(id, posX, posY, label, startTicks, endTicks) {
        const existing = this.treasuresList.find(treasure => treasure.id === id);
        if (existing) {
            existing.touch();
            return;
        }
        this.treasuresList.push(new Treasure(id, posX, posY, label, startTicks, endTicks));
    }

    removeTreasure(id) {
        this.treasuresList = this.treasuresList.filter(treasure => treasure.id !== id);
    }

    Clear() {
        this.treasuresList = [];
    }

    cleanupStaleEntities(maxAgeMs = 120000) {
        const now = Date.now();
        const before = this.treasuresList.length;
        this.treasuresList = this.treasuresList.filter(treasure =>
            (now - treasure.lastUpdateTime) < maxAgeMs
        );
        const removed = before - this.treasuresList.length;
        if (removed > 0) {
            window.logger?.debug(CATEGORIES.DUNGEONS, 'local_treasure_cleanup', {removed, maxAgeMs});
        }
        return removed;
    }
}
