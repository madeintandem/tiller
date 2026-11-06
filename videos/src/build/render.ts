import { rmSync, statSync } from "node:fs";
import { resolve } from "node:path";
import { bundle } from "@remotion/bundler";
import { renderMedia, renderStill, selectComposition } from "@remotion/renderer";
import { video } from "../theme.js";
import type { Timeline } from "../timeline/build.js";

const ENTRY = resolve(import.meta.dirname, "../index.ts");

// Render the Video composition to H.264 at 1920x1080, 30 fps. `publicDir` is the
// build directory: the timeline's asset paths are relative to it, so staticFile()
// resolves the prepared intro, each beat's narration, and each capture.
export async function renderVideo(timeline: Timeline, publicDir: string, outPath: string): Promise<string> {
  // The bundler copies publicDir; a previous render must not ride along.
  rmSync(outPath, { force: true });
  const serveUrl = await bundle({ entryPoint: ENTRY, publicDir });
  const inputProps = { timeline };
  const composition = await selectComposition({ serveUrl, id: "Video", inputProps });
  if (composition.width !== video.width || composition.height !== video.height || composition.fps !== video.fps) {
    throw new Error(
      `Video composition is ${composition.width}x${composition.height} at ${composition.fps} fps, ` +
        `expected ${video.width}x${video.height} at ${video.fps}`,
    );
  }
  await renderMedia({ composition, serveUrl, codec: "h264", outputLocation: outPath, inputProps });
  return outPath;
}

const JPEG_LIMIT = 2 * 1024 * 1024;

// Render the Thumbnail still to a 1280x720 JPEG under YouTube's 2 MB limit. `publicDir`
// supplies the brand assets, as for the video.
export async function renderThumbnail(title: string, publicDir: string, outPath: string): Promise<string> {
  rmSync(outPath, { force: true });
  const serveUrl = await bundle({ entryPoint: ENTRY, publicDir });
  const inputProps = { title };
  const composition = await selectComposition({ serveUrl, id: "Thumbnail", inputProps });
  await renderStill({ composition, serveUrl, output: outPath, inputProps, imageFormat: "jpeg", jpegQuality: 90 });
  const size = statSync(outPath).size;
  if (size >= JPEG_LIMIT) throw new Error(`thumbnail ${outPath} is ${size} bytes, over YouTube's 2 MB limit`);
  return outPath;
}
