import {DrawingUtils} from "../utils/DrawingUtils.js";
import {CATEGORIES} from "../constants/LoggerConstants.js";
import settingsSync from "../utils/SettingsSync.js";
import imageCache from "../utils/ImageCache.js";
import zonesDatabase from "../data/ZonesDatabase.js";

export class MapDrawing extends DrawingUtils
{
    interpolate(curr_map, lpX, lpY , t)
    {
        const hX = lpX;
        const hY = -lpY;

        curr_map.hX = this.lerp(curr_map.hX, hX, t);
        curr_map.hY = this.lerp(curr_map.hY, hY, t);
    }

    draw(ctx, curr_map)
    {
        if (curr_map.id < 0)
            return;

        const zoom = this.getZoomLevel();
        const scaleFactor = 4 * zoom;
        const id = curr_map.id.toString();
        const extent = zonesDatabase.getMapAssetExtent(id);
        const center = zonesDatabase.getMapAssetCenter(id);
        const size = extent * scaleFactor;
        const adjX = (curr_map.hX - center.x) * scaleFactor;
        const adjY = (curr_map.hY + center.y) * scaleFactor;
        // Roads of Avalon passage/tunnel instances have a per-instance zone id (e.g.
        // "PSG-0039#2") but only one downloaded tile per base zone ("PSG-0039.webp" - see
        // download-and-optimize-map.ts, which names files from zone.file, not zone.id). Using
        // the raw id as the image name 404s for every one of these (#15) - resolve through
        // zone.file instead, same truncation the downloader itself uses.
        const zoneFile = zonesDatabase.getZoneFile(id);
        const imageName = zoneFile ? zoneFile.split('_')[0] : id;
        this.DrawImageMap(ctx, adjX, adjY, imageName, size, size);
    }
    DrawImageMap(ctx, x, y, imageName, drawWidth, drawHeight)
    {
        // Fill background => if no map image or corner to prevent glitch textures
        ctx.fillStyle = '#1a1c23';
        ctx.fillRect(0, 0, ctx.width, ctx.height);

        if (!settingsSync.getBool("settingShowMap", true)) return;

        if (imageName === undefined || imageName == "undefined")
            return;

        const src = "/images/Maps/" + imageName + ".webp";

        const preloadedImage = imageCache.GetPreloadedImage(src, "Maps");

        if (preloadedImage === null) return;

        if (preloadedImage)
        {
            ctx.save();

            ctx.scale(1, -1);
            const center = this.getCanvasCenter();
            ctx.translate(center, -center);

            ctx.rotate(-0.785398);
            ctx.translate(-x, y);

            ctx.drawImage(preloadedImage, -drawWidth/2, -drawHeight/2, drawWidth, drawHeight);
            ctx.restore();
        }
        else
        {
            imageCache.preloadImageAndAddToList(src, "Maps")
            .then(() => {
                window.logger?.info(CATEGORIES.MAP, 'map_loaded', {src: src});
            })
            .catch((error) => {
                window.logger?.warn(CATEGORIES.MAP, 'map_load_failed', {src: src, error: error?.message});
            });
        }
    }
}