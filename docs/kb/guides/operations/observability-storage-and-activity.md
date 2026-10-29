---
title: Observability, storage and Activity
summary: Use logs, metrics, captures and the Activity view to diagnose requests and retain useful history.
category: guides
tags: [operations, logs, metrics, activity, captures, stdout, http]
config_keys: [logLevel, logToStdout, metricsMaxInMemory, captureBuffer]
updated: 2026-10-23
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

## Prompt and Cached columns

Every backend reports token counts its own way. llama-swap normalizes them on
write, so one meaning holds for all backends:

- `Cached` counts prompt tokens the backend reused from its own prefix cache.
- `Prompt` counts only the new tokens the backend had to process. It never
  includes `Cached`.
- `Prefill` is the prompt rate. llama.cpp, TabbyAPI and oMLX report it. For
  vLLM, llama-swap divides `Prompt` by the time to the first token.
- For vLLM and other OpenAI-style usage, `Prompt` and `Cached` add up to the
  full prompt length. Anthropic usage also keeps `cache_creation_input_tokens`
  out of `input_tokens`, so the sum can be smaller than the full prompt.

Backends report these fields differently. llama.cpp sends `timings.prompt_n`
(new tokens) and `timings.cache_n`. vLLM sends `usage.prompt_tokens` with the
cached prefix included and `usage.prompt_tokens_details.cached_tokens`. In
Anthropic-style usage, `input_tokens` already excludes `cache_read_input_tokens`.
llama-swap subtracts the cached count from the OpenAI-style totals so that all
three spellings agree.

What goes wrong:

- Rows written before this change stored the vLLM `prompt_tokens` value as-is,
  so `Prompt` included `Cached` for those requests. Old rows are not rewritten.
- vLLM reports a cached count only with `--enable-prompt-tokens-details`. Without
  it, `Cached` shows `-` and `Prompt` keeps the whole prompt for vLLM.

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

Do not put secrets in captures or debug logs. Reduce retention after diagnosing
an issue.

## Speed charts on the Activity view

The Activity cards plot prompt tokens per second and generated tokens per second,
one value per request. Each chart uses a log axis, so every bucket covers the
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
