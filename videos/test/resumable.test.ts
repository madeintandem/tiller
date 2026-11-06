import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { resumableUpload, UPLOAD_URL, type Req, type Res } from "../src/publish/resumable.js";

describe("resumableUpload", () => {
  let dir: string;
  let file: string;
  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), "resumable-"));
    file = join(dir, "v.mp4");
    writeFileSync(file, Buffer.from("0123456789")); // 10 bytes
  });
  afterEach(() => rmSync(dir, { recursive: true, force: true }));

  const script = (...steps: (Res | Error)[]) => {
    const calls: Parameters<Req>[0][] = [];
    const request: Req = async (o) => {
      calls.push(o);
      const s = steps.shift();
      if (!s) throw new Error("unexpected request");
      if (s instanceof Error) throw s;
      return s;
    };
    return { request, calls };
  };
  const sleep = vi.fn().mockResolvedValue(undefined);

  it("opens a session, sends chunks with Content-Range, follows a 308, and returns the id", async () => {
    const { request, calls } = script(
      { status: 200, location: "https://up/session" },
      { status: 308, range: "bytes=0-3" },
      { status: 308, range: "bytes=0-7" },
      { status: 200, data: { id: "VID1" } },
    );
    expect(await resumableUpload(request, { snippet: { title: "t" } }, file, { chunk: 4, sleep })).toBe("VID1");
    expect(calls[0]).toMatchObject({ url: UPLOAD_URL, method: "POST", headers: { "X-Upload-Content-Length": "10", "X-Upload-Content-Type": "video/mp4" } });
    expect(calls.slice(1).map((c) => c.headers["Content-Range"])).toEqual(["bytes 0-3/10", "bytes 4-7/10", "bytes 8-9/10"]);
    expect(calls[1].url).toBe("https://up/session");
    expect((calls[3].data as Buffer).toString()).toBe("89");
  });

  it("after one 503 asks the session for its offset and resumes from there", async () => {
    const { request, calls } = script(
      { status: 200, location: "https://up/session" },
      { status: 308, range: "bytes=0-3" },
      { status: 503 },
      { status: 308, range: "bytes=0-5" }, // the server holds 6 bytes
      { status: 200, data: { id: "VID2" } },
    );
    expect(await resumableUpload(request, {}, file, { chunk: 4, sleep })).toBe("VID2");
    expect(calls.map((c) => c.headers["Content-Range"]).slice(1)).toEqual(["bytes 0-3/10", "bytes 4-7/10", "bytes */10", "bytes 6-9/10"]);
    expect(sleep).toHaveBeenCalled();
  });

  it("treats a network error like a 5xx and gives up after bounded retries", async () => {
    const boom = new Error("net");
    const { request } = script({ status: 200, location: "https://up/s" }, ...Array(10).fill(boom));
    await expect(resumableUpload(request, {}, file, { chunk: 4, sleep })).rejects.toThrow(/after 5 retries/);
  });

  it("fails fast on a 4xx", async () => {
    const { request } = script({ status: 200, location: "https://up/s" }, { status: 403 });
    await expect(resumableUpload(request, {}, file, { chunk: 4, sleep })).rejects.toThrow(/HTTP 403/);
  });

  it("fails when the session has no location", async () => {
    const { request } = script({ status: 200 });
    await expect(resumableUpload(request, {}, file, { sleep })).rejects.toThrow(/could not start/);
  });
});
