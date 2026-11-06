import { closeSync, openSync, readSync, statSync } from "node:fs";
import type { youtube_v3 } from "googleapis";

// One HTTP exchange, normalized: `location` and `range` are the response headers of those names.
export type Res = { status: number; location?: string; range?: string; data?: any };
export type Req = (o: {
  url: string;
  method: "POST" | "PUT";
  headers: Record<string, string>;
  data?: Buffer | string;
}) => Promise<Res>;

export const UPLOAD_URL = "https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status";
export const CHUNK = 8 * 1024 * 1024; // a multiple of 256 KiB, as the protocol requires
const MAX_RETRIES = 5;

const sleepMs = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

// First byte the server still needs, from a 308's Range header ("bytes=0-N"; absent means nothing yet).
const nextOffset = (range?: string) => {
  const m = range?.match(/bytes=\d+-(\d+)/);
  return m ? Number(m[1]) + 1 : 0;
};

// YouTube resumable upload: open a session with the video resource, then PUT the file in
// chunks. On a network error or 5xx the session is asked how much it holds and the upload
// resumes from there, with bounded retries and backoff. Returns the new video's ID.
export async function resumableUpload(
  request: Req,
  resource: youtube_v3.Schema$Video,
  file: string,
  opts: { chunk?: number; sleep?: (ms: number) => Promise<void> } = {},
): Promise<string> {
  const chunk = opts.chunk ?? CHUNK;
  const sleep = opts.sleep ?? sleepMs;
  const size = statSync(file).size;
  if (size === 0) throw new Error(`${file} is empty`);

  const init = await request({
    url: UPLOAD_URL,
    method: "POST",
    headers: {
      "Content-Type": "application/json; charset=UTF-8",
      "X-Upload-Content-Type": "video/mp4",
      "X-Upload-Content-Length": String(size),
    },
    data: JSON.stringify(resource),
  });
  if (init.status !== 200 || !init.location) {
    throw new Error(`could not start the upload: HTTP ${init.status}${init.location ? "" : " with no session location"}`);
  }
  const session = init.location;

  const fd = openSync(file, "r");
  try {
    let offset = 0;
    let failures = 0;
    let resync = false; // the last chunk failed: ask the session where it stands before sending more
    while (true) {
      let res: Res | undefined;
      try {
        if (resync) {
          res = await request({ url: session, method: "PUT", headers: { "Content-Range": `bytes */${size}`, "Content-Length": "0" } });
        } else {
          const len = Math.min(chunk, size - offset);
          const buf = Buffer.alloc(len);
          readSync(fd, buf, 0, len, offset);
          res = await request({
            url: session,
            method: "PUT",
            headers: { "Content-Range": `bytes ${offset}-${offset + len - 1}/${size}`, "Content-Length": String(len), "Content-Type": "video/mp4" },
            data: buf,
          });
        }
      } catch {
        res = undefined; // network error: treated like a 5xx
      }
      if (res && (res.status === 200 || res.status === 201)) {
        if (!res.data?.id) throw new Error("the upload finished but returned no video id");
        return res.data.id as string;
      }
      if (res?.status === 308) {
        offset = nextOffset(res.range);
        resync = false;
        failures = 0;
        continue;
      }
      if (res && res.status < 500) throw new Error(`upload failed: HTTP ${res.status}`);
      if (++failures > MAX_RETRIES) throw new Error(`upload failed after ${MAX_RETRIES} retries${res ? `: HTTP ${res.status}` : " (network error)"}`);
      resync = true;
      await sleep(Math.min(30_000, 500 * 2 ** (failures - 1)));
    }
  } finally {
    closeSync(fd);
  }
}
