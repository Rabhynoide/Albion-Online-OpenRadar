import {describe, test, expect, beforeEach, afterEach, vi} from 'vitest';
import {initUpdateBadge} from './UpdateBadge.js';

function jsonResponse(body, ok = true) {
    return Promise.resolve({
        ok,
        status: ok ? 200 : 500,
        json: () => Promise.resolve(body),
    });
}

function setupDom() {
    document.body.innerHTML = `
        <div id="sidebarVersionDesktop"><p>v1.0.2</p></div>
        <div id="sidebarVersionMobile"><p>v1.0.2</p></div>
    `;
}

describe('UpdateBadge', () => {
    let originalFetch;

    beforeEach(() => {
        setupDom();
        window.logger = {debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn()};
        originalFetch = globalThis.fetch;
    });

    afterEach(() => {
        globalThis.fetch = originalFetch;
    });

    test('injects a badge into both sidebar containers when an update is available', async () => {
        globalThis.fetch = vi.fn(() => jsonResponse({
            available: true, currentVersion: '1.0.2', latestVersion: '1.1.0', releaseUrl: 'https://example.invalid/1.1.0',
        }));

        await initUpdateBadge();

        const badges = document.querySelectorAll('.update-badge');
        expect(badges).toHaveLength(2);
        expect(document.getElementById('sidebarVersionDesktop').textContent).toContain('v1.1.0 disponible');
        const link = document.getElementById('sidebarVersionDesktop').querySelector('a');
        expect(link.href).toBe('https://example.invalid/1.1.0');
        expect(link.target).toBe('_blank');
    });

    test('injects nothing when no update is available', async () => {
        globalThis.fetch = vi.fn(() => jsonResponse({available: false, currentVersion: '1.0.2'}));

        await initUpdateBadge();

        expect(document.querySelectorAll('.update-badge')).toHaveLength(0);
    });

    test('injects nothing on a non-ok response', async () => {
        globalThis.fetch = vi.fn(() => jsonResponse({}, false));

        await initUpdateBadge();

        expect(document.querySelectorAll('.update-badge')).toHaveLength(0);
    });

    test('injects nothing and logs on a network failure', async () => {
        globalThis.fetch = vi.fn(() => Promise.reject(new Error('network down')));

        await initUpdateBadge();

        expect(document.querySelectorAll('.update-badge')).toHaveLength(0);
        expect(window.logger.warn).toHaveBeenCalled();
    });

    test('clicking dismiss removes every badge and posts the dismiss endpoint', async () => {
        globalThis.fetch = vi.fn(() => jsonResponse({
            available: true, currentVersion: '1.0.2', latestVersion: '1.1.0', releaseUrl: 'https://example.invalid/1.1.0',
        }));
        await initUpdateBadge();
        expect(document.querySelectorAll('.update-badge')).toHaveLength(2);

        globalThis.fetch = vi.fn(() => jsonResponse({available: false}));
        document.querySelector('.update-badge button').click();
        await Promise.resolve();
        await Promise.resolve();

        expect(document.querySelectorAll('.update-badge')).toHaveLength(0);
        expect(globalThis.fetch).toHaveBeenCalledWith('/api/settings/update/dismiss', {method: 'POST'});
    });
});
