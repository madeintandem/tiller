import { createReadStream, existsSync, readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { createServer } from "node:http";
import { execFile } from "node:child_process";
import { dirname, join } from "node:path";
import { google, type youtube_v3 } from "googleapis";
import { layoutFor, type Layout } from "../build/layout.js";
import { loadConfig, type Config } from "../config.js";
import { loadPublishable, videoResource } from "./metadata.js";

export type Published = Record<string, { youtubeId: string; uploadedAt: string; privacy: string }>;

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
  const yt = google.youtube({ version: "v3", auth: await authorize(layout) });
  return {
    async insertVideo(resource, mp4Path) {
      // The googleapis media upload is resumable.
      const r = await yt.videos.insert({
        part: ["snippet", "status"],
        requestBody: resource,
        media: { body: createReadStream(mp4Path) },
      });
      if (!r.data.id) throw new Error("videos.insert returned no video id");
      return r.data.id;
    },
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
// Every refusal happens before the first API call.
export async function publish(
  id: string,
  opts: { layout?: Layout; api?: YouTubeApi; cfg?: Config } = {},
): Promise<string> {
  // VIDEOS_APPROVALS never applies to a publish.
  const layout = opts.layout ?? layoutFor(undefined, { ...process.env, VIDEOS_APPROVALS: undefined });
  const publishedFile = join(layout.root, "published.json");
  const published: Published = existsSync(publishedFile) ? JSON.parse(readFileSync(publishedFile, "utf8")) : {};
  if (published[id]) {
    throw new Error(`${id} is already published as YouTube video ${published[id].youtubeId}; remove its entry from published.json to upload again`);
  }
  const { mp4, srt, jpg, meta } = loadPublishable(id, layout);
  const cfg = opts.cfg ?? loadConfig(layout.config);
  const resource = videoResource(meta, cfg);

  const api = opts.api ?? (await realApi(layout));
  const youtubeId = await api.insertVideo(resource, mp4);
  // Recorded at once: a later failure must not let a retry upload a second copy.
  published[id] = { youtubeId, uploadedAt: new Date().toISOString(), privacy: cfg.youtube.privacy };
  writeFileSync(publishedFile, JSON.stringify(published, null, 2) + "\n");
  await api.setThumbnail(youtubeId, jpg);
  await api.insertCaption(youtubeId, srt);
  return youtubeId;
}
