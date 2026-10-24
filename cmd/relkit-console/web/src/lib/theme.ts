import * as React from "react";

/** Theme state shared by the toggle and the boot script in index.html.
 *
 *  "system" is not a colour, it is the absence of a stored choice: the media
 *  query decides, and it keeps deciding when the OS flips at sunset. Storing a
 *  resolved colour instead would freeze a laptop in white after dark. */

export type ThemeChoice = "light" | "dark" | "system";

const STORAGE_KEY = "relkit-theme";

function applyTheme(choice: ThemeChoice) {
  const root = document.documentElement;
  const dark =
    choice === "dark" ||
    (choice === "system" &&
      window.matchMedia("(prefers-color-scheme: dark)").matches);
  root.classList.toggle("dark", dark);
  root.dataset.theme = choice;
}

export function readTheme(): ThemeChoice {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored === "light" || stored === "dark") return stored;
  } catch {
    // Private mode without storage: fall through to the system preference.
  }
  return "system";
}

export function useTheme() {
  const [choice, setChoice] = React.useState<ThemeChoice>(readTheme);

  React.useEffect(() => {
    applyTheme(choice);
    try {
      if (choice === "system") window.localStorage.removeItem(STORAGE_KEY);
      else window.localStorage.setItem(STORAGE_KEY, choice);
    } catch {
      // Not being able to remember the choice is not worth an error.
    }
  }, [choice]);

  React.useEffect(() => {
    if (choice !== "system") return;
    const query = window.matchMedia("(prefers-color-scheme: dark)");
    const listener = () => applyTheme("system");
    query.addEventListener("change", listener);
    return () => query.removeEventListener("change", listener);
  }, [choice]);

  return { choice, setChoice };
}

/** The first paint already has the right class (index.html does it inline), so
 *  this only has to keep it in sync after mount. */
export function ThemeBoot() {
  const { choice } = useTheme();
  React.useEffect(() => {
    applyTheme(choice);
  }, [choice]);
  return null;
}
