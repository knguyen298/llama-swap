/**
 * Every request from the web UI goes through the /ui door on the server.
 * The server applies the auth.ui setting at that door, removes the prefix,
 * and runs the same handler an API client reaches on the public path. This
 * lets the UI work with no API key when a reverse proxy does the login.
 */
export const UI_BASE = "/ui";
