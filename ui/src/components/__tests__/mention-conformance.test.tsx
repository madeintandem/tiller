import { cleanup, render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";
import examples from "../../../../testdata/work-mentions.json";
import Prose from "../Prose";

afterEach(cleanup);

describe("shared Markdown relationship conformance", () => {
  for (const example of examples) {
    it(example.name, () => {
      const { container } = render(<MemoryRouter><Prose markdown={example.markdown} /></MemoryRouter>);
      expect(Array.from(container.querySelectorAll('a[href^="/node/"]')).map((link) => link.textContent))
        .toEqual(example.mentions.map((mention) => mention.id));
    });
  }
});
