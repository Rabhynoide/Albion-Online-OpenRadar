// synthetic: v1 draws every treasure with the same shared icon - no per-type settings gate
// lives in this layer (see LocalTreasuresHandler, which gates on settingLocalTreasures instead).

import {describe, test, expect, beforeEach, vi} from 'vitest';

const {LocalTreasuresDrawing} = await import('./LocalTreasuresDrawing.js');

describe('LocalTreasuresDrawing', () => {
    let drawing;
    let ctx;

    beforeEach(() => {
        drawing = new LocalTreasuresDrawing();
        drawing.DrawCustomImage = vi.fn();
        drawing.transformPoint = vi.fn((x, y) => ({x, y}));
        drawing.interpolateEntity = vi.fn();
        ctx = {};
    });

    test('invalidate draws the shared legendary icon for a CHEST-type treasure', () => {
        const treasure = {hX: 10, hY: 20, label: 'CHEST'};

        drawing.invalidate(ctx, [treasure]);

        expect(drawing.DrawCustomImage).toHaveBeenCalledWith(ctx, 10, 20, 'legendary', 'Resources', 35);
    });

    test('invalidate draws every treasure regardless of label', () => {
        const treasures = [
            {hX: 1, hY: 2, label: 'SPECIAL_EVENT_1'},
            {hX: 3, hY: 4, label: 'SMUGGLER_PILE'},
            {hX: 5, hY: 6, label: 'RESOURCE_T6'},
        ];

        drawing.invalidate(ctx, treasures);

        expect(drawing.DrawCustomImage).toHaveBeenCalledTimes(3);
        expect(drawing.DrawCustomImage).toHaveBeenNthCalledWith(1, ctx, 1, 2, 'legendary', 'Resources', 35);
        expect(drawing.DrawCustomImage).toHaveBeenNthCalledWith(2, ctx, 3, 4, 'legendary', 'Resources', 35);
        expect(drawing.DrawCustomImage).toHaveBeenNthCalledWith(3, ctx, 5, 6, 'legendary', 'Resources', 35);
    });

    test('invalidate on an empty list draws nothing', () => {
        drawing.invalidate(ctx, []);

        expect(drawing.DrawCustomImage).not.toHaveBeenCalled();
    });

    test('interpolate delegates to interpolateEntity per treasure', () => {
        const treasures = [{id: 1}, {id: 2}];

        drawing.interpolate(treasures, 0, 0, 0.5);

        expect(drawing.interpolateEntity).toHaveBeenCalledTimes(2);
    });
});
