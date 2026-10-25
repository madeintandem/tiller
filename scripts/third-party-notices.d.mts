export function goToolchain(goMod: string): string;

// Structural Vite/Rolldown plugin interface, shared without depending on either
// surface's separately installed build-tool version.
export function noticesPlugin(surface: "ui" | "site"): {
  name: string;
  apply: "build";
  generateBundle(
    this: { emitFile(file: { type: "asset"; fileName: string; source: string }): string },
    options: unknown,
    bundle: unknown,
  ): void;
};

export function pagefindSourceArchive(): string;
export function pagefindEmittedJavaScriptNotices(directory: string, version: string): string;
export function noticesIntegration(): {
  name: string;
  hooks: { "astro:build:done": (options: { dir: URL }) => void };
};
