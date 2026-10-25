/// <reference types="vitest/config" />
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import { noticesPlugin } from "../scripts/third-party-notices.mjs";

// Production assets build into dist, which ui/embed.go embeds into the tiller
// binary. Dev mode proxies /api to a running `tiller serve` on its default
// port so the vite dev server and the real read API work side by side.
export default defineConfig({
  plugins: [react(), noticesPlugin("ui")],
  build: {
    outDir: "dist",
  },
  server: {
    proxy: {
      "/api": "http://127.0.0.1:4400",
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: "./src/test/setup.ts",
    css: {
      // Vitest stubs every *.css request to an empty module by default (a
      // perf shortcut, since jsdom tests almost never need real style
      // output). states.tokens.test.ts reads tokens.css's literal source via
      // a `?raw` import to check the TS<->CSS token-name seam, so it needs
      // the real file content; scoped to those files rather than flipping
      // CSS processing on for the whole suite.
      // history.css joined for the phone-width pass (whole-branch merge
      // gate finding): history.css.test.ts pins .hrow's 48rem flex-wrap
      // override and its position after the base rule, the same source-
      // order property task 25's own fix wave test caught a real bug in.
      // home.css joined the same pass: home.css.test.ts pins .card-grid's
      // 430px minmax floor dropping to an unconstrained 1fr below 48rem,
      // the fix for the one full-page horizontal scroll this pass found.
      // board.css, build.css, and workitems.css: board.css.test.ts pins the
      // static heads row above each board's scroller, and that no head on
      // either board sticks inside it. styles.css: styles.css.test.ts pins
      // the topbar sticking to the viewport.
      include: [
        /components\/board\.css/,
        /src\/styles\.css/,
        /styles\/tokens\.css/,
        /views\/build\.css/,
        /views\/history\.css/,
        /views\/home\.css/,
        /views\/workitems\.css/,
      ],
    },
  },
});
