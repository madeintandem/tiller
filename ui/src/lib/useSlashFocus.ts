import { type RefObject, useEffect } from "react";

// Anything a reader types into. A "/" typed here is text (or a combobox
// keystroke), never the shortcut.
const TYPING_TARGET = 'input, textarea, select, [contenteditable]:not([contenteditable="false"])';

// useSlashFocus focuses the input when "/" is pressed with no modifier and
// the key was not aimed at another typing target. The slash is swallowed so
// it is not typed into the field it just focused.
export function useSlashFocus(ref: RefObject<HTMLInputElement | null>): void {
  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== "/" || event.metaKey || event.ctrlKey || event.altKey || event.defaultPrevented) {
        return;
      }
      if (event.target instanceof Element && event.target.closest(TYPING_TARGET) !== null) {
        return;
      }
      if (ref.current === null) {
        return;
      }
      event.preventDefault();
      ref.current.focus();
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [ref]);
}
