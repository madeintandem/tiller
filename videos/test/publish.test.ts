import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DISCLOSURE } from "../src/build/meta.js";
import { layoutFor, type Layout } from "../src/build/layout.js";
import type { Config } from "../src/config.js";
import { videoResource } from "../src/publish/metadata.js";
import { publish, type YouTubeApi } from "../src/publish/youtube.js";
import { scriptHash } from "../src/script/load.js";

const cfg = (privacy: string = "private"): Config =>
  ({ elevenlabs: { apiKey: "", voiceId: "", modelId: "m" }, youtube: { privacy, categoryId: "28" } }) as Config;

const baseMeta = {
  title: "A test video",
  description: `Body\n\n0:00 Intro\n\n${DISCLOSURE}`,
  tags: ["tiller", "test"],
  chapters: ["0:00 Intro"],
  durationSeconds: 61,
  fake: false,
  scriptHash: "",
};

describe("videoResource", () => {
  it("sets synthetic media, not-for-kids, the configured privacy and category", () => {
    const r = videoResource(baseMeta, cfg("unlisted"));
    expect(r.status).toMatchObject({
      privacyStatus: "unlisted",
      containsSyntheticMedia: true,
      selfDeclaredMadeForKids: false,
    });
    expect(r.snippet).toMatchObject({ title: "A test video", tags: ["tiller", "test"], categoryId: "28" });
    expect(r.snippet?.description).toContain(DISCLOSURE);
  });

  it("throws when privacy is anything but private or unlisted", () => {
    expect(() => videoResource(baseMeta, cfg("public"))).toThrow(/private.*unlisted/);
  });

  it("throws when the description lacks the disclosure line", () => {
    expect(() => videoResource({ ...baseMeta, description: "Body" }, cfg())).toThrow(/disclosure/);
  });
});

describe("publish", () => {
  let root: string;
  let layout: Layout;
  let api: { [K in keyof YouTubeApi]: ReturnType<typeof vi.fn> };

  const meta = (over: object = {}) =>
    writeFileSync(join(root, "out", "v-test.meta.json"), JSON.stringify({ ...baseMeta, scriptHash: scriptHash(join(root, "scripts", "v-test.yaml")), ...over }));
  const approve = (hash = scriptHash(join(root, "scripts", "v-test.yaml"))) =>
    writeFileSync(join(root, "approvals.json"), JSON.stringify({ "v-test": hash }));
  const run = () => publish("v-test", { layout, api: api as unknown as YouTubeApi, cfg: cfg() });
  const refuses = async (re: RegExp) => {
    await expect(run()).rejects.toThrow(re);
    expect(api.insertVideo).not.toHaveBeenCalled();
    expect(api.setThumbnail).not.toHaveBeenCalled();
    expect(api.insertCaption).not.toHaveBeenCalled();
  };

  beforeEach(() => {
    root = mkdtempSync(join(tmpdir(), "publish-"));
    mkdirSync(join(root, "scripts"));
    mkdirSync(join(root, "out"));
    copyFileSync("test/fixtures/v-test.yaml", join(root, "scripts", "v-test.yaml"));
    for (const ext of ["mp4", "srt", "jpg"]) writeFileSync(join(root, "out", `v-test.${ext}`), "x");
    layout = layoutFor(root, {});
    meta();
    approve();
    api = {
      insertVideo: vi.fn().mockResolvedValue("YT123"),
      setThumbnail: vi.fn().mockResolvedValue(undefined),
      insertCaption: vi.fn().mockResolvedValue(undefined),
    };
  });
  afterEach(() => rmSync(root, { recursive: true, force: true }));

  it("uploads video, thumbnail, and captions, then records the upload", async () => {
    expect(await run()).toBe("YT123");
    expect(api.insertVideo).toHaveBeenCalledOnce();
    expect(api.insertVideo.mock.calls[0][0].status.containsSyntheticMedia).toBe(true);
    expect(api.setThumbnail).toHaveBeenCalledWith("YT123", join(root, "out", "v-test.jpg"));
    expect(api.insertCaption).toHaveBeenCalledWith("YT123", join(root, "out", "v-test.srt"));
    const rec = JSON.parse(readFileSync(join(root, "published.json"), "utf8"))["v-test"];
    expect(rec).toMatchObject({ youtubeId: "YT123", privacy: "private" });
    expect(rec.uploadedAt).toMatch(/^\d{4}-\d\d-\d\dT/);
  });

  it("refuses an id already in published.json, naming the YouTube ID", async () => {
    writeFileSync(join(root, "published.json"), JSON.stringify({ "v-test": { youtubeId: "OLD99", uploadedAt: "x", privacy: "private", thumbnail: true, captions: true } }));
    await refuses(/OLD99/);
  });

  it("a complete entry refusal tells the user to delete the video in Studio first", async () => {
    writeFileSync(join(root, "published.json"), JSON.stringify({ "v-test": { youtubeId: "OLD99", uploadedAt: "x", privacy: "private", thumbnail: true, captions: true } }));
    await expect(run()).rejects.toThrow(/YouTube Studio/);
  });

  it("a failed thumbnail keeps the entry (thumbnail false), names the ID, and a rerun finishes without re-uploading", async () => {
    api.setThumbnail.mockRejectedValueOnce(new Error("quota"));
    await expect(run()).rejects.toThrow(/uploaded as YT123; thumbnail failed: quota; rerun npm run publish -- v-test/);
    const rec = () => JSON.parse(readFileSync(join(root, "published.json"), "utf8"))["v-test"];
    expect(rec()).toMatchObject({ youtubeId: "YT123", thumbnail: false, captions: false });
    expect(await run()).toBe("YT123");
    expect(api.insertVideo).toHaveBeenCalledOnce();
    expect(api.setThumbnail).toHaveBeenCalledTimes(2);
    expect(api.insertCaption).toHaveBeenCalledOnce();
    expect(rec()).toMatchObject({ thumbnail: true, captions: true });
  });

  it("a rerun after only captions failed performs captions alone", async () => {
    api.insertCaption.mockRejectedValueOnce(new Error("boom"));
    await expect(run()).rejects.toThrow(/captions failed: boom/);
    await run();
    expect(api.insertVideo).toHaveBeenCalledOnce();
    expect(api.setThumbnail).toHaveBeenCalledOnce();
    expect(api.insertCaption).toHaveBeenCalledTimes(2);
  });

  it("refuses a fake render", async () => {
    meta({ fake: true });
    await refuses(/fake/i);
  });

  it("refuses a stale build whose script changed since", async () => {
    meta({ scriptHash: "deadbeef" });
    await refuses(/stale|changed/i);
  });

  it("refuses a script that is not approved in the committed approvals.json", async () => {
    approve("0000");
    await refuses(/not approved/);
  });

  it("ignores VIDEOS_APPROVALS: approval is read from the committed file", async () => {
    const other = join(root, "other.json");
    writeFileSync(other, JSON.stringify({ "v-test": scriptHash(join(root, "scripts", "v-test.yaml")) }));
    approve("0000");
    layout = layoutFor(root, { VIDEOS_APPROVALS: other });
    await refuses(/not approved/);
  });

  for (const ext of ["mp4", "srt", "jpg", "meta.json"]) {
    it(`refuses when ${ext} is missing`, async () => {
      rmSync(join(root, "out", `v-test.${ext}`));
      await refuses(new RegExp(`v-test\\.${ext.replace(".", "\\.")}`));
      expect(existsSync(join(root, "published.json"))).toBe(false);
    });
  }
});
