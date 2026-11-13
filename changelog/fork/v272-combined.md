## v272-combined (Nov 13, 2026)

Fork build of upstream `main` at `85cbb9b` with `feat/ui-auth` and
`feat/activity-speed-histogram` merged, each rebased onto it. That commit is
upstream v272 plus three later commits.

- **The web UI door is hardened.** A review of `auth.ui` found gaps, and this
  build closes them. The public paths are unchanged. A missing or wrong API key
  gets the same answer as before on every path outside `/ui/`.
  - A request with a percent-encoded prefix, such as `/%75i/`, gets a 404. A
    reverse proxy that matches rules on the raw path cannot be skipped this way.
  - A Tailcat client always needs an API key for `/ui/`, in both modes. Tailcat
    traffic does not pass the reverse proxy.
  - A wrong method under `/ui/` gets a 405. Before, it got the web UI page.
  - A bare `/ui/upstream` gets a 404. Before, it got a redirect to `/upstream/`,
    which left the web UI door.
  - The example config shows the default `ui: apiKeys`. Before, it showed
    `ui: none`.
  - The Vite dev server forwards `/ui`, so `npm run dev` reaches llama-swap
    again.
- **`/v1/realtime` can take the model from the first `session.update` event.**
  Behind `auth.ui: none` the Playground still reaches it at `/ui/v1/realtime`.
- `GET /props?autoload=false` answers 409 for a model that is not running, and
  does not load it.
- TensorFold usage metrics fill in the Activity view.
- The Playground chat accepts a pasted image.
- The web UI reloads when a lazy page chunk fails to load, for example after an
  upgrade.
- The Activity table renders less often, and decision models get their own
  category and badge.
- User `metadata` fields are kept next to `capabilities` in `/v1/models`.
- The recorders pass 1xx interim responses through, so a large upload with
  `Expect: 100-continue` works.
- The mobile sidebar closes when you select an item.
- The log bucket speed charts carry over unchanged from `v270-combined`.
- Fork build note: release notes for fork builds now live in `changelog/fork/`,
  one file per release. The earlier `vNNN-combined` entries moved there.
  `release.yml` reads that folder for a `vNNN-combined` tag.
