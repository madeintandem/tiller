import "@testing-library/jest-dom/vitest";

// jsdom does not implement scrollIntoView; the navigator reveals the current
// row with it, so every test that mounts the navigator needs a no-op.
if (typeof Element.prototype.scrollIntoView !== "function") {
  Element.prototype.scrollIntoView = () => undefined;
}
