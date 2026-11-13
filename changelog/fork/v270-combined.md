## v270-combined (Nov 04, 2026)

Fork build of upstream `main` at `2e3459c` with `feat/ui-auth` and
`feat/activity-speed-histogram` merged, each rebased onto it. Upstream has
tagged no release for these commits yet, so this build carries four changes made
after v269 and the name v270 is the fork's own next number.

- **`fix/1218-prompt-cached-tokens` is archived and dropped from this build.**
  The branch contributed one thing: the `Prompt`, `Cached` and `Prefill` section
  of the Observability guide, and that section is removed. Upstream's Hipfire
  section in the same guide, and `docs/development/upstream-compat.md`, still
  state the same column rules, so nothing is left undocumented.
- **`/v1/realtime` now proxies, and a Realtime tab appears in the Playground.**
  Upstream added OpenAI Realtime API style websocket proxying to the model named
  by `?model=`, and a Playground tab that talks to speech to speech models such
  as LFM3 Audio through llama-server using the browser's microphone. Each session
  is written to the Activity view as one row when the socket closes. Behind
  `auth.ui: none` the tab reaches the endpoint through the web UI door at
  `/ui/v1/realtime`, so it works with a reverse proxy that owns web UI login and
  no API key in the browser.
- Hipfire usage metrics, so Hipfire models fill in the Activity view's Prefill
  and Decode columns.
- `cmd/kubeswap` can set annotations and service accounts on what it creates.
- The Makefile now decides whether to rebuild the UI from a stamp file instead
  of file dates.
- `feat/ui-auth` and the log bucket speed charts carry over unchanged from
  `v269-combined`.
- Fork build note: `release.yml` drops the `trigger-tap-update` job, this fork
  publishes no Homebrew tap, and `Publish Helm chart` skips a `vNNN-combined`
  tag instead of failing on it.
