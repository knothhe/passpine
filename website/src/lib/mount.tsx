import { StrictMode, type ReactNode } from "react";
import { flushSync } from "react-dom";
import { createRoot } from "react-dom/client";
import "../styles.css";

// The prerendered markup is in the build's default language; a fresh client
// render replaces it with the visitor's language instead of hydrating.
export function mount(app: ReactNode) {
  const root = createRoot(document.getElementById("root")!);
  // Swap the markup in one task so the browser's scroll position survives.
  flushSync(() => root.render(<StrictMode>{app}</StrictMode>));
  // The language may change the layout, so re-apply a fragment like /#download.
  const id = decodeURIComponent(location.hash.slice(1));
  if (id) document.getElementById(id)?.scrollIntoView({ behavior: "instant" });
}
