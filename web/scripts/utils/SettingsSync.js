import {CATEGORIES} from "../constants/LoggerConstants.js";

const CHANNEL_NAME = 'openradar-settings';

export class SettingsSync {
    constructor() {
        this.channel = null;
        this.listeners = new Map();
        this.isInitialized = false;
        this.cache = new Map();

        this._boundMessageHandler = (event) => this.handleMessage(event.data);
        this._boundStorageHandler = (event) => {
            if (event.key && event.newValue !== null) {
                this.handleMessage({ type: 'setting-changed', key: event.key, value: event.newValue });
            }
        };
        this._usingFallback = false;

        if (typeof BroadcastChannel !== 'undefined') {
            this.initialize();
        } else {
            this.setupFallback();
        }
    }

    initialize() {
        try {
            this.channel = new BroadcastChannel(CHANNEL_NAME);
            this.channel.addEventListener('message', this._boundMessageHandler);
            this.isInitialized = true;
        } catch (e) {
            window.logger?.info(CATEGORIES.SYSTEM, 'BroadcastChannel_Fallback', {reason: e?.message});
            this.setupFallback();
        }
    }

    setupFallback() {
        window.addEventListener('storage', this._boundStorageHandler);
        this._usingFallback = true;
        window.logger?.info(CATEGORIES.SYSTEM, 'SettingsSync_UsingStorageFallback', {});
    }

    _getCached(key) {
        if (this.cache.has(key)) return this.cache.get(key);
        const value = localStorage.getItem(key);
        this.cache.set(key, value);
        return value;
    }

    // Issue #21: hydrate localStorage from settings-sync.json on startup, so settings survive
    // a browser data wipe or move to another machine. Only fills keys ABSENT locally - a value
    // already in localStorage is trusted as-is (it may include edits made offline that haven't
    // synced to the backend yet), so this never clobbers the current browser's state, it only
    // recovers what's missing. Call once, before any feature code reads a setting.
    async loadFromBackend() {
        try {
            const response = await fetch('/api/settings/sync');
            if (!response.ok) return;
            const settings = await response.json();
            let hydrated = 0;
            for (const [key, value] of Object.entries(settings)) {
                if (localStorage.getItem(key) === null) {
                    this._setLocal(key, value);
                    hydrated++;
                }
            }
            if (hydrated > 0) {
                window.logger?.info(CATEGORIES.SYSTEM, 'SettingsSyncBackendHydrated', {hydrated});
            }
        } catch (error) {
            window.logger?.debug(CATEGORIES.SYSTEM, 'SettingsSyncBackendLoadFailed', {
                error: error?.message || error
            });
        }
    }

    // Best-effort write-through to settings-sync.json. Mirrors ZoneGraph.reportTransition's
    // fetch guard: try/catch around the call itself (fetch can throw synchronously, e.g.
    // unavailable in the current environment) in addition to .catch() on the rejection, so a
    // sync failure never breaks the setting change it's piggybacking on.
    _syncToBackend(key, value) {
        try {
            Promise.resolve(
                fetch('/api/settings/sync', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({key, value}),
                })
            ).catch((error) => {
                window.logger?.debug(CATEGORIES.SYSTEM, 'SettingsSyncBackendWriteFailed', {
                    key,
                    error: error?.message || error
                });
            });
        } catch (error) {
            window.logger?.debug(CATEGORIES.SYSTEM, 'SettingsSyncBackendWriteFailed', {
                key,
                error: error?.message || error
            });
        }
    }

    _deleteFromBackend(key) {
        try {
            Promise.resolve(
                fetch(`/api/settings/sync?key=${encodeURIComponent(key)}`, {method: 'DELETE'})
            ).catch((error) => {
                window.logger?.debug(CATEGORIES.SYSTEM, 'SettingsSyncBackendDeleteFailed', {
                    key,
                    error: error?.message || error
                });
            });
        } catch (error) {
            window.logger?.debug(CATEGORIES.SYSTEM, 'SettingsSyncBackendDeleteFailed', {
                key,
                error: error?.message || error
            });
        }
    }

    handleMessage(data) {
        if (data.type === 'setting-changed' || data.type === 'setting-removed') {
            if (data.type === 'setting-changed') {
                this.cache.set(data.key, data.value);
            } else {
                this.cache.delete(data.key);
            }

            const listeners = this.listeners.get(data.key) || [];
            listeners.forEach(callback => {
                try { callback(data.key, data.value); } catch (error) {
                    window.logger?.error(CATEGORIES.SYSTEM, 'SettingsSyncListenerError', {
                        key: data.key,
                        error: error?.message || error
                    });
                }
            });

            const wildcardListeners = this.listeners.get('*') || [];
            wildcardListeners.forEach(callback => {
                try { callback(data.key, data.value); } catch (error) {
                    window.logger?.error(CATEGORIES.SYSTEM, 'SettingsSyncWildcardListenerError', {
                        key: data.key,
                        error: error?.message || error
                    });
                }
            });
        }
    }

    // Local-only: updates this tab's localStorage/cache and notifies other tabs, without
    // touching the backend. Used by broadcast() (which adds the backend write-through) and by
    // loadFromBackend() (which must NOT write back what it just read).
    _setLocal(key, value) {
        localStorage.setItem(key, value);

        if (this.channel && this.isInitialized) {
            try {
                this.channel.postMessage({ type: 'setting-changed', key, value, timestamp: Date.now() });
            } catch (error) {
                window.logger?.error(CATEGORIES.SYSTEM, 'SettingsSyncBroadcastFailed', {
                    key,
                    error: error?.message || error
                });
            }
        }

        this.handleMessage({ type: 'setting-changed', key, value });
    }

    broadcast(key, value) {
        this._setLocal(key, value);
        this._syncToBackend(key, value);
    }

    on(key, callback) {
        if (!this.listeners.has(key)) this.listeners.set(key, []);
        this.listeners.get(key).push(callback);
    }

    off(key, callback) {
        if (!this.listeners.has(key)) return;
        const listeners = this.listeners.get(key);
        const index = listeners.indexOf(callback);
        if (index > -1) listeners.splice(index, 1);
    }

    removeAllListeners(key) { this.listeners.delete(key); }

    get(key, defaultValue = null) {
        const value = this._getCached(key);
        return value !== null ? value : defaultValue;
    }

    set(key, value) { this.broadcast(key, value); }

    getBool(key, defaultValue = false) {
        const value = this._getCached(key);
        if (value === null) return defaultValue;
        return value === 'true';
    }

    setBool(key, value) { this.broadcast(key, value.toString()); }

    getNumber(key, defaultValue = 0) {
        const value = this._getCached(key);
        if (value === null || value === '') return defaultValue;
        const parsed = parseInt(value, 10);
        return isNaN(parsed) ? defaultValue : parsed;
    }

    setNumber(key, value) { this.broadcast(key, value.toString()); }

    getFloat(key, defaultValue = 0) {
        const value = this._getCached(key);
        if (value === null || value === '') return defaultValue;
        const parsed = parseFloat(value);
        return isNaN(parsed) ? defaultValue : parsed;
    }

    setFloat(key, value) { this.broadcast(key, value.toString()); }

    getJSON(key, defaultValue = null) {
        const value = this._getCached(key);
        if (value === null || value === '') return defaultValue;
        try { return JSON.parse(value); }
        catch (error) {
            window.logger?.error(CATEGORIES.SYSTEM, 'SettingsSyncJSONParseFailed', {
                key,
                error: error?.message || error
            });
            return defaultValue;
        }
    }

    setJSON(key, value) {
        try { this.broadcast(key, JSON.stringify(value)); } catch (error) {
            window.logger?.error(CATEGORIES.SYSTEM, 'SettingsSyncJSONStringifyFailed', {
                key,
                error: error?.message || error
            });
        }
    }

    remove(key) {
        localStorage.removeItem(key);

        if (this.channel && this.isInitialized) {
            try {
                this.channel.postMessage({ type: 'setting-removed', key, timestamp: Date.now() });
            } catch (error) {
                window.logger?.error(CATEGORIES.SYSTEM, 'SettingsSyncRemoveFailed', {
                    key,
                    error: error?.message || error
                });
            }
        }

        this.handleMessage({ type: 'setting-removed', key, value: null });
        this._deleteFromBackend(key);
    }

    destroy() {
        if (this.channel) {
            this.channel.removeEventListener('message', this._boundMessageHandler);
            this.channel.close();
            this.channel = null;
        }
        if (this._usingFallback) {
            window.removeEventListener('storage', this._boundStorageHandler);
        }
        this.listeners.clear();
        this.cache.clear();
        this.isInitialized = false;
    }
}

let settingsSyncInstance = null;

export function getSettingsSync() {
    if (!settingsSyncInstance) {
        settingsSyncInstance = new SettingsSync();
        window.addEventListener('beforeunload', () => {
            if (settingsSyncInstance) settingsSyncInstance.destroy();
        });
    }
    return settingsSyncInstance;
}

export default getSettingsSync();
