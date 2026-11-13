## v264-combined (Oct 13, 2026)




Fork build of upstream v264 with one feature branch merged.

- feat/ui-auth: a new `auth.ui` setting. The default, `apiKeys`, keeps the
  upstream behavior. With `none`, everything under `/ui/` has no
  authentication, so a reverse proxy must gate it. Every other path still
  requires an API key. The web UI sends every request through `/ui/` and holds
  no key. The audio.cpp passthrough added in v264 sits behind the same door, so
  `/ui/audiocpp/` keeps working with `auth.ui: none`.
- Upstream v264 adds `models.capabilities.voices`, kubeswap DRA ResourceClaims
  with NFS volumes, and a Wake-on-LAN confirmation UI.
- Fork build note: `release.yml` drops the `trigger-tap-update` job. This fork
  publishes no Homebrew tap, and the absent `TAP_REPO_PAT` secret made every
  fork release report a failed run.
