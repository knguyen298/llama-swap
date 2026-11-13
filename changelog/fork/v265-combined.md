## v265-combined (Oct 14, 2026)




Fork build of upstream v265 with two branches merged: `feat/ui-auth` and
`fix/1218-prompt-cached-tokens`.

- feat/ui-auth: a new `auth.ui` setting. The default, `apiKeys`, keeps the
  upstream behavior. With `none`, everything under `/ui/` has no
  authentication, so a reverse proxy must gate it. Every other path still
  requires an API key. The web UI sends every request through `/ui/` and holds
  no key.
- `auth.ui` and the base path support shipped in v265 share one URL layer:
  `apiUrl()` adds the `/ui` door after it re-attaches a stripped proxy prefix.
  Outside the browser the path is unchanged, so the docs-agent CLI keeps using
  the public door and its API key.
- `GET /logs` and `GET /upstream` stay on the public path. Their HTML redirect
  target is relative to that path, so serving them through the door produced
  `/ui/ui/`.
- fix/1218-prompt-cached-tokens: the Activity page's `Prompt` column now means
  the same thing for every backend. OpenAI-style `prompt_tokens` (vLLM) and
  Responses API `input_tokens` include the cached prefix that `Cached` already
  reports, so llama-swap subtracts it when it writes the row, to match
  llama.cpp's `timings.prompt_n`. `Prefill` divides that count once, because
  `buildMetrics` no longer subtracts the cached tokens itself. Anthropic-style
  `input_tokens` already excludes `cache_read_input_tokens`, so it is left
  alone. Activity rows written by earlier builds keep the old meaning.
- Fork build note: `release.yml` drops the `trigger-tap-update` job. This fork
  publishes no Homebrew tap, and the absent `TAP_REPO_PAT` secret made every
  fork release report a failed run.
