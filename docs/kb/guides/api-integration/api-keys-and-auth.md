---
title: API keys, access control, and keeping secrets out of your config
summary: apiKeys authentication has no per-key rate limits or user permissions; auth.trustedHeader lets a reverse proxy own web UI login; use env macros for secrets.
category: guides
tags: [security, api-keys, auth, secrets, env, rate-limit, users, permissions, access-control, web-ui, reverse-proxy, authelia, trusted-header]
config_keys: [apiKeys, auth.trustedHeader, auth.trustedProxies, macros, peers.*.apiKey, models.*.env]
updated: 2026-09-02
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
a reverse proxy vouches for UI users through `auth.trustedHeader` (see
below).

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
logs people in, let it vouch for them instead:

```yaml
apiKeys:
  - "${env.LLAMA_SWAP_KEY}"
auth:
  trustedHeader: Remote-User
  trustedProxies: ["172.18.0.0/16"]
```

With `auth.trustedHeader` set:

- `/ui/`, `/api/*` (including `/api/mcp`), `/logs` and `/logs/stream` accept
  only requests carrying that header. An API key gets a 401 there, always.
  These are the paths only the UI uses.
- Everything else accepts an API key **or** the header: `/v1/*` including
  `/v1/models`, `/models`, `/upstream/*`, `/comfyui/*`, `/metrics`, `/unload`
  and `/running`. That is how the Playground and the upstream links work from
  the browser without a key, and how API clients and Prometheus keep using
  keys.

This is the same trusted header pattern Grafana's auth proxy and Open WebUI
use. The proxy is the only source of truth about who a person is, so two
things must hold:

- **Only the proxy can reach llama-swap.** Bind to localhost or a Docker
  network the proxy shares. Anyone who can connect directly can add the
  header themselves.
- **`trustedProxies` names the proxy.** It lists the IPs or CIDR ranges
  allowed to present the header, checked against the connecting address.
  `X-Forwarded-For` is ignored on purpose. Leave it empty only when the
  network already guarantees the first point; llama-swap logs a warning at
  startup in that case.

On the proxy side, forward the header on authenticated requests and bypass
login for `/v1/` so API clients can present their key. For Caddy with
Authelia:

```
forward_auth authelia:9091 {
    uri /api/authz/forward-auth
    copy_headers Remote-User Remote-Groups Remote-Email Remote-Name
}
reverse_proxy llama-swap:8080
```

with an Authelia access control rule of `policy: bypass` for
`^/v1/` and `policy: one_factor` (or `two_factor`) for `^/(ui|api|logs)`.
Authelia does not set `Remote-User` on bypassed requests, so a client on
`/v1/` without a session still has to present an API key.

`auth.trustedHeader` does not change what an API key can do outside the UI.
An inference key can still call `/unload`, as it can today.

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
