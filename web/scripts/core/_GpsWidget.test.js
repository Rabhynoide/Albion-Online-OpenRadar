import {describe, test, expect, beforeEach, afterEach, vi} from 'vitest';

vi.mock('../utils/SettingsSync.js', () => ({
    default: {
        getJSON: vi.fn(() => null),
        setJSON: vi.fn(),
    },
}));

vi.mock('../data/ZonesDatabase.js', () => ({
    default: {
        zones: {
            '0001': {name: 'Lymhurst'},
            '0002': {name: 'Bridgewatch'},
        },
        loaded: true,
        load: vi.fn(),
        getZone: vi.fn((id) => (id === '0001' || id === '0002') ? {} : null),
        getZoneName: vi.fn((id) => ({'0001': 'Lymhurst', '0002': 'Bridgewatch'})[id] ?? id),
        getZoneType: vi.fn(() => ''),
        getPvpType: vi.fn(() => 'safe'),
    },
}));

function setupDom() {
    document.body.innerHTML = `
        <input id="gpsDestinationInput" />
        <datalist id="gpsZoneOptions"></datalist>
        <button id="gpsSetBtn"></button>
        <button id="gpsClearBtn"></button>
        <div id="gpsResult"></div>
        <div id="gpsRouteDots"></div>
    `;
}

describe('GpsWidget', () => {
    let settingsSync, zonesDatabase, GpsWidget;

    beforeEach(async () => {
        vi.resetModules();
        vi.clearAllMocks();
        setupDom();
        window.logger = {warn: vi.fn()};
        window.currentMapId = null;

        settingsSync = (await import('../utils/SettingsSync.js')).default;
        zonesDatabase = (await import('../data/ZonesDatabase.js')).default;
        settingsSync.getJSON.mockReturnValue(null);
        zonesDatabase.loaded = true;

        GpsWidget = await import('./GpsWidget.js');
    });

    afterEach(() => {
        vi.useRealTimers();
    });

    describe('gpsZoneLabel / gpsParseZoneId', () => {
        test('round-trips a zone id through the label', () => {
            const label = GpsWidget.gpsZoneLabel('0002', {name: 'Bridgewatch'});
            expect(label).toBe('Bridgewatch (0002)');
            expect(GpsWidget.gpsParseZoneId(label)).toBe('0002');
        });

        test('gpsParseZoneId returns null when no id is present', () => {
            expect(GpsWidget.gpsParseZoneId('no id here')).toBeNull();
            expect(GpsWidget.gpsParseZoneId('')).toBeNull();
            expect(GpsWidget.gpsParseZoneId(null)).toBeNull();
        });
    });

    describe('gpsIsOnAvalonRoad', () => {
        test.each([
            ['TUNNEL_ROYAL', true],
            ['TUNNEL_BLACK_HIGH', true],
            ['tunnel_deep', true],
            ['TUNNEL_HIDEOUT', false],
            ['SAFEAREA', false],
            ['', false],
            [undefined, false],
        ])('%s -> %s', (type, expected) => {
            expect(GpsWidget.gpsIsOnAvalonRoad(type)).toBe(expected);
        });
    });

    describe('initGpsWidget', () => {
        test('populates the zone datalist from zonesDatabase', async () => {
            await GpsWidget.initGpsWidget();
            const options = document.getElementById('gpsZoneOptions').querySelectorAll('option');
            expect(options).toHaveLength(2);
        });

        test('shows a neutral message when no destination is saved', async () => {
            await GpsWidget.initGpsWidget();
            expect(document.getElementById('gpsResult').textContent).toBe('No destination set.');
        });

        test('restores a saved destination into the input, without a live route yet', async () => {
            settingsSync.getJSON.mockReturnValue({id: '0002', name: 'Bridgewatch'});
            await GpsWidget.initGpsWidget();
            expect(document.getElementById('gpsDestinationInput').value).toBe('Bridgewatch (0002)');
            expect(document.getElementById('gpsResult').textContent)
                .toBe('Destination: Bridgewatch. Open the Radar page to see the live route.');
        });

        test('setDestination persists a valid selection', async () => {
            await GpsWidget.initGpsWidget();
            document.getElementById('gpsDestinationInput').value = 'Bridgewatch (0002)';
            document.getElementById('gpsSetBtn').click();
            expect(settingsSync.setJSON).toHaveBeenCalledWith('gpsDestination', {id: '0002', name: 'Bridgewatch'});
        });

        test('setDestination rejects an unknown zone without persisting', async () => {
            await GpsWidget.initGpsWidget();
            document.getElementById('gpsDestinationInput').value = 'Nowhere (9999)';
            document.getElementById('gpsSetBtn').click();
            expect(settingsSync.setJSON).not.toHaveBeenCalled();
            expect(document.getElementById('gpsResult').textContent).toBe('Unknown zone - pick one from the suggestions list.');
        });

        test('clearDestination clears the saved value and the input', async () => {
            settingsSync.getJSON.mockReturnValue({id: '0002', name: 'Bridgewatch'});
            await GpsWidget.initGpsWidget();
            document.getElementById('gpsClearBtn').click();
            expect(settingsSync.setJSON).toHaveBeenCalledWith('gpsDestination', null);
            expect(document.getElementById('gpsDestinationInput').value).toBe('');
        });

        test('is a no-op on a second call (no duplicate options or listeners)', async () => {
            await GpsWidget.initGpsWidget();
            await GpsWidget.initGpsWidget();
            const options = document.getElementById('gpsZoneOptions').querySelectorAll('option');
            expect(options).toHaveLength(2);

            document.getElementById('gpsDestinationInput').value = 'Bridgewatch (0002)';
            document.getElementById('gpsSetBtn').click();
            expect(settingsSync.setJSON).toHaveBeenCalledTimes(1);
        });
    });

    describe('startLiveGps', () => {
        test('computes the live route while active, and reverts to the static message on cleanup', async () => {
            settingsSync.getJSON.mockReturnValue({id: '0002', name: 'Bridgewatch'});
            await GpsWidget.initGpsWidget();

            window.currentMapId = '0001';
            const zoneGraph = {
                getNextHop: vi.fn(() => ({nextZoneId: '0002', hops: 1, viaPos: null})),
                getFullPath: vi.fn(() => ({path: ['0001', '0002'], stale: false, assumed: false})),
            };
            const onMapChange = vi.fn();

            const cleanup = GpsWidget.startLiveGps(zoneGraph, onMapChange, vi.fn(() => ({x: 0, y: 0})), vi.fn(), vi.fn());

            const resultEl = document.getElementById('gpsResult');
            expect(resultEl.textContent).toBe('Next: Bridgewatch - 1 hop(s)');
            expect(resultEl.textContent).not.toContain('to Bridgewatch');
            expect(onMapChange).toHaveBeenCalledWith(expect.any(Function));

            cleanup();
            expect(document.getElementById('gpsResult').textContent)
                .toBe('Destination: Bridgewatch. Open the Radar page to see the live route.');
        });

        test('colors the next zone name by its pvp danger level', async () => {
            settingsSync.getJSON.mockReturnValue({id: '0002', name: 'Bridgewatch'});
            await GpsWidget.initGpsWidget();

            window.currentMapId = '0001';
            zonesDatabase.getPvpType.mockReturnValue('red');
            const zoneGraph = {
                getNextHop: vi.fn(() => ({nextZoneId: '0002', hops: 1, viaPos: null})),
                getFullPath: vi.fn(() => ({path: ['0001', '0002'], stale: false, assumed: false})),
            };

            const cleanup = GpsWidget.startLiveGps(zoneGraph, vi.fn(), vi.fn(() => ({x: 0, y: 0})), vi.fn(), vi.fn());

            const zoneSpan = document.querySelector('#gpsResult span');
            expect(zoneSpan.textContent).toBe('Bridgewatch');
            expect(zoneSpan.style.color).toBe('#ff8800'); // red pvp
            cleanup();
        });

        test('renders one route dot per zone in the full path, colored by pvp danger', async () => {
            settingsSync.getJSON.mockReturnValue({id: '0002', name: 'Bridgewatch'});
            await GpsWidget.initGpsWidget();

            window.currentMapId = '0001';
            zonesDatabase.getPvpType.mockImplementation((id) => id === '0001' ? 'safe' : 'yellow');
            const zoneGraph = {
                getNextHop: vi.fn(() => ({nextZoneId: '0002', hops: 1, viaPos: null})),
                getFullPath: vi.fn(() => ({path: ['0001', '0002'], stale: false, assumed: false})),
            };

            const cleanup = GpsWidget.startLiveGps(zoneGraph, vi.fn(), vi.fn(() => ({x: 0, y: 0})), vi.fn(), vi.fn());

            const dots = document.querySelectorAll('#gpsRouteDots > span');
            expect(dots).toHaveLength(2);
            expect(dots[0].style.backgroundColor).toBe('#44ff44'); // safe
            expect(dots[1].style.backgroundColor).toBe('#ffff00'); // yellow
            expect(dots[0].title).toBe('Lymhurst');
            expect(dots[1].title).toBe('Bridgewatch');

            cleanup();
            expect(document.querySelectorAll('#gpsRouteDots > span')).toHaveLength(0);
        });

        test('refreshes once a second while live', async () => {
            vi.useFakeTimers();
            settingsSync.getJSON.mockReturnValue({id: '0002', name: 'Bridgewatch'});
            await GpsWidget.initGpsWidget();

            window.currentMapId = '0002';
            const zoneGraph = {getNextHop: vi.fn()};
            const cleanup = GpsWidget.startLiveGps(zoneGraph, vi.fn(), vi.fn(), vi.fn(), vi.fn());
            expect(document.getElementById('gpsResult').textContent).toBe('You are in Bridgewatch.');

            window.currentMapId = '0001';
            zoneGraph.getNextHop.mockReturnValue(null);
            vi.advanceTimersByTime(1000);
            expect(document.getElementById('gpsResult').textContent).toBe('Destination: Bridgewatch. No known route from here yet.');

            cleanup();
        });

        test('flags an unexplored Avalonian Road route distinctly from "no route"', async () => {
            settingsSync.getJSON.mockReturnValue({id: '0002', name: 'Bridgewatch'});
            await GpsWidget.initGpsWidget();
            window.currentMapId = '0001';
            zonesDatabase.getZoneType.mockReturnValue('TUNNEL_ROYAL');
            const zoneGraph = {getNextHop: vi.fn(() => null)};

            const cleanup = GpsWidget.startLiveGps(zoneGraph, vi.fn(), vi.fn(), vi.fn(), vi.fn());
            expect(document.getElementById('gpsResult').textContent).toContain("unexplored Avalonian Road");
            cleanup();
        });
    });
});
