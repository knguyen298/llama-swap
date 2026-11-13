## v268-combined (Oct 29, 2026)


Fork build of upstream v268 with three branches merged: `feat/ui-auth`,
`fix/1218-prompt-cached-tokens` and `feat/activity-speed-histogram`. It moves
past v266-combined and v266.1-combined, and it picks up everything upstream
shipped in v267 and v268, including pause and resume inference, the reorganised
Models page, and `helpagent.disable`.

- pause and resume are served at both doors, so the Models page buttons work
  through the web UI path and through the public path alike
- the Activity speed charts, corrected after a look at real traffic. The axis
  break no longer brightens on hover, the dotted vertical lines at the two ends
  of the plotted window are gone, and the chart draws 176 units tall instead of
  224, close to the height of the charts it replaces
- the horizontal axis and both axis breaks are one path painted once, with no
  fill, which removes both the lighter dots at the joints and the filled
  triangles that appeared where a break met the axis
- `TokenHistogram.svelte` is deleted. `GET /api/metrics/stats` still returns the
  equal-width bins, so an older web UI keeps working, and this web UI shows its
  empty state against an older binary rather than carrying a second chart
  renderer
- Fork build note: `release.yml` drops the `trigger-tap-update` job, this fork
  publishes no Homebrew tap, and `Publish Helm chart` now skips a
  `vNNN-combined` tag instead of failing on it, because that job takes the chart
  version from a plain `vNNN` tag
