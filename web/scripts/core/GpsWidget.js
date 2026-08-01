// GpsWidget - persistent sidebar GPS: destination search/set/clear works from any page
// (it's just a settingsSync value), but the live "next hop" route can only be computed
// while the Radar page's WebSocket/EventRouter/ZoneGraph are actually running - navigating
// away tears all of that down (see Utils.js destroyRadar). So this module always renders
// the destination, and shows the live route only while startLiveGps() is active.
import zonesDatabase from '../data/ZonesDatabase.js';
import settingsSync from '../utils/SettingsSync.js';
import {CATEGORIES} from '../constants/LoggerConstants.js';

// GPS: label carries the zone id in parens ("Bridgewatch (0002)") so free-text search
// against a plain <datalist> (no extra JS dependency) can be resolved back to an id.
export function gpsZoneLabel(id, zone) {
    return `${zone.name} (${id})`;
}

export function gpsParseZoneId(label) {
    const match = /\(([^()]+)\)\s*$/.exec((label || '').trim());
    return match ? match[1] : null;
}

// Roads of Avalon (TUNNEL_ROYAL, TUNNEL_BLACK_*, TUNNEL_DEEP, TUNNEL_HIGH/MEDIUM/LOW...)
// reset in-game and carry zero static connectivity data (confirmed against real game data:
// the extracted zone-graph.json has no edges at all touching a TNL- zone) - every hop through
// one has to be learned live. TUNNEL_HIDEOUT* is excluded: player hideouts, not roads.
export function gpsIsOnAvalonRoad(zoneType) {
    const t = (zoneType || '').toUpperCase();
    return t.includes('TUNNEL') && !t.includes('HIDEOUT');
}

// Same palette as RadarRenderer.js's renderZoneInfo (the on-canvas zone-name box), so a zone's
// color means the same thing everywhere in the app.
const PVP_COLORS = {
    black: '#ff4444',
    red: '#ff8800',
    yellow: '#ffff00',
    safe: '#44ff44',
};

function pvpColor(zoneId) {
    const pvpType = zonesDatabase?.getPvpType ? zonesDatabase.getPvpType(zoneId) : 'safe';
    return PVP_COLORS[pvpType] || PVP_COLORS.safe;
}

let elements = null;
let liveContext = null; // set by startLiveGps(); null means "show a static, non-live message"
// The (from,to) of the edge currently shown as "Next: ..." - the only edge the "remove this
// route" button is allowed to touch, so a click always matches what's on screen.
let removableEdge = null;

function getElements() {
    if (elements) return elements;
    const input = document.getElementById('gpsDestinationInput');
    const optionsList = document.getElementById('gpsZoneOptions');
    const setBtn = document.getElementById('gpsSetBtn');
    const clearBtn = document.getElementById('gpsClearBtn');
    const resultEl = document.getElementById('gpsResult');
    const routeDotsEl = document.getElementById('gpsRouteDots');
    const removeRouteBtn = document.getElementById('gpsRemoveRouteBtn');
    if (!input || !optionsList || !setBtn || !clearBtn || !resultEl || !routeDotsEl || !removeRouteBtn) return null;
    elements = {input, optionsList, setBtn, clearBtn, resultEl, routeDotsEl, removeRouteBtn};
    return elements;
}

// Hides the "remove this route" button and forgets which edge it would have removed -
// called from every renderResult() branch that isn't showing a concrete next-hop edge.
function hideRemoveRouteButton(els) {
    removableEdge = null;
    els.removeRouteBtn.style.display = 'none';
}

function clearRouteDots(routeDotsEl) {
    routeDotsEl.replaceChildren();
}

// One dot per zone in the route (inclusive of both ends), colored by that zone's danger
// level - same idea as EVE Online's route bar, adapted to Albion's 4 discrete pvp tiers
// instead of a continuous security value.
//
// Geometry is set inline rather than via Tailwind utility classes: these dots are created
// from JS, not written literally in a .gohtml file, so they only get picked up by Tailwind's
// content scan (@source "../scripts/**/*.js" in input.css) if the CSS bundle happens to be
// rebuilt after this code changes - inline styles sidestep that build-ordering trap entirely.
function renderRouteDots(routeDotsEl, path) {
    const fragment = document.createDocumentFragment();
    for (const zoneId of path) {
        const dot = document.createElement('span');
        dot.style.cssText = 'display:inline-block; width:10px; height:10px; border-radius:50%; flex-shrink:0;';
        dot.style.backgroundColor = pvpColor(zoneId);
        dot.title = zonesDatabase?.getZoneName ? zonesDatabase.getZoneName(zoneId) : zoneId;
        fragment.appendChild(dot);
    }
    routeDotsEl.replaceChildren(fragment);
}

function populateZoneOptions(optionsList) {
    if (!zonesDatabase?.zones) return;
    const fragment = document.createDocumentFragment();
    for (const [id, zone] of Object.entries(zonesDatabase.zones)) {
        const opt = document.createElement('option');
        opt.value = gpsZoneLabel(id, zone);
        fragment.appendChild(opt);
    }
    optionsList.appendChild(fragment);
}

function renderResult() {
    const els = getElements();
    if (!els) return;
    const dest = settingsSync.getJSON('gpsDestination', null);

    if (!dest?.id) {
        els.resultEl.textContent = 'No destination set.';
        clearRouteDots(els.routeDotsEl);
        hideRemoveRouteButton(els);
        return;
    }

    if (!liveContext) {
        els.resultEl.textContent = `Destination: ${dest.name}. Open the Radar page to see the live route.`;
        clearRouteDots(els.routeDotsEl);
        hideRemoveRouteButton(els);
        return;
    }

    const {zoneGraph, getLocalPlayerPosition, relativeScreenBearing, bearingToCompassLabel} = liveContext;
    const currentZoneId = window.currentMapId;
    if (currentZoneId === undefined || currentZoneId === null || currentZoneId === -1) {
        els.resultEl.textContent = `Destination: ${dest.name}. Waiting for zone data...`;
        clearRouteDots(els.routeDotsEl);
        hideRemoveRouteButton(els);
        return;
    }
    if (String(currentZoneId) === String(dest.id)) {
        els.resultEl.textContent = `You are in ${dest.name}.`;
        clearRouteDots(els.routeDotsEl);
        hideRemoveRouteButton(els);
        return;
    }

    const hop = zoneGraph.getNextHop(String(currentZoneId), String(dest.id));
    if (!hop) {
        const currentType = zonesDatabase?.getZoneType ? zonesDatabase.getZoneType(currentZoneId) : '';
        els.resultEl.textContent = gpsIsOnAvalonRoad(currentType)
            ? `Destination: ${dest.name}. You're on an unexplored Avalonian Road - the GPS learns each hop as you walk it, so this route isn't known yet. Keep going and it'll pick up from here next time.`
            : `Destination: ${dest.name}. No known route from here yet.`;
        clearRouteDots(els.routeDotsEl);
        hideRemoveRouteButton(els);
        return;
    }

    const nextZoneName = zonesDatabase?.getZoneName ? zonesDatabase.getZoneName(hop.nextZoneId) : hop.nextZoneId;
    let via = '';
    if (hop.viaPos) {
        const player = getLocalPlayerPosition();
        const bearing = relativeScreenBearing(hop.viaPos[0] - player.x, hop.viaPos[1] - player.y);
        via = ` (exit ${bearingToCompassLabel(bearing)})`;
    }
    // "assumed" = we never actually confirmed this direction is passable, only inferred
    // it as the reverse of a road you walked in - flag it distinctly from "stale" (an
    // observation that's simply old) so the note is honest about which kind of guess it is.
    let note = '';
    if (hop.assumed) {
        note = ' - includes an unconfirmed U-turn (we know you got here, not that you can go back)';
    } else if (hop.stale) {
        note = ' - route may be outdated (based on an older discovered road)';
    }

    const nextZoneSpan = document.createElement('span');
    nextZoneSpan.textContent = nextZoneName;
    nextZoneSpan.style.color = pvpColor(hop.nextZoneId);
    nextZoneSpan.style.fontWeight = '600';
    els.resultEl.replaceChildren('Next: ', nextZoneSpan, `${via} - ${hop.hops} hop(s)${note}`);

    const fullPath = zoneGraph.getFullPath(String(currentZoneId), String(dest.id));
    if (fullPath?.path?.length > 1) {
        renderRouteDots(els.routeDotsEl, fullPath.path);
    } else {
        clearRouteDots(els.routeDotsEl);
    }

    // Issue #5: a discovered road can reset/change before the 24h staleness window catches
    // up - let the player flag "this exit doesn't exist anymore" instead of waiting it out.
    // removeEdge() itself no-ops on static (non-discovered) edges, so this is safe to offer
    // even when we can't tell from here whether the hop came from the static graph or not.
    removableEdge = {from: String(currentZoneId), to: hop.nextZoneId};
    els.removeRouteBtn.style.display = '';
}

function setDestination() {
    const els = getElements();
    if (!els) return;
    const id = gpsParseZoneId(els.input.value);
    if (!id || !zonesDatabase?.getZone(id)) {
        els.resultEl.textContent = 'Unknown zone - pick one from the suggestions list.';
        clearRouteDots(els.routeDotsEl);
        hideRemoveRouteButton(els);
        return;
    }
    settingsSync.setJSON('gpsDestination', {id, name: zonesDatabase.getZoneName(id)});
    renderResult();
}

function clearDestination() {
    const els = getElements();
    if (!els) return;
    settingsSync.setJSON('gpsDestination', null);
    els.input.value = '';
    renderResult();
}

// Removes exactly the edge currently shown as "Next: ..." (issue #5). removableEdge is only
// ever set by renderResult()'s live-route branch, so this can't act on stale UI state - if
// the route changed since the button was drawn, the next render already updated it.
function removeCurrentRoute() {
    if (!removableEdge || !liveContext) return;
    liveContext.zoneGraph.removeEdge(removableEdge.from, removableEdge.to);
    renderResult();
}

let initialized = false;

// Wires the persistent sidebar GPS widget once at app boot: works on every page (destination
// is just a settingsSync value), independent of whether the Radar page has ever been visited.
export async function initGpsWidget() {
    if (initialized) return;
    const els = getElements();
    if (!els) return;
    initialized = true;

    if (!zonesDatabase.loaded) {
        try {
            await zonesDatabase.load();
        } catch (err) {
            window.logger?.warn(CATEGORIES.SYSTEM, 'GpsWidgetZonesLoadFailed', {error: err?.message});
        }
    }
    populateZoneOptions(els.optionsList);

    els.setBtn.addEventListener('click', setDestination);
    els.clearBtn.addEventListener('click', clearDestination);
    els.removeRouteBtn.addEventListener('click', removeCurrentRoute);

    const savedDest = settingsSync.getJSON('gpsDestination', null);
    if (savedDest?.name) els.input.value = gpsZoneLabel(savedDest.id, {name: savedDest.name});

    renderResult();
}

// Switches the widget into "live" mode: called from the Radar page's init (after
// zoneGraph.load()), refreshes on every zone change and once a second (to pick up compass
// bearing changes as the player walks). Returns a cleanup function that reverts to the
// static, non-live message - call it from the Radar page's destroy.
export function startLiveGps(zoneGraph, onMapChange, getLocalPlayerPosition, relativeScreenBearing, bearingToCompassLabel) {
    liveContext = {zoneGraph, getLocalPlayerPosition, relativeScreenBearing, bearingToCompassLabel};
    onMapChange(renderResult);
    renderResult();
    const refreshIntervalId = setInterval(renderResult, 1000);

    return () => {
        liveContext = null;
        clearInterval(refreshIntervalId);
        renderResult();
    };
}
