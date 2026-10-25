// Vitest coverage for Prose (task B2): the component that typesets a node's
// markdown body instead of dumping it in ProseSection's pre-block. Written
// before the implementation exists, per the branch's TDD discipline; run
// once red, then made to pass.

import { cleanup, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it } from "vitest";

import Prose, { ProseInline } from "../Prose";

afterEach(() => {
  cleanup();
});

function renderProse(markdown: string) {
  return render(
    <MemoryRouter>
      <Prose markdown={markdown} />
    </MemoryRouter>,
  );
}

function renderInline(markdown: string) {
  return render(
    <MemoryRouter>
      <ProseInline markdown={markdown} />
    </MemoryRouter>,
  );
}

describe("Prose headings", () => {
  it("demotes a markdown h2 to an h3 so the page's own h2 stays unique", () => {
    renderProse("## Dependencies\n\nRoot epic, no epic-level predecessor.");

    const heading = screen.getByRole("heading", { level: 3, name: "Dependencies" });
    expect(heading.tagName).toBe("H3");
    expect(screen.queryByRole("heading", { level: 2 })).not.toBeInTheDocument();
  });

  it("demotes every other level by one, clamping h6 at h6 (nowhere lower to go)", () => {
    renderProse("# One\n\n### Three\n\n###### Six");

    expect(screen.getByRole("heading", { level: 2, name: "One" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 4, name: "Three" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 6, name: "Six" })).toBeInTheDocument();
  });

  it("demotes the middle levels too: h4 to h5 and h5 to h6", () => {
    renderProse("#### Four\n\n##### Five");

    expect(screen.getByRole("heading", { level: 5, name: "Four" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { level: 6, name: "Five" })).toBeInTheDocument();
  });
});

describe("Prose lists", () => {
  it("renders a markdown list as real list markup", () => {
    renderProse("- one\n- two\n- three");

    const list = screen.getByRole("list");
    const items = screen.getAllByRole("listitem");
    expect(list).toBeInTheDocument();
    expect(items.map((li) => li.textContent)).toEqual(["one", "two", "three"]);
  });
});

describe("Prose code", () => {
  // prose.css discriminates inline from block code purely by DOM shape:
  // `.prose :not(pre) > code` tints, `.prose pre code` takes block chrome.
  // The selector is mirrored verbatim here and asserted via Element.matches,
  // so drift in either place turns a test red. No JS classification exists
  // (the old className-keyed Code component misfiled language-less fences,
  // whose code carries no className at all, exactly like inline code).
  const INLINE_TINT_SELECTOR = ".prose :not(pre) > code";

  it("renders inline code as a bare code element the inline tint selector matches", () => {
    const { container } = renderProse("See the `epic.eval.md` file.");

    const code = container.querySelector("code");
    expect(code).not.toBeNull();
    expect(code?.textContent).toBe("epic.eval.md");
    expect(code?.className).toBe("");
    expect(code?.matches(INLINE_TINT_SELECTOR)).toBe(true);
  });

  it("keeps a language-tagged fence pre-wrapped, outside the inline tint selector", () => {
    const { container } = renderProse("```js\nconst x = 1;\n```");

    const code = container.querySelector("pre > code");
    expect(code).not.toBeNull();
    expect(code?.matches(INLINE_TINT_SELECTOR)).toBe(false);
  });

  it("keeps a language-less fence pre-wrapped and untinted, never misfiled as inline", () => {
    const { container } = renderProse("```\nplain fence\n```");

    const code = container.querySelector("pre > code");
    expect(code).not.toBeNull();
    // The regression this pins: remark-rehype passes NO className for a
    // language-less fence (verified by probe), so any classification keyed
    // on className misfiles this shape as inline. Class absence plus
    // selector mismatch is the whole contract.
    expect(code?.className).toBe("");
    expect(code?.matches(INLINE_TINT_SELECTOR)).toBe(false);
  });
});

describe("Prose GFM", () => {
  it("renders a GFM table as a real table (the remark-gfm dependency's coverage)", () => {
    renderProse("| a | b |\n| --- | --- |\n| 1 | 2 |");

    expect(screen.getByRole("table")).toBeInTheDocument();
    expect(screen.getByRole("columnheader", { name: "a" })).toBeInTheDocument();
    expect(screen.getByRole("cell", { name: "2" })).toBeInTheDocument();
  });
});

describe("Prose raw HTML", () => {
  it("renders a raw script tag as literal text, never as a real element", () => {
    const { container } = renderProse("Payload: <script>alert(1)</script> after.");

    expect(container.querySelector("script")).toBeNull();
    expect(container.textContent).toContain("<script>alert(1)</script>");
  });

  it("renders another raw HTML tag as literal text too, not as a real element", () => {
    const { container } = renderProse("<b>not actually bold</b>");

    expect(container.querySelector("b")).toBeNull();
    expect(container.textContent).toContain("<b>not actually bold</b>");
  });
});

describe("Prose links", () => {
  it("renders an internal /node/... href as an in-tab react-router link", () => {
    renderProse("See [the identity feature](/node/feature/F-pueftm) for context.");

    const link = screen.getByRole("link", { name: "the identity feature" });
    expect(link).toHaveAttribute("href", "/node/feature/F-pueftm");
    expect(link).not.toHaveAttribute("target");
  });

  it("renders a relative (non /node/...) href as plain, non-clickable text too", () => {
    const { container } = renderProse("See [the design doc](../design/04-phase.md) for context.");

    expect(screen.queryByRole("link", { name: "the design doc" })).not.toBeInTheDocument();
    expect(container.textContent).toContain("the design doc");
  });

  // The two active-payload schemes, pinned so no future config change
  // (adding rehype plugins, touching urlTransform, widening the allow-list)
  // can silently reintroduce a live anchor. Two layers currently stand
  // behind each assertion: react-markdown's default urlTransform strips the
  // scheme AND the /node/ allow-list refuses the href; the pin is on the
  // observable outcome, so it holds as long as either layer survives.
  it("neuters a javascript: href: no anchor element at all", () => {
    const { container } = renderProse("[click me](javascript:alert(1))");

    expect(container.querySelector("a")).toBeNull();
    expect(container.textContent).toContain("click me");
  });

  it("neuters a data: href: no anchor element at all", () => {
    const { container } = renderProse(
      "[payload](data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==)",
    );

    expect(container.querySelector("a")).toBeNull();
    expect(container.textContent).toContain("payload");
  });

  it("renders a mailto: href as plain text per the allow-list", () => {
    const { container } = renderProse("Mail [the owner](mailto:jc@example.com) about it.");

    expect(container.querySelector("a")).toBeNull();
    expect(container.textContent).toContain("the owner");
  });

  it("renders a #fragment href as plain text per the allow-list", () => {
    const { container } = renderProse("See [the section](#dependencies) below.");

    expect(container.querySelector("a")).toBeNull();
    expect(container.textContent).toContain("the section");
  });
});

describe.each([
  ["block", renderProse],
  ["inline", renderInline],
] as const)("Prose %s reference safety", (_form, renderMarkdown) => {
  it("opens authored HTTPS links in protected new tabs without nesting ID links", () => {
    const { container } = renderMarkdown(
      "[F-79dh3q](https://example.com/docs?q=reference#section) and [guide][home]; T-k3p9w6\n\n[home]: https://example.com/guide",
    );
    const links = screen.getAllByRole("link");
    expect(links).toHaveLength(3);
    expect(links[0]).toHaveAttribute("href", "https://example.com/docs?q=reference#section");
    expect(links[1]).toHaveAttribute("href", "https://example.com/guide");
    for (const link of links.slice(0, 2)) {
      expect(link).toHaveAttribute("target", "_blank");
      expect(link).toHaveAttribute("rel", "noopener noreferrer");
      expect(link).toHaveAccessibleName(expect.stringContaining("opens in a new tab"));
    }
    expect(container.querySelector("a a")).toBeNull();
    expect(links[2]).toHaveAttribute("href", "/node/task/T-k3p9w6");
    expect(links[2]).not.toHaveAttribute("target");
  });

  it.each([
    "http://example.com",
    "//example.com",
    "ftp://example.com/file",
    "mailto:owner@example.com",
    "javascript:alert%281%29",
    "data:text/html;base64,PHNjcmlwdD4=",
    "../design.md",
    "/file/design.md",
    "#section",
    "https://",
  ])("keeps denied destination %s readable but non-navigable", (href) => {
    const { container } = renderMarkdown(`[reference](${href})`);
    expect(container.querySelector("a")).toBeNull();
    expect(container.textContent).toContain("reference");
  });

  it("never mounts Markdown or raw HTML images, retaining inert alt text", () => {
    const { container } = renderMarkdown([
      "![remote F-79dh3q](https://example.com/tracker.png)",
      "![local](/file/private.png)",
      "![embedded](data:image/png;base64,aGVsbG8=)",
      "![reference image][tracker]",
      '<img src="https://example.com/raw-tracker.png" alt="raw">',
      "",
      "[tracker]: https://example.com/reference-tracker.png",
    ].join("\n"));
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("a")).toBeNull();
    for (const alt of ["remote F-79dh3q", "local", "embedded", "reference image"]) {
      expect(container.textContent).toContain(alt);
    }
    expect(container.textContent).toContain('<img src="https://example.com/raw-tracker.png" alt="raw">');
  });
});

describe("Prose work references", () => {
  it("links IDs across prose, formatting, lists, tables, and standalone code spans", () => {
    renderProse([
      "## Related E-j8m4q2",
      "",
      "Depends on **F-79dh3q**, then (`T-k3p9w6`).",
      "",
      "- E-j8m4q2 and T-k3p9w6.",
      "",
      "| Feature |",
      "| --- |",
      "| F-79dh3q |",
    ].join("\n"));

    for (const [id, noun] of [
      ["E-j8m4q2", "epic"],
      ["F-79dh3q", "feature"],
      ["T-k3p9w6", "task"],
    ]) {
      const links = screen.getAllByRole("link", { name: id });
      expect(links).toHaveLength(2);
      for (const link of links) {
        expect(link).toHaveAttribute("href", `/node/${noun}/${id}`);
      }
    }
    expect(screen.getAllByRole("link", { name: "T-k3p9w6" })[0].querySelector("code"))
      .toHaveTextContent("T-k3p9w6");
  });

  it("leaves commands, code blocks, existing links, and lookalikes untouched", () => {
    const { container } = renderProse([
      "`tiller show feature F-79dh3q`",
      "",
      "```",
      "E-j8m4q2",
      "```",
      "",
      "    T-k3p9w6",
      "",
      "[F-79dh3q](/node/feature/explicit-target)",
      "[T-k3p9w6](https://example.com)",
      "[E-j8m4q2][explicit]",
      "",
      "[explicit]: /node/epic/explicit-target",
      "",
      "F-79dh3q1 F-79dh3 F-79DH3Q f-79dh3q prefixF-79dh3q F-79dh3q-suffix",
      "F-79dh3q_suffix path/F-79dh3q F-79dh3q.md Q-h2n7m5",
    ].join("\n"));

    expect(screen.getAllByRole("link").map((link) => link.getAttribute("href"))).toEqual([
      "/node/feature/explicit-target",
      "https://example.com",
      "/node/epic/explicit-target",
    ]);
    expect(container.querySelector("a a")).toBeNull();
    expect(container.querySelector("pre")).toHaveTextContent("E-j8m4q2");
    expect(container.textContent).toContain("tiller show feature F-79dh3q");
  });

  it("links work IDs in inline prose without turning commands into links", () => {
    renderInline("Check F-79dh3q with `tiller check feature F-79dh3q`.");
    expect(screen.getAllByRole("link")).toHaveLength(1);
    expect(screen.getByRole("link", { name: "F-79dh3q" }))
      .toHaveAttribute("href", "/node/feature/F-79dh3q");
  });
});

describe("ProseInline", () => {
  it("renders inline code as a code element inside a span with no paragraph wrapper", () => {
    const { container } = renderInline(
      'Answer Q-1 first with `tiller answer question Q-1 --answer "..."`',
    );
    const span = container.querySelector("span.prose-inline");
    expect(span).not.toBeNull();
    expect(span?.querySelector("p")).toBeNull();
    expect(span?.querySelector("code")?.textContent).toBe('tiller answer question Q-1 --answer "..."');
    expect(span?.textContent).not.toContain("`");
    expect(span?.querySelector("code")?.matches(".prose-inline code")).toBe(true);
  });

  it("keeps posture 1: raw HTML in an inline string renders as escaped text, never markup", () => {
    const { container } = renderInline("before <b>bold</b> after");
    expect(container.querySelector("b")).toBeNull();
    expect(container.textContent).toContain("<b>bold</b>");
  });
});

