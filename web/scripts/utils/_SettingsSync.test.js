import {describe, test, expect, beforeEach, afterEach, vi} from 'vitest';
import {SettingsSync} from './SettingsSync.js';

function jsonResponse(body, ok = true) {
    return Promise.resolve({
        ok,
        json: () => Promise.resolve(body),
    });
}

describe('SettingsSync backend sync (issue #21)', () => {
    let sync;
    let originalFetch;

    beforeEach(() => {
        localStorage.clear();
        window.logger = {debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn()};
        originalFetch = globalThis.fetch;
        globalThis.fetch = vi.fn(() => jsonResponse({}));
        sync = new SettingsSync();
    });

    afterEach(() => {
        sync.destroy();
        globalThis.fetch = originalFetch;
    });

    describe('loadFromBackend', () => {
        // @verified 2026-08-02: a fresh/wiped browser has nothing in localStorage - every
        // backend-persisted key should be restored.
        test('hydrates localStorage from every key the backend returns', async () => {
            globalThis.fetch.mockReturnValue(jsonResponse({settingChestGreen: 'true', ignoreList: '["Alice"]'}));

            await sync.loadFromBackend();

            expect(localStorage.getItem('settingChestGreen')).toBe('true');
            expect(localStorage.getItem('ignoreList')).toBe('["Alice"]');
        });

        // @verified 2026-08-02: a value already in this browser (possibly an unsynced offline
        // edit) must never be clobbered by the backend's hydration pass.
        test('does not overwrite a key already present locally', async () => {
            localStorage.setItem('settingChestGreen', 'false');
            globalThis.fetch.mockReturnValue(jsonResponse({settingChestGreen: 'true'}));

            await sync.loadFromBackend();

            expect(localStorage.getItem('settingChestGreen')).toBe('false');
        });

        test('fills only the missing keys when some are present and some are not', async () => {
            localStorage.setItem('settingChestGreen', 'false');
            globalThis.fetch.mockReturnValue(jsonResponse({settingChestGreen: 'true', settingMistSolo: 'true'}));

            await sync.loadFromBackend();

            expect(localStorage.getItem('settingChestGreen')).toBe('false');
            expect(localStorage.getItem('settingMistSolo')).toBe('true');
        });

        // @verified 2026-08-02: backend unreachable (offline, server not running yet) must not
        // throw or block startup - localStorage stays as the working fallback.
        test('a network failure is swallowed, not thrown', async () => {
            globalThis.fetch.mockRejectedValue(new Error('network down'));

            await expect(sync.loadFromBackend()).resolves.toBeUndefined();
        });

        test('a non-ok response is treated as no data, not an error', async () => {
            globalThis.fetch.mockReturnValue(jsonResponse({}, false));
            localStorage.setItem('untouched', 'x');

            await sync.loadFromBackend();

            expect(localStorage.getItem('untouched')).toBe('x');
        });

        // @verified 2026-08-02: a hydrated value must be immediately readable through the normal
        // getters, not just sitting in localStorage.
        test('a hydrated value is readable through getBool immediately after', async () => {
            globalThis.fetch.mockReturnValue(jsonResponse({settingChestGreen: 'true'}));

            await sync.loadFromBackend();

            expect(sync.getBool('settingChestGreen')).toBe(true);
        });
    });

    describe('write-through on change', () => {
        // @verified 2026-08-02: every setter (set/setBool/setJSON, all funnel through
        // broadcast()) must persist to the backend too, not just localStorage.
        test('setBool POSTs the key/value pair to the backend', () => {
            sync.setBool('settingChestGreen', true);

            expect(globalThis.fetch).toHaveBeenCalledWith('/api/settings/sync', expect.objectContaining({
                method: 'POST',
                body: JSON.stringify({key: 'settingChestGreen', value: 'true'}),
            }));
        });

        test('setJSON POSTs the stringified value', () => {
            sync.setJSON('ignoreList', ['Alice']);

            expect(globalThis.fetch).toHaveBeenCalledWith('/api/settings/sync', expect.objectContaining({
                method: 'POST',
                body: JSON.stringify({key: 'ignoreList', value: '["Alice"]'}),
            }));
        });

        // @verified 2026-08-02: a backend write failure must not throw out of a setter call -
        // the setting is already correctly applied locally/cross-tab regardless.
        test('a backend write failure does not throw', () => {
            globalThis.fetch.mockRejectedValue(new Error('network down'));

            expect(() => sync.setBool('settingChestGreen', true)).not.toThrow();
        });

        test('remove() sends a DELETE with the key as a query param', () => {
            sync.remove('settingChestGreen');

            expect(globalThis.fetch).toHaveBeenCalledWith(
                '/api/settings/sync?key=settingChestGreen',
                expect.objectContaining({method: 'DELETE'})
            );
        });
    });
});
