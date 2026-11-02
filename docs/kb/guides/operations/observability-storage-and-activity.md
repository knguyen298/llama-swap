---
title: Observability, storage and Activity
summary: Use logs, metrics, captures and the Activity view to diagnose requests and retain useful history.
category: guides
tags: [operations, logs, metrics, activity, captures, stdout, http]
config_keys: [logLevel, logToStdout, metricsMaxInMemory, captureBuffer, store.path, store.persistCaptures]
updated: 2026-10-31
---

# Observability, storage and Activity

Use the Activity view for recent request timing and model events, logs for
process and proxy failures, and metrics for trends. `metricsMaxInMemory` and
`captureBuffer` bound retained in-memory data; increase them only when the
memory cost is acceptable.

```yaml
logLevel: debug
logToStdout: "proxy,http"
metricsMaxInMemory: 1000
captureBuffer: 100
```

## Prompt, Cached and Prefill columns

Each backend spells its token counts differently and means different things by
them. llama-swap normalizes them when it writes a row, so one meaning holds for
every backend. `docs/development/upstream-compat.md` lists the rules for each
upstream.

- `Prompt` is the whole prompt, cached tokens included.
- `Cached` is the part of `Prompt` the backend reused from its own prefix cache.
  Adding `Cached` to `Prompt` counts those tokens twice.
- `Prefill` is prompt tokens per second for the tokens the backend computed. The
  cached part is not in the numerator. llama.cpp, TabbyAPI and oMLX report this
  rate. For vLLM, llama-swap divides `Prompt` minus `Cached` by the prefill time,
  which is `time_to_first_token_ms` less `queue_time_ms`.

How each backend reports it:

- llama.cpp sends `timings.prompt_n`, which counts only computed tokens, and
  `timings.cache_n`. llama-swap stores the sum as `Prompt`.
- The OpenAI shape sends `usage.prompt_tokens` with the cached prefix already
  inside it, and `usage.prompt_tokens_details.cached_tokens` beside it.
- Anthropic's `input_tokens` leaves out both cache counts, so llama-swap adds
  `cache_read_input_tokens` and `cache_creation_input_tokens` back.

What goes wrong:

- Rows written before v269 mix the two meanings. A llama.cpp row stored
  `prompt_n` alone, so its `Prompt` left out `Cached`, while an OpenAI-style row
  stored `prompt_tokens` and included it. Old rows are not rewritten.
- vLLM reports a cached count only with `--enable-prompt-tokens-details`. Without
  it `Cached` shows `-`, llama-swap cannot split the prompt, and `Prefill` uses
  the whole prompt.
- A request that reports neither timings nor a rate adds no point to the speed
  charts on the Activity view.

## Log streams

llama-swap keeps three log streams. Each has its own tab on the UI's Logs page
and its own endpoint, `GET /logs/stream/<stream>`:

- `proxy`: llama-swap's own messages, such as model swaps, loading and errors.
- `upstream`: a copy of the upstream processes' stdout and stderr.
- `http`: one access log line per HTTP request (client, method, path, status,
  size, duration).

`logToStdout` chooses which streams are also written to stdout. It takes a
comma separated list such as `"proxy,http"` or `"upstream,http"`, or one of
`"both"` (every stream) and `"none"`. The default is `"proxy"`.

What goes wrong:

- HTTP access lines used to be part of the proxy log. With the default
  `"proxy"` they no longer reach stdout. Use `"proxy,http"` to keep them.
- A value outside the list, like `true` or `"proxy,debug"`, fails config
  loading.
- `logToStdout` is read at startup only. Restart llama-swap after changing it.

## Keeping activity and captures across restarts

By default activity rows and request/response captures are kept in memory and
lost on restart. `store.path` keeps activity rows in a SQLite file.
`store.persistCaptures: true` also keeps captures, in a second SQLite file
next to it named with `_captures` added to the base name:

```yaml
store:
  path: /data/metrics.db # captures go in /data/metrics_captures.db
  persistCaptures: true
captureBuffer: 2048 # MB of compressed captures to keep
```

Captures live in their own file so large blobs do not slow down the activity
log. Each activity row records the ID of its capture, such as
`26103120-a7Kq`: the UTC hour (yymmddhh) and four random characters. A row
whose capture was pruned shows no capture.

`captureBuffer` limits both in-memory and on-disk captures by their
compressed size. When a new capture pushes the total over the limit, the
oldest captures are deleted in the background. `captureBuffer: 0` turns
captures off. Stored captures stay in the file but are hidden until captures
are turned back on. SQLite reuses the space of deleted captures, so the file
grows to about `captureBuffer` and then stays that size.

What goes wrong:

- `persistCaptures` without `store.path` fails config loading.
- Moving or renaming only one of the two files breaks the pair. Move both, or
  change `store.path` and rename `metrics_captures.db` to match.
- A single capture larger than `captureBuffer` is skipped and logged as a
  warning.
- At very high request rates two requests in the same hour can draw the
  same capture ID. The second capture is skipped with a warning; the first
  is never overwritten.

Do not put secrets in captures or debug logs. Reduce retention after diagnosing
an issue.

## Speed charts on the Activity view

The Activity cards plot prompt tokens per second and generated tokens per second,
one value per request. The plotted prompt rate is the rate the backend applied to
the tokens it computed, the same number the `Prefill` column shows. It is not
`Prompt` divided by a time, because `Prompt` includes the cached part of the
prompt and a cached token costs no compute. Each chart uses a log axis, so every bucket covers the
same multiplicative range. The grid starts at 12 buckets per decade. It gets
finer, up to 192, when the plotted range is too narrow to fill 16 bars, and
coarser, down to 3, when the chart would otherwise draw more bars than requests,
which is four requests per bar. A model that runs at 60 tokens/sec within 10
percent therefore draws about 17 bars, not 2. Each chart states the factor one bar covers, so two time
ranges stay comparable. The Y axis is the share of the reported requests, not a
raw count.

llama-swap drops a rate when it cannot describe throughput:

- a prefill rate for a prompt shorter than 32 tokens, because the value then
  measures request overhead. The median reported rate is 5 tokens/sec for
  one-token prompts and 186 tokens/sec for 32 to 127 tokens.
- a generation rate for a completion shorter than 2 tokens, because an
  inter-token latency needs a gap between two tokens.
- any rate above 100,000 tokens/sec.

The line above each chart states how many requests reported a rate and how many
were dropped. Rates are also clipped to a display window of the 0.5th to the
99.5th percentile. Hover the kink at an end of the axis to see how many requests
sit outside that window. The p50, p90, p95 and p99 lines are always drawn, each
with its own dash pattern. Hover a bar to print its bucket range and its
request count in the two rows above the chart. Hover a percentile line to print
its value in one of those rows. A percentile line appears only when at least
five samples sit beyond it, so a small model fleet shows fewer lines than a large
one.

What goes wrong:

- An upstream that reports no timings adds no samples. A pooled chart therefore
  describes only the requests that reported rates, and it mixes every model.
  Filter by model before you compare speeds.
- A pooled chart shows the mix of your models, not the speed of any one of them.
  Two humps usually mean two models, not one unstable model.
