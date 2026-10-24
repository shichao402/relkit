import * as React from "react";

/** Remembering where focus was before a dialog opened.
 *
 *  Radix returns focus to a DialogTrigger on close. Every dialog here is
 *  controlled by an `open` prop and has no trigger, so focus fell to <body> and
 *  the next Tab restarted at the top of the page: the keyboard lost its place.
 *
 *  Reading document.activeElement inside onOpenAutoFocus is too late as well,
 *  because by then Radix has already parked focus on its focus guard. The only
 *  moment the opener still has focus is before that, so a document-level
 *  focusin listener keeps the last element that took focus outside any dialog.
 *  focusin bubbles, which is why one listener covers every screen. */

let lastExternal: HTMLElement | null = null;
let listening = false;

function isInsideDialog(node: Element | null): boolean {
  if (!node) return false;
  // The guards are Radix's own placeholders for the focus trap; moving focus
  // onto one is not the operator moving, so it must not overwrite the target.
  return (
    node.closest('[role="dialog"],[role="alertdialog"],[data-radix-focus-guard]') !== null
  );
}

function startListening() {
  if (listening || typeof document === "undefined") return;
  listening = true;
  document.addEventListener(
    "focusin",
    (event) => {
      const target = event.target as HTMLElement | null;
      if (!target || target === document.body) return;
      if (isInsideDialog(target)) return;
      lastExternal = target;
    },
    true,
  );
}

/** Returns the onCloseAutoFocus handler for a dialog's content: it puts focus
 *  back where the operator left it, or does nothing when that node is gone. */
export function useReturnFocus() {
  React.useEffect(startListening, []);
  return React.useCallback((event: Event) => {
    const target = lastExternal;
    if (!target || !target.isConnected) return;
    event.preventDefault();
    target.focus();
  }, []);
}
