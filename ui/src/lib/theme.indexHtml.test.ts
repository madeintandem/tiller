// Vitest coverage for the theme.ts<->index.html seam: index.html carries an
// inline pre-paint twin of bootTheme (see that file's own header comment,
// and theme.ts's THEME_STORAGE_KEY comment which points back at it) that
// hand-duplicates the storage key and the 'light'/'dark' literals rather
// than importing theme.ts, since the inline script must run before Vite's
// module script can even be requested. Nothing type-checks that duplication,
// so a rename on either side (the key, or either literal) would silently
// strand the other - the app would still build and every other test would
// still pass, and only a real reload with a stored pick would show the
// flash this twin exists to prevent. This file reads index.html's own
// literal source (via Vite's `?raw` import, the same mechanism
// states.tokens.test.ts uses for tokens.css) and asserts the twin still
// says what theme.ts says.

import { describe, expect, it } from "vitest";

import indexHtml from "../../index.html?raw";

import { THEME_STORAGE_KEY } from "./theme";

describe("index.html's inline pre-paint script stays in sync with theme.ts", () => {
  it(`reads the same storage key theme.ts writes ("${THEME_STORAGE_KEY}")`, () => {
    expect(indexHtml).toContain(`localStorage.getItem('${THEME_STORAGE_KEY}')`);
  });

  it("guards on exactly the 'light' and 'dark' literals theme.ts's ThemePick uses", () => {
    expect(indexHtml).toMatch(/t === 'light' \|\| t === 'dark'/);
  });
});
