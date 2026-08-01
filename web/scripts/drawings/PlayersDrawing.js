import {DrawingUtils} from "../utils/DrawingUtils.js";

export class PlayersDrawing extends DrawingUtils {
    constructor() {
        super();
        this.itemsInfo = {};
    }

    updateItemsInfo(newData) {
        this.itemsInfo = newData;
    }

    // Positions are XOR-encrypted and not decryptable client-side (see
    // PLAYER_POSITIONS_MITM.md), so there is deliberately no invalidate()/draw step here -
    // interpolate() still runs (see RadarRenderer.update()) since other code (player list
    // distance, stats) may read the interpolated hX/hY, but nothing ever paints from it.
    interpolate(players, lpX, lpY, t) {
        for (const playerOne of players) {
            this.interpolateEntity(playerOne, lpX, lpY, t);
        }
    }
}
