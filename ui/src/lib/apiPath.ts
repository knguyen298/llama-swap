/**
 * Resolves server-rooted paths ("/api/version") against the path prefix the
 * UI is mounted under.
 *
 * llama-swap is commonly served behind a reverse proxy that strips a prefix,
 * e.g. https://host/asubpath/llama-swap/ rewrites to the origin's "/". The
 * server still sees /api/... and /v1/..., but the browser must request
 * /asubpath/llama-swap/api/...; root-anchored URLs would resolve against the
 * origin root and escape the proxy's location block. Every same-origin fetch
 * and EventSource URL funnels through apiUrl() so the prefix is re-attached.
 *
 * The UI uses hash routing (svelte-spa-router), so the document path is the
 * static mount point ending in "/ui" (or "/ui/") and the prefix is whatever
 * precedes it. When there is no prefix, or no browser at all (the docs-agent
 * CLI under Node, unit tests), the path is returned unchanged: that keeps
 * cli/fetchBase.ts, which converts rooted paths against --base-url, working.
 */

/** Extract the mount prefix preceding the UI's own path segment. Exported
 * for unit testing; callers should use apiUrl(). */
export function uiPrefix(pathname: string): string {
  const cut = pathname.lastIndexOf("/ui");
  if (cut < 0) return "";
  const rest = pathname.slice(cut);
  if (rest === "/ui" || rest === "/ui/") return pathname.slice(0, cut);
  return "";
}

/** The door the web UI talks through. The server applies the auth.ui setting
 * at this prefix, removes it, and runs the same handler an API client reaches
 * on the public path. See uiDoor in internal/server/auth.go. */
export const UI_DOOR = "/ui";

/** Prefix a rooted server path with the current mount prefix and the UI door.
 *
 * Every request the web UI makes goes through the door, so the browser never
 * needs an API key when auth.ui is none. Outside the browser the path is
 * returned unchanged: cli/fetchBase.ts resolves rooted paths against
 * --base-url and authenticates through the public door with an API key. */
export function apiUrl(path: string): string {
  if (typeof window === "undefined") return path;
  const prefix = uiPrefix(window.location.pathname);
  return prefix + UI_DOOR + path;
}
