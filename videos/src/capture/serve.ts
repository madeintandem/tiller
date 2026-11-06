import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

const MARKS = resolve(import.meta.dirname, "../../fixture/marks");

function freePort(): Promise<number> {
  return new Promise((res, rej) => {
    const s = createServer();
    s.once("error", rej);
    s.listen(0, "127.0.0.1", () => {
      const { port } = s.address() as { port: number };
      s.close(() => res(port));
    });
  });
}

async function waitFor200(url: string, ms: number, abort: { stopped: boolean }): Promise<void> {
  const end = Date.now() + ms;
  while (Date.now() < end && !abort.stopped) {
    try {
      if ((await fetch(url)).status === 200) return;
    } catch {
      // not listening yet
    }
    await new Promise((r) => setTimeout(r, 150));
  }
  if (abort.stopped) return;
  throw new Error(`tiller serve did not answer 200 at ${url} within ${ms} ms`);
}

// Extracts a saved mark to a temp dir and serves it with the pinned binary.
export async function serveMark(mark: string): Promise<{ url: string; dir: string; stop(): Promise<void> }> {
  const dir = mkdtempSync(join(tmpdir(), `tiller-mark-${mark}-`));
  let child: ReturnType<typeof spawn> | undefined;
  const abort = { stopped: false };
  const stop = async () => {
    abort.stopped = true;
    const c = child;
    if (c && c.exitCode === null && c.signalCode === null) {
      const exited = new Promise<void>((r) => c.once("exit", () => r()));
      c.kill("SIGTERM");
      const t = setTimeout(() => c.kill("SIGKILL"), 3000);
      await exited;
      clearTimeout(t);
    }
    rmSync(dir, { recursive: true, force: true });
  };
  try {
    execFileSync("tar", ["xzf", join(MARKS, `${mark}.tar.gz`), "-C", dir]);
    const port = await freePort();
    const url = `http://127.0.0.1:${port}`;
    const c = spawn(process.env.TILLER || "tiller", ["serve", "--port", String(port), "--open=false"], {
      cwd: dir,
      stdio: "ignore",
      env: { ...process.env, BROWSER: "none" },
    });
    child = c;
    await Promise.race([
      waitFor200(`${url}/`, 20000, abort),
      new Promise<never>((_, rej) => {
        c.once("exit", (code) => rej(new Error(`tiller serve exited early (code ${code})`)));
        c.once("error", rej);
      }),
    ]);
    return { url, dir, stop };
  } catch (e) {
    await stop();
    throw e;
  }
}
