# site/screenshots

The machinery behind the console screenshots on the documentation site, kept
here so they can be reshot as the console evolves instead of rebuilt by hand.

Two parts:

- `acme.sh` builds the Acme Platform stand-in project, a greenfield project
  caught mid-flight, entirely through the `tiller` CLI: one feature merged and
  synced, one in motion per owner, a plan approved and waiting to be claimed,
  one epic awaiting approval, one open question, and bugs, chores, and tweaks on
  the shelves. Acme is the neutral stand-in the specs and fixtures already
  use; nothing in it names a real client or a real person.
- `shoot.mjs` drives a running console in WebKit and writes the site's
  images: Home, the Explore map with a feature selected, and the Fix
  lens, each in light and dark, 1440x780, converted to WebP.

## Reshoot

From the repository root, with the binary you want pictured built and on
your PATH (or named in `TILLER`), and `cwebp` installed (`brew install webp`):

```sh
cd site/screenshots
npm ci && npx playwright install webkit          # once
TILLER=$(command -v tiller) npm run project      # builds /tmp/tiller-shots/acme
(cd /tmp/tiller-shots/acme && tiller serve --port 4477 &)
npm run shoot -- 4477                             # writes ../public/console-*.webp
```

Then look at the images, commit the ones that changed, and stop the serve.
The site's pages reference them by name (`index.mdx` and `console.md`), so
adding a shot means adding an `<img>` pair there as well, one per theme,
using the `light:sl-hidden` and `dark:sl-hidden` classes the existing pairs
use.

The project script is deliberately verbose about state: every gate it takes
is one the console shows, so if a screenshot needs a state the script does
not reach, add the commands there rather than editing `.tiller/` by hand.
Keep it runnable against the current binary; when a command's shape changes,
this script is one of the places that finds out.
