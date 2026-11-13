## v269-combined (Nov 02, 2026)

Fork build of upstream v269 with `feat/ui-auth`, `fix/1218-prompt-cached-tokens`
and `feat/activity-speed-histogram` merged, each rebased onto upstream. It
carries everything upstream shipped in v269: usage parsing moved into
per-upstream handlers, activity captures persisted to the SQLite store, sidebar
category submenus, a PNG export for the Activity table, and a fix for memory
growth across config reloads.

- **Token counts now follow upstream, and that changes the `Prompt` column.**
  Upstream v269 defined `InputTokens` as the whole prompt, cached tokens
  included, for every backend, and `PromptPerSecond` as computed prompt tokens
  per second. The patch this fork carried in `v266-combined` and
  `v266.1-combined`, which subtracted the cached prefix from the stored prompt
  count so that backends agreed, is therefore dropped: upstream now makes them
  agree, in the other direction. Rows written by an earlier fork build keep the
  meaning they were written with, since old rows are not rewritten, so a
  `Prompt` value from before this build can sit beside a larger one from after
  it. The Activity speed charts plot the rate the backend reported, so they are
  unaffected.
- `fix/1218-prompt-cached-tokens` in this build is documentation only: a guide
  that states what the `Prompt`, `Cached` and `Prefill` columns mean, which
  field each backend sends for them, and what goes wrong.
- `auth.ui: none` still lets a reverse proxy own web UI login, and upstream's
  pause and resume endpoints are served at both doors, so the Models page
  buttons work through the web UI path and the public path alike.
- The Activity speed charts keep the log bucket view. The axis and its two
  breaks are one path painted without a fill, which removes the filled triangles
  that appeared where a break met the axis.
- Fork build note: `release.yml` drops the `trigger-tap-update` job, this fork
  publishes no Homebrew tap, and `Publish Helm chart` skips a `vNNN-combined`
  tag instead of failing on it.
