import {CATEGORIES} from '../constants/LoggerConstants.js';

// Sidebar footer containers (see internal/templates/layouts/sidebar.gohtml) - both the
// desktop and mobile sidebar show the version, so the badge is injected into both.
const CONTAINER_IDS = ['sidebarVersionDesktop', 'sidebarVersionMobile'];

// Checks once per page session whether a newer OpenRadar release is available (the actual
// GitHub check runs once per launch, server-side - see internal/updatecheck and
// docs/technical/AUTO_UPDATE_CHECK.md) and, if so, injects a small dismissible badge next to
// "vX.Y.Z" in the sidebar footer(s).
export async function initUpdateBadge() {
    let status;
    try {
        const response = await fetch('/api/settings/update');
        if (!response.ok) return;
        status = await response.json();
    } catch (error) {
        window.logger?.warn(CATEGORIES.SYSTEM, 'UpdateCheckFetchFailed', {error: error?.message || error});
        return;
    }

    if (!status?.available) return;

    for (const containerId of CONTAINER_IDS) {
        document.getElementById(containerId)?.appendChild(buildBadge(status));
    }
}

function buildBadge(status) {
    const badge = document.createElement('div');
    badge.className = 'update-badge flex items-center gap-1';

    const link = document.createElement('a');
    link.href = status.releaseUrl || '#';
    link.target = '_blank';
    link.rel = 'noopener noreferrer';
    link.className = 'text-xs text-warning hover:underline';
    link.textContent = `v${status.latestVersion} disponible`;
    badge.appendChild(link);

    const dismissBtn = document.createElement('button');
    dismissBtn.type = 'button';
    dismissBtn.className = 'text-base-content/40 hover:text-base-content/70 leading-none';
    dismissBtn.setAttribute('aria-label', 'Masquer la notification de mise à jour');
    dismissBtn.textContent = '×';
    dismissBtn.addEventListener('click', dismiss);
    badge.appendChild(dismissBtn);

    return badge;
}

// Removes the badge(s) immediately (the user's intent is already satisfied) and persists the
// dismissal best-effort - a failed POST just means the badge reappears on the next launch,
// never an error visible to the user (same fire-and-forget reasoning as ZoneGraph.reportTransition).
async function dismiss() {
    document.querySelectorAll('.update-badge').forEach(el => el.remove());
    try {
        await fetch('/api/settings/update/dismiss', {method: 'POST'});
    } catch (error) {
        window.logger?.warn(CATEGORIES.SYSTEM, 'UpdateDismissFailed', {error: error?.message || error});
    }
}
