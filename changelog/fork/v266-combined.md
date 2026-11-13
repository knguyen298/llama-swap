## v266-combined (Oct 22, 2026)



Fork build of upstream v266 with three branches merged: `feat/ui-auth`,
`fix/1218-prompt-cached-tokens` and `feat/activity-speed-histogram`. The first
two are described under v265-combined and are unchanged here.

- feat/activity-speed-histogram: the Activity view plots prompt speed and
  generation speed as a distribution on a log axis. The equal-width bins it
  replaces put 24,431 of 24,434 generation samples in one bar, because three
  one-token completions reported 1,000,000 tokens per second and stretched the
  axis over six decades.
- Each chart buckets rates on a geometric grid whose resolution adapts to the
  plotted window, from 3 to 192 buckets per decade on a nested ladder, aiming at
  16 bars and at least four requests per bar. One bar therefore covers the same
  multiplicative range everywhere on the axis, and each chart states that range.
- The plot runs from the 0.5th to the 99.5th percentile and draws a kink on the
  axis where it clips a tail. Hover the kink for the count outside the window.
- Rates that cannot describe throughput are dropped as the chart reads them:
  prefill under 32 prompt tokens, decode under 2 generated tokens, and any rate
  above 100,000 per second. Each chart states how many requests reported a rate,
  how many were dropped and how many reported no timing at all, so a pooled
  chart never implies it covers every request.
- A percentile line appears only when at least five samples sit beyond it, so a
  model fleet of 24 requests shows p50 and refuses p99.
- No stored row changes meaning and the database needs no migration. The
  `prompt_histogram` and `gen_histogram` fields of `GET /api/metrics/stats` keep
  the old bins, and the speed distributions arrive in new fields, so an old web
  UI works against this binary and a new web UI falls back against an old one.
- Fork build note: `release.yml` drops the `trigger-tap-update` job. This fork
  publishes no Homebrew tap, and the absent `TAP_REPO_PAT` secret made every
  fork release report a failed run.
