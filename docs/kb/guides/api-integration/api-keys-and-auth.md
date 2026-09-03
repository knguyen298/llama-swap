---
title: API keys, access control, and keeping secrets out of your config
summary: apiKeys guard inference like any hosted API; auth.ui lets a reverse proxy own web UI login (trustedHeader or none); use env macros for secrets.
category: guides
tags: [security, api-keys, auth, secrets, env, rate-limit, users, permissions, access-control, web-ui, reverse-proxy, authelia, trusted-header]
config_keys: [apiKeys, auth.ui, auth.trustedHeader, auth.trustedProxies, macros, peers.*.apiKey, models.*.env]
updated: 2026-09-03
---

# API keys and keeping secrets out of your config

## Requiring a key

By default llama-swap is unauthenticated. Add `apiKeys` and every request needs
one:

```yaml
apiKeys:
  - "sk-hunter2"
```

Clients may present it as `Authorization: Bearer <key>`, `x-api-key: <key>`, or
HTTP Basic. The web UI and everything under `/api/` are covered too, unless
you hand web UI login to a reverse proxy with `auth.ui` (see below).

Generate a real one:

```console
$ printf "sk-%s\n" "$(head -c 48 /dev/urandom | base64)"
```

Multiple keys are allowed, which is how you rotate without downtime: add the
new key, move clients over, remove the old one.

All keys are equivalent. llama-swap has no per-key rate limiting, user
accounts, roles, or per-key permissions. Put a reverse proxy or API gateway in
front of llama-swap when you need those controls.

**`apiKeys` is not a substitute for a firewall.** llama-swap starts processes
on your machine. Do not expose it to the internet on the strength of a bearer
token alone.

## Letting a reverse proxy own web UI login

With only `apiKeys` set, any client key also opens the web UI, and people get
an HTTP Basic prompt. If a proxy such as Authelia or oauth2-proxy already
logs people in, hand the web UI to it with `auth.ui`. The inference endpoints
are unaffected: `/v1/*`, `/models`, `/upstream/*` and `/comfyui/*` require an
API key in every mode, like any hosted inference API, and so do `/metrics`,
`/unload` and `/running`.

Two modes are available:

```yaml
apiKeys:
  - "${env.LLAMA_SWAP_KEY}"
auth:
  ui: trustedHeader
  trustedHeader: Remote-User
  trustedProxies: ["172.18.0.0/16"]
```

**`ui: trustedHeader`** — `/ui/`, `/api/*` (including `/api/mcp`), `/logs`
and `/logs/stream` accept only requests carrying the header the proxy sets
for logged-in users. An API key gets a 401 there, always. This is the same
trusted header pattern Grafana's auth proxy and Open WebUI use.

```yaml
apiKeys:
  - "${env.LLAMA_SWAP_KEY}"
auth:
  ui: none
```

**`ui: none`** — those same paths have no authentication in llama-swap at
all. Use it when the proxy gates them but cannot forward a user header, or
when you simply do not want llama-swap involved in UI login.

### How the Playground still works

The web UI never holds an API key. It reaches the model endpoints through a
mirror under `/api`: `/api/v1/chat/completions`, `/api/v1/models`,
`/api/upstream/<model>/`, `/api/comfyui/` and `/api/sdapi/...`. The mirror
follows the `auth.ui` mode, so a logged-in person can use the Playground and
open a model's own web UI, while the public `/v1/` paths stay key-only and
know nothing about the UI.

A person who can use the UI can therefore also call the mirror by hand. That
is the intended boundary: UI access means inference through the UI paths, an
API key means inference through `/v1/`. The two never cross.

### What the proxy has to do

- **Bypass login for the key-guarded paths**: `/v1/` at minimum, plus
  `/models`, `/upstream/`, `/comfyui/`, `/sdapi/` and the `/metrics`,
  `/unload`, `/running` endpoints if anything external calls them.
  llama-swap checks the key itself. Bypassing only `/v1/` is enough for
  OpenAI and Anthropic style clients.
- **Require login for everything else**: `/ui/`, `/api/`, `/logs` and `/`.
- **Forward the header** when using `trustedHeader`. For Traefik with
  Authelia that is `authResponseHeaders: [Remote-User]` on the forward-auth
  middleware; for Caddy, `copy_headers Remote-User` in `forward_auth`.
- **Publish llama-swap only to the proxy.** In `none` mode anything that can
  reach the port has the whole UI. In `trustedHeader` mode anything that can
  reach the port can forge the header unless `trustedProxies` names the
  proxy. `trustedProxies` is checked against the connecting address and
  ignores `X-Forwarded-For` on purpose. Leave it empty only when the network
  already guarantees this; llama-swap logs a warning at startup either way.

`auth.ui` does not change what an API key can do outside the UI. An
inference key can still call `/unload`, as it can today.

## Keep the keys out of the file

Use env macros so the config itself is safe to commit:

```yaml
apiKeys:
  - "${env.LLAMA_SWAP_KEY}"
  - "${env.LLAMA_SWAP_KEY_ROTATE}"
```

`${env.VAR}` is substituted before anything else. **If the variable is not set,
config loading fails with an error** — which is what you want. A typo becomes a
startup failure rather than an instance running with an empty key list.

The same applies anywhere a secret appears:

```yaml
peers:
  openrouter:
    proxy: https://openrouter.ai/api
    apiKey: ${env.OPENROUTER_API_KEY}
    models: [z-ai/glm-4.7, moonshotai/kimi-k2-0905]

models:
  hf-model:
    env:
      - "HF_TOKEN=${env.HF_TOKEN}"
    cmd: llama-server --port ${PORT} -hf some/repo
```

## How peer keys are used

`peers.*.apiKey` is injected into outgoing requests to that peer, as **both**
`Authorization: Bearer <key>` and `x-api-key: <key>`. Leave it blank and no key
is added. It accepts a macro, so use `${env.*}`.

This is the key llama-swap presents *to* the peer. It is unrelated to the
`apiKeys` clients present *to* llama-swap.

## What ends up in your config file

Worth being aware of, because config files get pasted into issues and chats:

- `apiKeys` — literal keys, unless you used env macros
- `peers.*.apiKey` — literal peer keys, same
- `models.*.env` — literal `NAME=value` pairs
- `models.*.cmd` / `cmdStop` — full command lines, which often carry
  `--api-key` flags and absolute paths
- `macros` — **after** env substitution has already run

That last one is the non-obvious one: a macro like
`llama: "llama-server --api-key ${env.KEY}"` holds the resolved secret once the
config is loaded.

Before sharing a config, strip those. Redact rather than delete so the shape of
the problem survives.

## Related

- `reference/config/apiKeys`, `reference/config/auth`, `reference/config/peers`
- `guides/configuration/macros` — how env macros resolve
