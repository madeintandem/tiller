import { createReadStream, existsSync, readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { createServer } from "node:http";
import { execFile } from "node:child_process";
import { dirname, join } from "node:path";
import { google, type youtube_v3 } from "googleapis";
import { layoutFor, type Layout } from "../build/layout.js";
import { loadConfig, type Config } from "../config.js";
import { resumableUpload, type Req } from "./resumable.js";
import { loadPublishable, videoResource } from "./metadata.js";

// `thumbnail` and `captions` record which post-insert steps are done, so a rerun finishes the rest.
export type Published = Record<string, { youtubeId: string; uploadedAt: string; privacy: string; thumbnail?: boolean; captions?: boolean }>;

// The three YouTube calls publish makes, so tests can stand in for the API.
export type YouTubeApi = {
  insertVideo(resource: youtube_v3.Schema$Video, mp4Path: string): Promise<string>;
  setThumbnail(videoId: string, jpgPath: string): Promise<void>;
  insertCaption(videoId: string, srtPath: string): Promise<void>;
};

const SCOPES = ["https://www.googleapis.com/auth/youtube.upload", "https://www.googleapis.com/auth/youtube.force-ssl"];

// OAuth with the client in .local/youtube-client.json (a Google "Desktop app" client).
// The token is cached in .local/youtube-token.json; the first run opens the browser for consent.
async function authorize(layout: Layout) {
  const clientFile = join(layout.local, "youtube-client.json");
  const tokenFile = join(layout.local, "youtube-token.json");
  if (!existsSync(clientFile)) throw new Error(`no YouTube OAuth client at ${clientFile}`);
  const raw = JSON.parse(readFileSync(clientFile, "utf8"));
  const c = raw.installed ?? raw.web;
  if (!c?.client_id || !c?.client_secret) throw new Error(`${clientFile} has no client_id and client_secret`);

  const server = createServer();
  await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
  const port = (server.address() as { port: number }).port;
  const auth = new google.auth.OAuth2(c.client_id, c.client_secret, `http://127.0.0.1:${port}`);
  auth.on("tokens", (t) => {
    const prev = existsSync(tokenFile) ? JSON.parse(readFileSync(tokenFile, "utf8")) : {};
    mkdirSync(dirname(tokenFile), { recursive: true });
    writeFileSync(tokenFile, JSON.stringify({ ...prev, ...t }, null, 2) + "\n", { mode: 0o600 });
  });
  try {
    if (existsSync(tokenFile)) {
      auth.setCredentials(JSON.parse(readFileSync(tokenFile, "utf8")));
    } else {
      const url = auth.generateAuthUrl({ access_type: "offline", scope: SCOPES, prompt: "consent" });
      console.log(`Opening the browser for YouTube consent. If it does not open, visit:\n${url}`);
      execFile("open", [url], () => {});
      const code = await new Promise<string>((resolve, reject) => {
        server.on("request", (req, res) => {
          const u = new URL(req.url ?? "/", `http://127.0.0.1:${port}`);
          const got = u.searchParams.get("code");
          res.end(got ? "Consent received. You can close this tab." : "No code received.");
          if (got) resolve(got);
          else if (u.searchParams.get("error")) reject(new Error(`consent failed: ${u.searchParams.get("error")}`));
        });
      });
      const { tokens } = await auth.getToken(code);
      auth.setCredentials(tokens);
      mkdirSync(dirname(tokenFile), { recursive: true });
      writeFileSync(tokenFile, JSON.stringify(tokens, null, 2) + "\n", { mode: 0o600 });
    }
  } finally {
    server.close();
  }
  return auth;
}

export async function realApi(layout: Layout): Promise<YouTubeApi> {
  const auth = await authorize(layout);
  const yt = google.youtube({ version: "v3", auth });
  // The OAuth client signs each request; statuses are returned, not thrown, so the
  // resumable protocol can read 308s and 5xxs itself.
  const request: Req = async (o) => {
    const r: any = await auth.request({ ...o, validateStatus: () => true, redirect: "manual", responseType: "json" } as any);
    const h = (n: string): string | undefined => (typeof r.headers?.get === "function" ? r.headers.get(n) : r.headers?.[n]) ?? undefined;
    return { status: r.status, location: h("location"), range: h("range"), data: r.data };
  };
  return {
    insertVideo: (resource, mp4Path) => resumableUpload(request, resource, mp4Path),
    async setThumbnail(videoId, jpgPath) {
      await yt.thumbnails.set({ videoId, media: { mimeType: "image/jpeg", body: createReadStream(jpgPath) } });
    },
    async insertCaption(videoId, srtPath) {
      await yt.captions.insert({
        part: ["snippet"],
        requestBody: { snippet: { videoId, language: "en", name: "English" } },
        media: { mimeType: "application/octet-stream", body: createReadStream(srtPath) },
      });
    },
  };
}

// Upload the built video, its thumbnail, and its captions. Returns the YouTube video ID.
// Every refusal happens before the first API call. A rerun on an entry whose thumbnail or
// captions are not done performs only those steps; it never uploads the video again.
export async function publish(
  id: string,
  opts: { layout?: Layout; api?: YouTubeApi; cfg?: Config } = {},
): Promise<string> {
  // VIDEOS_APPROVALS never applies to a publish.
  const layout = opts.layout ?? layoutFor(undefined, { ...process.env, VIDEOS_APPROVALS: undefined });
  const publishedFile = join(layout.root, "published.json");
  const published: Published = existsSync(publishedFile) ? JSON.parse(readFileSync(publishedFile, "utf8")) : {};
  const prior = published[id];
  if (prior?.thumbnail && prior?.captions) {
    throw new Error(
      `${id} is already published as YouTube video ${prior.youtubeId}; to upload a new copy, delete that video in YouTube Studio first, then remove its published.json entry`,
    );
  }
  const { mp4, srt, jpg, meta } = loadPublishable(id, layout);
  const cfg = opts.cfg ?? loadConfig(layout.config);
  const resource = videoResource(meta, cfg);

  const api = opts.api ?? (await realApi(layout));
  const save = () => writeFileSync(publishedFile, JSON.stringify(published, null, 2) + "\n");
  let entry = prior;
  if (!entry) {
    const youtubeId = await api.insertVideo(resource, mp4);
    // Recorded at once: a later failure must not let a retry upload a second copy.
    entry = published[id] = { youtubeId, uploadedAt: new Date().toISOString(), privacy: cfg.youtube.privacy, thumbnail: false, captions: false };
    save();
  }
  const done = entry;
  const steps: ["thumbnail" | "captions", () => Promise<void>][] = [
    ["thumbnail", () => api.setThumbnail(done.youtubeId, jpg)],
    ["captions", () => api.insertCaption(done.youtubeId, srt)],
  ];
  for (const [name, run] of steps) {
    if (done[name]) continue;
    try {
      await run();
    } catch (e) {
      throw new Error(`uploaded as ${done.youtubeId}; ${name} failed: ${(e as Error).message}; rerun npm run publish -- ${id} to finish`);
    }
    done[name] = true;
    save();
  }
  return done.youtubeId;
}
