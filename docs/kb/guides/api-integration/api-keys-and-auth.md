---
title: API keys, access control, and keeping secrets out of your config
summary: apiKeys guard the public API; auth.ui can hand web UI login to a reverse proxy; use env macros for secrets.
category: guides
tags: [security, api-keys, auth, secrets, env, rate-limit, users, permissions, access-control, web-ui, reverse-proxy, authelia]
config_keys: [apiKeys, auth.ui, macros, peers.*.apiKey, models.*.env]
updated: 2026-09-06
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
HTTP Basic. The web UI and everything under `/api/` are covered too. You can
hand web UI login to a reverse proxy with `auth.ui` (see below).

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

With only `apiKeys` set, any client key also opens the web UI, and the browser
shows an HTTP Basic prompt. If a proxy such as Authelia or oauth2-proxy already
logs people in, hand the web UI to it with `auth.ui: none`:

```yaml
apiKeys:
  - "${env.LLAMA_SWAP_KEY}"
auth:
  ui: none
```

### Two doors, one set of handlers

llama-swap has one set of handlers and two doors in front of them. The public
door is every path outside `/ui/`. It always requires an API key. The UI door
is `/ui/`. It follows `auth.ui`: it requires an API key by default, or nothing
with `ui: none`.

The web UI makes every request through the UI door. The Playground calls
`/ui/v1/chat/completions`, the Models page calls `/ui/api/profiles`, and a
model's own web UI opens at `/ui/upstream/<model>/`. The server removes the
`/ui` prefix and runs the same handler an API client reaches at
`/v1/chat/completions`, `/api/profiles`, or `/upstream/<model>/`. The web UI
never holds an API key.

| Path | `auth.ui: apiKeys` (default) | `auth.ui: none` |
|---|---|---|
| `/ui/` and everything under it | API key | none, the proxy decides |
| every other path (`/v1/*`, `/api/*`, `/logs`, `/upstream/*`, `/metrics`, ...) | API key | API key |
| `/health` | none | none |

A person who reaches the web UI can also call `/ui/v1/...` by hand. That is
the intended boundary. Access to the web UI means inference through the UI
door. An API key means inference through the public door. The two never cross.

### What the proxy must do

- Require login for `/ui/`. With `ui: none` this is the only path llama-swap
  leaves open.
- Skip login for `/v1/`, so API clients reach llama-swap with only their key.
  Add `/models`, `/upstream/`, `/comfyui/`, `/sdapi/`, `/metrics`, `/unload`,
  or `/running` to that list if an external client uses them.
- Publish llama-swap only to the proxy. With `ui: none`, anything that can
  reach the port directly gets the whole web UI, including inference.
  llama-swap logs a warning at startup as a reminder.

`auth.ui` does not change what an API key can do outside the web UI. An
inference key can still call `/unload` and `/api/models/unload`, as it can
today.

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
