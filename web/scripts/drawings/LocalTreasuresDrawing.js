import {DrawingUtils} from "../utils/DrawingUtils.js";

// v1: a single shared icon for every local-treasure type (chests, temporary resource nodes,
// smuggler piles, timed special events) - no bespoke per-type icons exist yet. `label` is kept
// on each entity so per-type icons can be added later without touching the parsing layer.
export class LocalTreasuresDrawing extends DrawingUtils {
    interpolate(treasures, lpX, lpY, t) {
        for (const treasureOne of treasures) {
            this.interpolateEntity(treasureOne, lpX, lpY, t);
        }
    }

    invalidate(ctx, treasures) {
        for (const treasureOne of treasures) {
            const point = this.transformPoint(treasureOne.hX, treasureOne.hY);
            this.DrawCustomImage(ctx, point.x, point.y, "legendary", "Resources", 35);
        }
    }
}
