import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
// Self-hosted @font-face declarations (Space Grotesk, Inter, Space Mono,
// Task 1). Registering a font family has no cascade-order interaction with
// the rule sheets below: a @font-face block only takes effect once some rule
// references the family, so where this import lands relative to the others
// is inert. tokens.css below is what references these families now (Task 2),
// which is what actually triggers a download.
import "./styles/fonts.css";
// The shared skin loads before App so the view stylesheets App pulls in land
// after it in the bundle: styles.css is the base layer, and a view's
// equal-specificity override of a shared primitive must win the cascade.
import "./styles.css";
// The console-v2 design-token layer (Task 2): the role-named custom
// properties (surfaces, inks, the seven --st-* state pairs) plus the shared
// primitives built on them (StateDot, StateBar, Chip, CopyCmd, SectionHead).
// Every sheet in the app, styles.css above included, now reads these
// properties, and that survives loading second because a custom property
// resolves where it is USED, not where it is declared: both files declare on
// :root, so the whole set is in scope for every rule regardless of which
// sheet landed first. Only the rule order below matters, and that is the
// base-layer ordering the comment above describes.
import "./styles/tokens.css";
import App from "./App";
import { bootTheme } from "./lib/theme";

// Apply the persisted theme (Task 3): a top-level synchronous call, not a
// React effect, so it runs before createRoot().render() below rather than
// after React's first commit - an effect only runs once the browser has
// already painted, which is the flash (dark pick, light first frame) this
// ordering avoids for everything after this point. The actual pre-paint
// guarantee is index.html's inline script, since this module script is
// still one Vite can defer past first paint; this call is what keeps the
// attribute correct (and re-normalized) for the rest of the app's lifetime.
// bootTheme reads localStorage itself (lib/theme.ts), folding a missing or
// garbage value to "auto" the same way the control does.
bootTheme();

const root = document.getElementById("root");
if (root === null) {
  throw new Error("index.html is missing the #root element");
}

createRoot(root).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);
