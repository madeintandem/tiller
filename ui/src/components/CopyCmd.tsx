// CopyCmd is the chart-table pass's one shared copy affordance (Global
// Constraint: every command copies through ONE shared component with a
// "Copied" toast). It replaces the pass-1 CopyCommand, deleted outright on
// this branch rather than left standing beside it, and differs from it
// deliberately: the whole chip is the button, and the confirmation is a
// single toast shared by every CopyCmd on the page rather than a
// per-instance label swap, so copying two commands in a row reads as one
// running conversation, not two independent widgets. Magenta is opt-in only:
// accent must be passed explicitly true, never a default, per the Global
// Constraint that magenta appears only where something needs the human.

function clipboardOrNull(): Clipboard | null {
  if (typeof navigator === "undefined") {
    return null;
  }
  return (navigator as { clipboard?: Clipboard }).clipboard ?? null;
}

// The toast is a singleton DOM node appended once to document.body and reused
// by every CopyCmd, so "single shared toast root" holds no matter how many
// copy chips are mounted: they all address the same physical element rather
// than each rendering (and animating) their own.
let toastEl: HTMLDivElement | null = null;
let hideTimer: ReturnType<typeof setTimeout> | undefined;

function toastRoot(): HTMLDivElement {
  if (toastEl === null) {
    toastEl = document.createElement("div");
    toastEl.className = "copy-toast";
    toastEl.setAttribute("role", "status");
    toastEl.setAttribute("aria-live", "polite");
    document.body.appendChild(toastEl);
  }
  return toastEl;
}

function showToast(message: string) {
  const el = toastRoot();
  el.textContent = message;
  el.classList.add("visible");
  window.clearTimeout(hideTimer);
  hideTimer = window.setTimeout(() => {
    el.classList.remove("visible");
  }, 1500);
}

// selectionCopy is the path for a console served on plain http beyond
// localhost: that page is not a secure context, so the browser withholds
// navigator.clipboard entirely, but a click still grants the older
// select-and-copy command. A throwaway offscreen textarea carries the text.
function selectionCopy(text: string): boolean {
  if (typeof document.execCommand !== "function") {
    return false;
  }
  const area = document.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.style.position = "fixed";
  area.style.top = "-1000px";
  area.style.opacity = "0";
  const prior = document.activeElement as HTMLElement | null;
  document.body.appendChild(area);
  area.focus();
  area.select();
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } catch {
    ok = false;
  }
  area.remove();
  prior?.focus();
  return ok;
}

function fallback(text: string) {
  if (selectionCopy(text)) {
    showToast("Copied");
  }
  // Otherwise a refused copy is fine: the text is right there to select.
}

function copy(text: string) {
  const clipboard = clipboardOrNull();
  if (clipboard === null) {
    fallback(text);
    return;
  }
  clipboard
    .writeText(text)
    .then(() => {
      showToast("Copied");
    })
    .catch(() => {
      fallback(text);
    });
}

// CopyRef is the same affordance in the header's quiet register: the node's
// ref reads exactly as it did as a plain span, and a click copies the
// paste-able id rather than the display (a keyed node's ref is "key (id)",
// which no command accepts whole). It rides the clipboard path and the one
// toast root above so every copy on the page stays one running conversation,
// and its icon stays faint, never magenta: nothing here needs the human.
export function CopyRef({ display, id }: { display: string; id: string }) {
  return (
    <button type="button" className="copy-ref" title="Copy id" onClick={() => copy(id)}>
      <span className="mono">{display}</span>
      <span className="copy-ref-icon" aria-hidden="true">
        ⧉
      </span>
    </button>
  );
}

export default function CopyCmd({ cmd, accent }: { cmd: string; accent?: boolean }) {
  return (
    <button
      type="button"
      className={accent === true ? "copy-cmd copy-cmd-accent" : "copy-cmd"}
      onClick={() => copy(cmd)}
    >
      <code>{cmd}</code>
      <span className="copy-cmd-icon" aria-hidden="true">
        ⧉
      </span>
    </button>
  );
}
