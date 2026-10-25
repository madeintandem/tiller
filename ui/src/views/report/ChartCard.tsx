// ChartCard lets a reader enlarge one Report chart. It renders its chart
// twice: once on the page, where ChartFrame shows a magnifier button, and,
// while open, again inside a native modal <dialog>, where the frame draws the
// chart taller and with no button. Both copies are the live chart, so
// tooltips work in either. The native dialog brings focus handling and
// Escape; a press and release on the backdrop and the Close button close it
// too, and focus goes back to the button that opened it.

import { createContext, useId, useRef, useState, type MouseEvent, type ReactNode } from "react";

export interface ChartExpandState {
  // expanded is true for the copy drawn inside the dialog.
  expanded: boolean;
  // onExpand opens the dialog. The frame passes the button that asked, so
  // focus can return to it on close.
  onExpand?: (opener: HTMLElement) => void;
  // titleId is the id the enlarged copy puts on its heading, which names
  // the dialog.
  titleId?: string;
}

export const ChartExpand = createContext<ChartExpandState>({ expanded: false });

// onBackdrop is true when a pointer event lands on ::backdrop: it targets the
// dialog element itself but lies outside the panel's box. A press on the
// panel's own scrollbar also targets the dialog, but inside the box.
function onBackdrop(e: MouseEvent<HTMLDialogElement>): boolean {
  if (e.target !== e.currentTarget) return false;
  const box = e.currentTarget.getBoundingClientRect();
  return e.clientX < box.left || e.clientX > box.right || e.clientY < box.top || e.clientY > box.bottom;
}

export default function ChartCard({ children }: { children: ReactNode }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const opener = useRef<HTMLElement | null>(null);
  const pressedBackdrop = useRef(false);
  const [open, setOpen] = useState(false);
  const titleId = useId();

  // The dialog opens before the enlarged copy mounts in it, so the copy
  // measures the open dialog's width on its first render rather than the
  // zero width of a closed one. The click's update renders before the next
  // paint, so the empty dialog is never seen.
  function expand(button: HTMLElement) {
    opener.current = button;
    dialog.current?.showModal();
    setOpen(true);
  }

  function handleClose() {
    setOpen(false);
    opener.current?.focus();
  }

  return (
    <>
      <ChartExpand.Provider value={{ expanded: false, onExpand: expand }}>{children}</ChartExpand.Provider>
      <dialog
        ref={dialog}
        className="report-dialog"
        aria-labelledby={titleId}
        onClose={handleClose}
        // A click closes only when its press also began on the backdrop, so
        // a drag that starts in the panel and ends outside it does not.
        onPointerDown={(e) => {
          pressedBackdrop.current = onBackdrop(e);
        }}
        onClick={(e) => {
          if (pressedBackdrop.current && onBackdrop(e)) dialog.current?.close();
          pressedBackdrop.current = false;
        }}
      >
        {open && (
          <div className="report-dialog-body">
            <button
              type="button"
              className="report-icon-button report-dialog-close"
              aria-label="Close"
              // The dialog opened empty, so its own focusing found nothing;
              // Close takes focus as it mounts.
              autoFocus
              onClick={() => dialog.current?.close()}
            >
              <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
                <path d="M4 4l8 8M12 4l-8 8" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
              </svg>
            </button>
            <ChartExpand.Provider value={{ expanded: true, titleId }}>{children}</ChartExpand.Provider>
          </div>
        )}
      </dialog>
    </>
  );
}
