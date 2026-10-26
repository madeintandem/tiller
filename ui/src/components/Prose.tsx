// Prose is the console's safe Markdown renderer. Node prose and FileDetail's
// loose-file reader both pass served content through this same component, so
// heading structure, GFM support, raw-HTML escaping, and link policy cannot
// drift between destinations.
//
// Three deliberate postures, all load-bearing:
//
// 1. No rehype-raw, anywhere. react-markdown's default pipeline never turns
//    raw HTML in the source (a stray <script>, a pasted <b>) into a real DOM
//    element: mdast's "html" node survives remark-rehype as literal text, so
//    it renders back out escaped, the same as any other paragraph content.
//    React's own child-escaping is the whole XSS posture here, exactly as
//    ProseSection's own header comment already promised for the pre-block it
//    replaces; adding rehype-raw would reintroduce exactly the risk that
//    comment rules out, so this component never reaches for it.
//
// 2. Every heading level demotes by one (h1->h2 ... h6 stays h6, clamped:
//    HTML has no h7). Rendered Markdown always sits inside a page surface,
//    whether that is a node section or FileDetail's Explore reading pane, so
//    its headings must remain subordinate to the surrounding shell. A source
//    "# Release notes" therefore becomes the file reader's h2, while a node's
//    documented "## Dependencies" becomes an h3 styled by prose.css.
//
// 3. /node/... links stay in the console; HTTPS references open a protected
//    new tab. All other hrefs stay inert, and images render only their alt
//    text so reading prose cannot fetch tracking images. Bare work IDs become
//    node links in the Markdown AST, leaving existing links and code blocks
//    untouched. Node addresses resolve on navigation, not at render time.

import type { ReactNode } from "react";
import type { Link as MarkdownLink, PhrasingContent, Root, RootContent } from "mdast";
import type { Components } from "react-markdown";
import ReactMarkdown from "react-markdown";
import { Link } from "react-router-dom";
import remarkGfm from "remark-gfm";

// Component-scoped stylesheet, so it rides this component's own import graph
// (the convention: global layers like tokens.css load in main.tsx instead).
import "../styles/prose.css";

// Inline-vs-block code needs NO component here: prose.css discriminates by
// DOM shape (`.prose :not(pre) > code` tints inline, `.prose pre code` takes
// block chrome), which remark-rehype guarantees — block code is always
// pre-wrapped. An earlier className-keyed component misfiled language-less
// fences, whose code carries no className at all, exactly like inline code.

// Keep external navigation explicit: no protocol-relative URLs or browser
// normalization of whitespace/backslashes into an unintended destination.
function ProseLink({ href, children }: { href?: string; children?: ReactNode }) {
  if (href !== undefined && href.startsWith("/node/")) {
    return <Link to={href}>{children}</Link>;
  }
  if (href !== undefined && /^https:\/\/[^\s\\]+$/i.test(href)) {
    try {
      const url = new URL(href);
      if (url.protocol === "https:" && url.hostname !== "") {
        return (
          <a className="ext" href={href} target="_blank" rel="noopener noreferrer">
            {children}
            <span className="visually-hidden"> (opens in a new tab)</span>
          </a>
        );
      }
    } catch {
      // Malformed destinations remain readable, but cannot navigate.
    }
  }
  return <>{children}</>;
}

const WORK_NOUNS: Record<string, string> = { E: "epic", F: "feature", T: "task" };
// Do not link fragments of identifiers, paths, or filenames. A sentence's
// trailing period is punctuation, but ".md" is part of a filename.
const WORK_REFERENCE = /(?<![\p{L}\p{N}_/@.-])[EFT]-[a-z0-9]{6}(?![\p{L}\p{N}_/-]|\.[\p{L}\p{N}_])/gu;
const WORK_ID = /^[EFT]-[a-z0-9]{6}$/;

function workLink(id: string, child: PhrasingContent): MarkdownLink {
  return { type: "link", url: `/node/${WORK_NOUNS[id[0]]}/${id}`, children: [child] };
}

function remarkWorkReferences() {
  function transform(node: Root | RootContent): void {
    // Never descend into authored links, including reference-style links.
    if (node.type === "link" || node.type === "linkReference" || !("children" in node)) {
      return;
    }
    for (let i = 0; i < node.children.length; i++) {
      const child = node.children[i];
      if (child.type === "inlineCode" && WORK_ID.test(child.value)) {
        node.children.splice(i, 1, workLink(child.value, child));
      } else if (child.type === "text") {
        const parts: PhrasingContent[] = [];
        let end = 0;
        for (const match of child.value.matchAll(WORK_REFERENCE)) {
          if (match.index > end) {
            parts.push({ type: "text", value: child.value.slice(end, match.index) });
          }
          parts.push(workLink(match[0], { type: "text", value: match[0] }));
          end = match.index + match[0].length;
        }
        if (end === 0) continue;
        if (end < child.value.length) {
          parts.push({ type: "text", value: child.value.slice(end) });
        }
        node.children.splice(i, 1, ...parts);
        i += parts.length - 1;
      } else {
        transform(child);
      }
    }
  }
  return transform;
}

const REMARK_PLUGINS = [remarkGfm, remarkWorkReferences];

const COMPONENTS: Components = {
  h1: "h2",
  h2: "h3",
  h3: "h4",
  h4: "h5",
  h5: "h6",
  h6: "h6",
  a: ProseLink,
  img: ({ alt }) => <>{alt}</>,
};

export default function Prose({ markdown }: { markdown: string }) {
  return (
    <div className="prose">
      <ReactMarkdown remarkPlugins={REMARK_PLUGINS} components={COMPONENTS}>
        {markdown}
      </ReactMarkdown>
    </div>
  );
}

// INLINE_COMPONENTS is COMPONENTS with the paragraph unwrapped: a reason
// clause or any other one-line string rendered inside an existing element
// must not introduce a block, so a paragraph becomes its own children and
// nothing else changes (the same link policy, the same heading demotion,
// which a one-liner never reaches).
const INLINE_COMPONENTS: Components = {
  ...COMPONENTS,
  p: ({ children }) => <>{children}</>,
};

// ProseInline is Prose for a string that lives inside a line: the route
// strip's reason clause, where the walk's own wording carries a
// backticked command that should read as a code span, not as literal
// backticks. Same renderer, same three postures, a span instead of a div.
export function ProseInline({ markdown }: { markdown: string }) {
  return (
    <span className="prose-inline">
      <ReactMarkdown remarkPlugins={REMARK_PLUGINS} components={INLINE_COMPONENTS}>
        {markdown}
      </ReactMarkdown>
    </span>
  );
}
