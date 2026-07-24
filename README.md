<div align="center">

# 🌿 EcoRouter

**A self-hosted, terminal-managed LLM router — reachable from anywhere.**

*Deploy one binary to a cloud host. Point your agents at one URL with a Bearer token.
Nothing to install on the client. Automatic fallback. Optional token savings.*
*Now with a **universal interactive mode** — type any command and be guided step-by-step.*

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue)](LICENSE)
[![Single Binary](https://img.shields.io/badge/deploy-single_binary-brightgreen)]()
[![Zero Client Install](https://img.shields.io/badge/client-zero_install-orange)]()
[![Interactive](https://img.shields.io/badge/CLI-interactive_wizards-9cf)]()
[![Loopback Only](https://img.shields.io/badge/daemon-loopback_only-red)]()

[Quick Start](#-quick-start) •
[Interactive Mode](#-interactive-mode-beginner-first) •
[Install](#-installation) •
[CLI Reference](#-cli-reference) •
[Deployment](#-production-deployment) •
[Security](#-security-model)

</div>

---

## 📖 Table of Contents

- [What is EcoRouter?](#-what-is-ecorouter)
- [Why EcoRouter?](#-why-ecorouter)
- [Architecture](#-architecture)
- [Features](#-features)
- [Quick Start](#-quick-start)
- [Interactive Mode (beginner-first)](#-interactive-mode-beginner-first)
- [Installation](#-installation)
- [CLI Reference](#-cli-reference)
- [Providers & Compatibility](#-providers--compatibility)
- [Routing Modes](#-routing-modes)
- [Token Management](#-token-management)
- [Model Pricing](#-model-pricing)
- [Token Saving via External Savers](#-token-saving-via-external-savers)
- [Production Deployment](#-production-deployment)
- [Security Model](#-security-model)
- [Observability](#-observability)
- [Configuration](#-configuration)
- [Client Setup](#-client-setup-what-your-users-see)
- [Build from Source](#-build-from-source)
- [Roadmap](#-roadmap)
- [License](#-license)

---

## 🎯 What is EcoRouter?

EcoRouter is a **self-hosted reverse proxy for LLM API traffic**. You deploy it once to a cloud host (any Linux VPS, OCI, or locally), and any agent — Claude Code, Codex CLI, Cursor, your own scripts — points its base URL at your EcoRouter endpoint and authenticates with a **Bearer token**.

That's it. **No VPN. No tunnel client. No cert to install. No agent on the user's machine.** Just a URL and a token, delivered over standard HTTPS.

EcoRouter's job is narrow and reliable:

> **Authenticate the caller → pick a model → forward the request → return the response**

…with fallback when a model fails, round-robin when you want to spread load, health-aware circuit breaking, full observability, and an *optional* hop through an external token-saving proxy.

**New in v0.3:** every command is now **interactive**. A complete beginner can type `eco provider add` (or just `eco`) and be walked through it with arrow-key menus — no flags to memorize, no "read the docs" dead-ends. Power users keep full flag-based scripting, unchanged.

---

## 💡 Why EcoRouter?

<table>
<tr>
<td width="33%" valign="top">

### 🚀 Zero client footprint
Users receive a URL and a token. They export two env vars. Done.
No binary to install, no cert to trust, no tunnel to run.

</td>
<td width="33%" valign="top">

### 🧭 Beginner-first CLI
Type any command bare and get guided prompts. Pickers replace memorizing IDs. Every wizard prints the equivalent flag command so you learn scripting by osmosis.

</td>
<td width="33%" valign="top">

### 🔒 Secure by default
TLS everywhere, Argon2id-hashed tokens, per-token rate limits, brute-force lockout, spend caps, IP allow-lists — all enforced server-side.

</td>
</tr>
<tr>
<td width="33%" valign="top">

### ⚡ Deterministic routing
Single / fallback / round-robin. Same input + same state ⇒ same decision. No black-box "smart" magic.

</td>
<td width="33%" valign="top">

### 🔌 No hardcoding
No built-in provider list, no baked-in URLs, no fixed prices. You configure every provider, base URL, model, and price. Works with *any* OpenAI/Anthropic-compatible API.

</td>
<td width="33%" valign="top">

### 💰 Optional token savings
Plug in any OpenAI/Anthropic-compatible saver proxy (headroom, caveman-proxy) with one flag. EcoRouter never mutates payloads itself.

</td>
</tr>
</table>

---

## 🏗️ Architecture

```
    Anywhere on the internet                    Your host (any Linux VPS / OCI)
  ┌──────────────────────────┐              ┌─────────────────────────────────────────┐
  │                          │              │                                          │
  │   Laptop / CI / phone    │              │   ┌─────────────────────────────────┐   │
  │                          │   HTTPS 443  │   │  Caddy  (TLS · Let's Encrypt)   │   │
  │   base_url =             │ ───────────▶ │   │  reverse_proxy → 127.0.0.1:8080  │   │
  │   https://eco.you.dev    │  Bearer tok  │   └───────────────┬─────────────────┘   │
  │                          │              │                   │                       │
  │   NOTHING installed      │              │                   ▼                       │
  │                          │              │   ┌─────────────────────────────────┐   │
  └──────────────────────────┘              │   │       eco daemon  :8080          │   │
                                            │   │       (loopback only)             │   │
                                            │   │                                    │   │
                                            │   │  • auth  • routes  • health       │   │
                                            │   │  • rate limits  • spend caps      │   │
                                            │   │  • activity log  • audit log      │   │
                                            │   └────────┬───────────────┬──────────┘   │
                                            │            │               │              │
                                            │            ▼               ▼              │
                                            │     [optional saver]   provider APIs      │
                                            │     127.0.0.1:8787       (any OpenAI /     │
                                            │     headroom /            Anthropic-       │
                                            │     caveman-proxy)        compatible)      │
                                            │                                            │
                                            └─────────────────────────────────────────────┘

                                            Firewall: only 443 + 22 open.
                                            8080 is loopback — never exposed.
```

**Three key design decisions baked into this shape:**

1. **The daemon never faces the internet.** It binds `127.0.0.1:8080`. Only Caddy on `443` is public.
2. **The control plane is SSH-only.** All configuration happens by SSH-ing to the host and running `eco`. No web UI, ever.
3. **Everything the client speaks is standard.** HTTPS + `Authorization: Bearer` — no custom protocol, no client SDK.

---

## ✨ Features

### Interactive CLI (v0.3)
- ✅ **Run `eco` with no args** → arrow-key main menu
- ✅ **Every command is interactive** — type it bare and get guided prompts
- ✅ **`--wizard` / `-w` on any command** → force full step-by-step guidance
- ✅ **Partial-flag gap filling** — pass what you know, get prompted for the rest
- ✅ **Pickers everywhere** — revoke a token, switch a route, remove a provider without knowing IDs
- ✅ **"Equivalent command" printed** after every guided action, so you learn the flag form
- ✅ **TTY-gated** — pipes / CI / systemd never hang; missing input → a clear error naming the flag

### Routing
- ✅ **Single** model routes
- ✅ **Fallback** chains (try model A, then B, then C on failure)
- ✅ **Round-robin** rotation (deterministic, skips unhealthy models)
- ✅ **Circuit breaker** — auto-skips models with high error rates
- ✅ **Streaming (SSE) aware** — no fallback after first byte (correctness > cost)

### Providers (no hardcoding)
- ✅ Add **any** OpenAI-compatible provider (OpenAI, DeepSeek, Groq, OpenRouter, Together, xAI, Mistral, LM Studio, vLLM, llama.cpp, …)
- ✅ Add **any** Anthropic-compatible provider
- ✅ Local models via **no-auth** providers (Ollama, LM Studio)
- ✅ Auth style chosen explicitly: `--auth bearer | anthropic-key | none`
- ✅ **No baked-in base URLs** — you paste the URL from the provider's docs
- ✅ Live model-catalog fetch + multi-select which models to enable

### Security
- ✅ Bearer tokens with `eco_live_` prefix (leak-scanner friendly)
- ✅ **Argon2id** hashed storage — plaintext shown once at creation
- ✅ Per-token **rate limits**, **concurrency caps**, **daily spend caps**
- ✅ Global rate limit + global daily spend cap
- ✅ **Brute-force lockout** with configurable window/ban
- ✅ Optional **IP allow/deny CIDR lists**
- ✅ TLS-only (delegated to Caddy), HSTS, `X-Content-Type-Options`, minimal CORS
- ✅ Request body size cap
- ✅ Loopback-only daemon (no `--expose` flag exists, by design)

### Token savings
- ✅ Plug-in **external saver proxies** (headroom, caveman-proxy, any base-URL forward)
- ✅ Per-route `--via <saver>` opt-in
- ✅ Global default saver with per-route `--no-via` override
- ✅ Auto-bypass on saver failure (unless `--via-required`)

### Observability
- ✅ SQLite activity store: token · IP · route · model · tokens · latency · cost · status
- ✅ Security audit log: auth failures, lockouts, revocations, rate/spend hits
- ✅ Rollups by day / model / route / token
- ✅ **Cost estimation** from a user-editable `pricing.toml` — unknown models render as *unpriced*
- ✅ `--json` on every read command for scripting

### Operational
- ✅ **`eco init` wizard** — menu-driven setup from fresh host to live endpoint
- ✅ **`eco doctor`** — diagnoses config, DNS, ports, providers, savers, health, **pricing**
- ✅ **Non-interactive mode** on every command for CI/scripting (`ECO_NONINTERACTIVE=1`)
- ✅ Shell completions (bash / zsh / fish)
- ✅ Single **static binary**, no CGO (pure-Go SQLite)
- ✅ **Cross-platform** — Linux, macOS, Windows (platform-specific process handling)
- ✅ `systemd` service + Caddyfile shipped in `deploy/`
- ✅ One-shot **provisioning script** in `scripts/install-oci.sh`

---

## 🚀 Quick Start

> **New!** Every command is interactive. Type any `eco <noun> <verb>` with no
> arguments on a terminal and EcoRouter guides you step-by-step — no more flag
> errors. Power users can still pass all flags for scripts.

### Interactive (beginner-friendly)

```bash
make build && export PATH="$PWD/bin:$PATH"
export ECO_HOME="$PWD/.data"

eco                     # main menu on a TTY
eco provider add        # guided setup for a new provider
eco token revoke        # picker shows your tokens — no need to remember ids
eco use                 # picker shows your routes
eco config set          # pick a key, get a typed value prompt
```

Add `--wizard` to any command to force full guidance even with some flags set.
Add `--yes` to destructive commands (`remove`, `revoke`, `clear`, `rotate`) to skip confirmation.

### 60-second local demo (scriptable)

```bash
# 1. Build
make build
export PATH="$PWD/bin:$PATH"
export ECO_HOME="$PWD/.data"          # local data dir (default: ~/.ecorouter)

# 2. Non-interactive init (base URL is REQUIRED — no hardcoded defaults)
eco init --yes \
  --provider-name    openai \
  --provider-auth    bearer \
  --provider-base-url https://api.openai.com/v1 \
  --provider-key     "$OPENAI_API_KEY" \
  --route-mode       single \
  --route-models     gpt-4o-mini \
  --token-label      laptop

#   → prints your token ONCE:  eco_live_9f2a…

# 3. Optional: set model prices (otherwise activity shows "unpriced")
eco pricing set openai/gpt-4o-mini --in 0.15 --out 0.60

# 4. Start the daemon
eco start -d
eco status
eco doctor

# 5. Use it from any OpenAI-compatible client
export OPENAI_BASE_URL="http://127.0.0.1:8080/v1"
export OPENAI_API_KEY="eco_live_9f2a…"

curl "$OPENAI_BASE_URL/chat/completions" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}'
```

> `--type` / `--provider-type` still work as **hidden aliases** for backward compatibility.

For production (HTTPS + public access), see [Production Deployment](#-production-deployment).

---

## 🧭 Interactive Mode (beginner-first)

Every command follows the **same rule**, so you learn it once:

> **Missing input + terminal → prompt. All flags present → run silently. `--wizard` → guide everything. No terminal (CI/pipe) → clear error, never a hang.**

### The behavior matrix

| You type | On a terminal | In CI (`ECO_NONINTERACTIVE=1` / pipe) |
|---|---|---|
| `eco provider add` | Wizard walks you through every field | Error: `missing --name` (no hang) |
| `eco provider add openrouter` | Asks for URL + key (name is known) | Error: `missing --base-url` |
| `eco provider add openrouter --base-url https://…` | Asks only for the key | Error: `missing --key` |
| `eco provider add openrouter --base-url … --key $K --auth bearer` | Silent, no prompt | Silent, no prompt |
| `eco provider add … --wizard` | Walks through ALL steps, prefilled | Error (cannot prompt, no hang) |
| `eco token revoke` | **Picker** shows tokens — no ids to remember | Error: `missing --id` |
| `eco token revoke tok_abc --yes` | Silent, no prompt | Silent, no prompt |
| `eco route add` (no flags) | Wizard: name → mode → models | Error: `missing --name` |
| `eco use` | **Picker** of routes | Error: `missing --route` |

### Example — bare command becomes a guided flow

```
$ eco provider add

  🔌  Add a provider

  How does this provider authenticate?
  ❯ 🔑  Bearer token in Authorization header
    🗝️   x-api-key header
    🚫  No authentication

  Give this provider a name
  › openrouter

  Base URL  (copy from the provider's docs, include /v1)
  › https://openrouter.ai/api/v1

  API key (hidden)
  › ••••••••••••••••

  ⏳ Testing… ✓ Found 147 models.

  Which models do you want available?  (Space to toggle, Enter = all)
  [x] gpt-4o
  [x] claude-3-5-sonnet
  [ ] llama-3.3-70b
  …

  ✓ Provider "openrouter" added — 2 models enabled.

    Equivalent command:
      eco provider add openrouter \
        --auth bearer \
        --base-url https://openrouter.ai/api/v1 \
        --key $KEY \
        --models "gpt-4o,claude-3-5-sonnet"
```

### Example — picker instead of memorizing an ID

```
$ eco token revoke

  Which token do you want to revoke?
  ❯ alice-laptop   (tok_a1b2, active)
    ci             (tok_c3d4, active)
    old-demo       (tok_e5f6, active)

  Revoke "alice-laptop" (tok_a1b2)?
  This immediately stops the token from working.
  ❯ Yes
    No

  ✓ Token tok_a1b2 revoked.
```

### Disabling interactivity

Set `ECO_NONINTERACTIVE=1` (or run in a non-TTY like a pipe or systemd) to force strict flag mode — missing input becomes a clear error instead of a prompt. This is what makes CI safe.

---

## 📦 Installation

### Option 1 — Build from source (recommended for now)

```bash
git clone https://github.com/ganjarsantoso/ecorouter && cd ecorouter
make build                    # → ./bin/eco
sudo make install             # → /usr/local/bin/eco
```

### Option 2 — Cross-platform release binaries

```bash
make release                  # produces:
                              #   bin/eco-linux-amd64
                              #   bin/eco-linux-arm64
                              #   bin/eco-windows-amd64.exe
```

### Option 3 — OCI / any Linux VPS one-shot

```bash
sudo DOMAIN=eco.you.dev BINARY_SRC=./bin/eco ./scripts/install-oci.sh
```

Creates the `ecorouter` system user, `/etc/ecorouter/`, `/var/lib/ecorouter/`, installs the `systemd` unit, and writes a Caddyfile if `caddy` is present.

**Requirements:** Go 1.22+ (for building), Linux/macOS/Windows for the binary. No CGO needed.

---

## 🎮 CLI Reference

Grammar: **`eco <noun> <verb> [args] [flags]`** — two levels deep, guessable, tab-completable.
**Every mutating command** takes its primary identifier as an *optional positional arg* and prompts for anything missing.

<details>
<summary><b>🚦 Lifecycle & health</b></summary>

| Command | Description |
|---|---|
| `eco` | Open the interactive main menu (on a TTY) |
| `eco init` | First-run wizard (or fully non-interactive via `--yes` + flags) |
| `eco doctor` | Diagnose config, DNS, ports, providers, savers, tokens, health, pricing |
| `eco status` | Daemon status, domain, active route, saver default |
| `eco version` | Version, commit, build date, Go version |
| `eco start [-d] [--port N] [--domain FQDN]` | Start the daemon (loopback only). `--wizard` prompts for port/domain |
| `eco stop` / `eco restart` | Lifecycle control |
| `eco logs [-f]` | View / follow server logs (prompts view-vs-follow on a TTY) |
| `eco completion <shell>` | Emit bash / zsh / fish completion script |

</details>

<details>
<summary><b>🔌 Providers</b></summary>

| Command | Description |
|---|---|
| `eco provider add [name]` | Add a provider. Prompts for auth style, name, base URL, API key. `--auth bearer\|anthropic-key\|none`, `--base-url`, `--key`, `--models` |
| `eco provider list` | List providers with health dot + auth style. `--json` |
| `eco provider test [name]` | Live connectivity + auth check (picker if no name). Refreshes catalog |
| `eco provider remove [name]` | Remove provider + purge secret (picker + confirm) |

> `--type openai\|anthropic\|ollama` is a **hidden deprecated alias** for `--auth bearer\|anthropic-key\|none`.

</details>

<details>
<summary><b>🧠 Models</b></summary>

| Command | Description |
|---|---|
| `eco models` | List every model across providers. `--json` |
| `eco models --provider <name>` | Filter to one provider |
| `eco models --refresh` | Re-fetch catalogs (prompts "all or pick one?" on a TTY) |

</details>

<details>
<summary><b>🛣️ Routes</b></summary>

| Command | Description |
|---|---|
| `eco route add [name] --single <model>` | Single-model route |
| `eco route add [name] --fallback m1,m2,...` | Fallback route (ordered) |
| `eco route add [name] --round m1,m2,...` | Round-robin route |
| `eco route add ... --via <saver>` | Attach a saver hop |
| `eco route add ... --no-via` | Bypass the global default saver for this route |
| `eco route add ... --via-required` | Fail if saver unreachable (default: bypass silently) |
| `eco route list` | List routes with mode, models, saver. `--json` |
| `eco route show [name]` | Full detail (picker if no name) |
| `eco route remove [name]` | Delete a route (picker + confirm) |
| `eco route test [name]` | Dry-run: which model would be selected now, and why (picker if no name) |
| `eco use [route]` | Set the active/default route (picker if no route) |

> Bare `eco route add` runs a full wizard: name → mode → multi-select models → optional saver.

</details>

<details>
<summary><b>🎫 Tokens (client access credentials)</b></summary>

| Command | Description |
|---|---|
| `eco token new [label]` | Generate a Bearer token. **Printed once**. Wizard prompts: label, route scope, rate, daily cap, concurrency, expiry, model scope |
| `eco token new ... --route <name>` | Scope to a single route |
| `eco token new ... --models a,b` | Scope to specific models |
| `eco token new ... --expires 90d` | Set expiry (`90d`, `24h`, `30m`, or blank/`never`) |
| `eco token new ... --rate 60/min` | Per-token rate limit |
| `eco token new ... --daily-cap 5` | Daily USD spend cap |
| `eco token new ... --concurrency 2` | Max concurrent in-flight requests |
| `eco token list` | List tokens (never the secret). `--json` |
| `eco token rotate [id]` | Issue a new secret; old one invalidated (picker + confirm) |
| `eco token revoke [id]` | Instantly revoke (picker + confirm) |
| `eco token scope [id] --route ... --models ...` | Adjust scope after creation (picker if no id) |

</details>

<details>
<summary><b>💰 Pricing (model cost estimates)</b></summary>

| Command | Description |
|---|---|
| `eco pricing set [key] --in N --out N` | Set USD per-1M-token prices (`key` = `provider/model`). Wizard: provider → model → prices |
| `eco pricing list` | List all configured prices. `--json` |
| `eco pricing remove [key]` | Remove a price entry (picker + confirm) |

Prices live in a user-editable `pricing.toml`. There is **no built-in price table** — unknown models render as *unpriced*.

</details>

<details>
<summary><b>🛡️ Access control (optional)</b></summary>

| Command | Description |
|---|---|
| `eco access allow [cidr]` | Restrict endpoint to given CIDR(s) (prompts if no cidr) |
| `eco access deny [cidr]` | Block CIDR(s) |
| `eco access list` | Show current rules |
| `eco access clear` | Return to open (anywhere) access (confirm) |

Empty allow list = open. Deny always applies.

</details>

<details>
<summary><b>💾 Savers (external token-saving proxies)</b></summary>

| Command | Description |
|---|---|
| `eco saver add [name] --url <base-url>` | Register an external saver proxy (wizard prompts name + URL) |
| `eco saver list` | List with reachability. `--json` |
| `eco saver test [name]` | Round-trip check (picker if no name) |
| `eco saver default [name]` | Route all traffic through this saver unless a route sets `--no-via` |
| `eco saver remove [name]` | Unregister (picker + confirm) |

</details>

<details>
<summary><b>📊 Observability</b></summary>

| Command | Description |
|---|---|
| `eco activity` | Recent requests: token, IP, route, model, tokens, latency, cost, status. `--wizard` prompts window + token filter |
| `eco activity --since 1h` | Time-filtered (`1h`, `24h`, `7d`) |
| `eco activity --token <id>` | Filter to one token |
| `eco stats` | Rollups. `--wizard` prompts group + window |
| `eco stats --by <route\|model\|token\|day>` | Group dimension |
| `eco stats --since 24h` | Time window |
| `eco audit` | Security view: auth failures, lockouts, rate/spend hits, revocations |

</details>

<details>
<summary><b>⚙️ Config</b></summary>

| Command | Description |
|---|---|
| `eco config show` | Print effective config (secrets redacted). `--json` |
| `eco config set [key] [value]` | Set `domain`, `port`, `global_rate`, `global_daily_cap`, `timeout_ms` (menu if no key) |

</details>

### Global flags (all commands)

| Flag | Effect |
|---|---|
| `-w, --wizard` | Force interactive guidance for the current command |
| `--json` | Machine-readable output (read commands) |
| `--config <path>` | Alternate config file |
| `--no-color` | Disable ANSI color |
| `-q, --quiet` | Suppress non-essential output |
| `-v, --verbose` | Extra diagnostic output |
| `-h, --help` | Contextual help |

> Destructive commands (`remove`, `revoke`, `clear`, `rotate`) also accept `-y, --yes` to skip confirmation.

---

## 🔌 Providers & Compatibility

EcoRouter has **no hardcoded provider list and no baked-in URLs**. The `--auth` flag selects the *authentication style*, and you supply the base URL — so any compatible API works.

| `--auth` value | Header sent | Works with |
|---|---|---|
| `bearer` | `Authorization: Bearer <key>` | OpenAI, DeepSeek, Groq, OpenRouter, Together, xAI, Mistral, Fireworks, Perplexity, LM Studio, vLLM, llama.cpp server, … |
| `anthropic-key` | `x-api-key: <key>` + `anthropic-version` | Anthropic and Anthropic-compatible gateways |
| `none` | *(no auth header)* | Local models: Ollama, LM Studio |

### Examples

```bash
# OpenAI
eco provider add openai --auth bearer \
  --base-url https://api.openai.com/v1 --key $OPENAI_API_KEY

# DeepSeek
eco provider add deepseek --auth bearer \
  --base-url https://api.deepseek.com/v1 --key $DEEPSEEK_API_KEY

# OpenRouter
eco provider add openrouter --auth bearer \
  --base-url https://openrouter.ai/api/v1 --key $OPENROUTER_API_KEY

# Groq
eco provider add groq --auth bearer \
  --base-url https://api.groq.com/openai/v1 --key $GROQ_API_KEY

# Anthropic
eco provider add anthropic --auth anthropic-key \
  --base-url https://api.anthropic.com/v1 --key $ANTHROPIC_API_KEY

# Local Ollama (no auth)
eco provider add ollama --auth none \
  --base-url http://127.0.0.1:11434/v1
```

Models are referenced as `provider/model` in routes (e.g. `openrouter/gpt-4o`), which lets the same model name coexist across providers.

---

## 🛣️ Routing Modes

<table>
<tr>
<th>Mode</th>
<th>Behavior</th>
<th>Example</th>
</tr>

<tr>
<td valign="top"><b>Single</b></td>
<td valign="top">Always use one model. Fully deterministic.</td>
<td>

```bash
eco route add cheap --single openai/gpt-4o-mini
```

</td>
</tr>

<tr>
<td valign="top"><b>Fallback</b></td>
<td valign="top">Try model 1. On <code>5xx</code>, <code>429</code>, timeout, or connection failure, try model 2, then 3. First success wins.<br><br><b>Important:</b> A <code>4xx</code> other than <code>429</code> is <b>not</b> retried — that's a client/config error; retrying would hide the real problem.</td>
<td>

```bash
eco route add solid \
  --fallback openai/gpt-4o,openai/gpt-4o-mini
```

</td>
</tr>

<tr>
<td valign="top"><b>Round</b></td>
<td valign="top">Round-robin rotation across models. Each request advances a counter. Circuit-broken models are skipped.</td>
<td>

```bash
eco route add balanced \
  --round openai/gpt-4o,anthropic/claude-3-5-sonnet
```

</td>
</tr>

<tr>
<td valign="top"><b>Via <sub>(modifier)</sub></b></td>
<td valign="top">Not a mode — a modifier on any mode. Routes through an external saver hop before the provider.</td>
<td>

```bash
eco route add cost-optimized \
  --fallback openai/gpt-4o,openai/gpt-4o-mini \
  --via headroom
```

</td>
</tr>
</table>

### Streaming caveat 📡

Once the first response byte has streamed to the client, **fallback is no longer possible** — the bytes are already gone. Pre-first-byte failures still trigger fallback normally.

### Circuit breaker 🔌

Each model tracks a rolling window (default: **last 20 requests**). If error rate exceeds **50%** over ≥5 requests, the model is circuit-broken for **60 s**. Fallback and round-robin skip broken models automatically. `eco doctor` and `eco status` surface broken models with the reason.

---

## 🎫 Token Management

### Create with scope + guardrails

```bash
# Personal laptop — full access, 90-day expiry
eco token new "my-laptop" --expires 90d --rate 60/min

# CI pipeline — scoped to one route, one model, capped spend
eco token new "ci" \
  --route      default \
  --models     openai/gpt-4o-mini \
  --rate       30/min \
  --concurrency 2 \
  --daily-cap  5 \
  --expires    90d

# Or just run it interactively:
eco token new         # prompts: label → route → rate → cap → concurrency → expiry
```

Output shows the plaintext **once**:

```
┌────────────────────────────────────────────────────────────┐
│  eco_live_9f2a7cKM3pQx4YnZ...                              │
│  ← copy now, shown once                                    │
└────────────────────────────────────────────────────────────┘

  Auth:  Authorization: Bearer <token>
  Daily spend cap: $5.00
  Max concurrent:  2
```

### Manage (all support pickers)

```bash
eco token list                 # never shows secrets
eco token rotate               # picker → confirm → new secret, same scope
eco token revoke               # picker → confirm → instant kill
eco token scope                # picker → route + model multi-select
```

### Global controls

```bash
eco config set global_rate       "1200/min"
eco config set global_daily_cap  50
```

---

## 💰 Model Pricing

Cost estimates come from a **user-editable** file at `$ECO_HOME/pricing.toml` (default `~/.ecorouter/pricing.toml`). There is **no built-in price table** — anything unset renders as *unpriced* (never a fake `$0`).

### Set prices

```bash
# Scriptable
eco pricing set openai/gpt-4o      --in 2.50 --out 10.00
eco pricing set openai/gpt-4o-mini --in 0.15 --out 0.60
eco pricing list
eco pricing remove openai/gpt-4o-mini

# Or interactively
eco pricing set        # provider picker → model multi-select → in/out prompts
```

### `pricing.toml` format

```toml
[prices."openai/gpt-4o"]
input_per_1m  = 2.50
output_per_1m = 10.00

[prices."openai/gpt-4o-mini"]
input_per_1m  = 0.15
output_per_1m = 0.60
```

Matching is: exact `provider/model` → bare `model` → prefix (so `gpt-4o` covers `gpt-4o-2024-11-20`). `eco doctor` warns if no pricing file exists.

---

## 💾 Token Saving via External Savers

EcoRouter **never compresses payloads itself**. Instead, it optionally forwards traffic through an external, OpenAI/Anthropic-compatible saver proxy chosen by the operator:

```
client ──HTTPS──▶ Caddy ──▶ EcoRouter ──▶ [saver hop] ──▶ provider
                                          (headroom, caveman-proxy, ...)
```

### Compatible tools

| Tool | Type | Plugs in? |
|---|---|---|
| **[headroom](https://headroomlabs.ai/)** | Drop-in local proxy (`headroom proxy --port 8787`) | ✅ `eco saver add headroom --url http://127.0.0.1:8787` |
| **caveman (proxy)** | Byte-safe local proxy | ✅ Same base-URL registration |
| **rtk** | Shell-output compressor (wraps commands) | ❌ Runs beside your agent, not through EcoRouter |
| **caveman / ponytail (skills)** | Client-side agent skills | ❌ Live *in* the agent, before EcoRouter |
| **LLMLingua** | Python library | ⚠️ Only if self-wrapped behind a base URL |

### Setup

```bash
headroom proxy --port 8787 &                          # 1. start the saver (loopback)
eco saver add headroom --url http://127.0.0.1:8787    # 2. register it
eco saver test headroom

eco route add cost-optimized \                        # 3a. attach to one route
  --fallback openai/gpt-4o,openai/gpt-4o-mini --via headroom

eco saver default headroom                            # 3b. — OR — global default
eco route add audit --single openai/gpt-4o --no-via   # 3c. bypass where needed
```

### Reliability

- If the saver is unreachable and `via_required = false` (default) → **bypass** and go direct. Saving must never reduce reliability.
- If `--via-required` → the request fails with a clear error.

---

## 🌐 Production Deployment

### Topology

| Layer | Component | Binding | Exposure |
|---|---|---|---|
| **Edge** | Caddy (TLS via Let's Encrypt) | `0.0.0.0:443` | Public (only port open besides SSH) |
| **Core** | `eco` daemon (systemd service) | `127.0.0.1:8080` | Loopback only — never public |
| **Optional** | saver (headroom / caveman-proxy) | `127.0.0.1:8787` | Loopback only |
| **Data** | SQLite activity/audit DB | file, `0600` | Local |
| **Secrets** | Provider keys + token hashes | `0600`, service-user owned | Local |

### Firewall

| Direction | Port | Source | Purpose |
|---|---|---|---|
| Ingress | `443/tcp` | `0.0.0.0/0` | Public HTTPS endpoint |
| Ingress | `22/tcp` | your operator IPs | SSH admin |
| Ingress | `8080/tcp` | **none** | Daemon stays loopback |
| Egress | `443/tcp` | `0.0.0.0/0` | Reach provider APIs |

### Deploy

```bash
git clone https://github.com/ganjarsantoso/ecorouter && cd ecorouter
make build

# Provision: creates ecorouter user, /etc/ecorouter, /var/lib/ecorouter,
# installs systemd unit, writes Caddyfile if caddy present.
sudo DOMAIN=eco.you.dev BINARY_SRC=./bin/eco ./scripts/install-oci.sh

# Configure as the service user (base URL is required — no hardcoded defaults)
sudo -u ecorouter env \
  ECO_HOME=/var/lib/ecorouter \
  ECO_CONFIG=/etc/ecorouter/config.toml \
  eco init --yes \
    --domain eco.you.dev \
    --provider-name openai \
    --provider-auth bearer \
    --provider-base-url https://api.openai.com/v1 \
    --provider-key "$OPENAI_API_KEY" \
    --route-mode fallback \
    --route-models openai/gpt-4o,openai/gpt-4o-mini

sudo systemctl start ecorouter
eco doctor
```

### systemd unit (shipped in `deploy/ecorouter.service`)

```ini
[Service]
User=ecorouter
Group=ecorouter
Environment=ECO_HOME=/var/lib/ecorouter
Environment=ECO_CONFIG=/etc/ecorouter/config.toml
ExecStart=/usr/local/bin/eco start --config /etc/ecorouter/config.toml
Restart=on-failure
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/ecorouter
```

### Caddyfile (shipped in `deploy/Caddyfile`)

```caddyfile
eco.you.dev {
    encode zstd gzip
    reverse_proxy 127.0.0.1:8080

    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
        X-Content-Type-Options "nosniff"
        -Server
    }

    request_body {
        max_size 10MB
    }
}
```

Caddy handles Let's Encrypt automatically — every standard HTTPS client trusts the certificate out of the box. **Nothing extra on the client.**

---

## 🔒 Security Model

EcoRouter assumes it's on the open internet. Security lives in **three layers** that require zero client cooperation.

### Layer 1 — Transport

- HTTPS only; plain HTTP is redirected
- TLS 1.2 minimum, 1.3 preferred (via Caddy)
- HSTS with long `max-age`
- Public-CA (Let's Encrypt) certs — trusted by every HTTPS client

### Layer 2 — Authentication

| Control | Implementation |
|---|---|
| Token format | `eco_live_` prefix + 32 random bytes, base62-encoded |
| Storage | **Argon2id** salted hash (`m=64 MiB, t=1, p=4`) — plaintext never persisted |
| Presentation | `Authorization: Bearer eco_live_…` — the header agents already send |
| Constant-time compare | `subtle.ConstantTimeCompare` — no timing leaks |
| Scoping | Per-token route + model whitelist |
| Expiry & rotation | Optional expiry (`90d`, `24h`, …); rotate with a single command |
| Revocation | Instant kill; revoked tokens fail immediately |
| No default token | Ships with **zero** valid tokens — operator must create the first |

### Layer 3 — Abuse resistance

- **Per-token rate limits** (`60/min` default) + **global rate** ceiling
- **Concurrency caps** per token
- **Brute-force lockout** (default: 5 failures in 1 min → 15 min IP ban)
- **IP allow/deny CIDRs** (optional; off by default)
- **Spend guardrails**: per-token + global daily USD cap → `429` when exceeded
- **Request body cap** (default 10 MB) to prevent memory-exhaustion payloads

### Host hardening

- Non-root system user (`ecorouter`)
- `NoNewPrivileges`, `ProtectSystem=strict`, `ProtectHome=true`, `PrivateTmp=true` in systemd
- Provider keys in `0600` file, service-user owned
- Loopback-only daemon — no way to expose it, by design (`--expose` does not exist)

### Threat model

| Threat | Mitigation |
|---|---|
| Token sniffed in transit | TLS + HSTS |
| Token brute-forced | High entropy + Argon2id + lockout + rate limit |
| Token leaked to a repo | `eco_live_` prefix (secret scanners) + easy rotate/revoke + expiry |
| Daemon attacked directly | Loopback binding — public can't reach it |
| Runaway cost / abuse | Per-token + global rate + spend caps |
| Host compromise via SSH | Key-only, IP-restricted SSH, non-root service user |
| Stolen token misuse scope | Token scoping to route/models + instant revocation |

---

## 📊 Observability

Every request is recorded in a local SQLite database. No telemetry, no phone-home.

### Live activity

```bash
$ eco activity --since 1h

TIME      TOKEN     IP              ROUTE    MODEL         TOK IN/OUT  LAT   STATUS  COST
14:22:11  laptop    203.0.113.9     default  gpt-4o-mini   183 / 47    412ms 200     $0.0001
14:23:02  ci        198.51.100.4    default  gpt-4o-mini   1024 / 512  692ms 200     $0.0005
14:23:44  laptop    203.0.113.9     default  gpt-4o        892 / 245   1.2s  200     $0.0047
14:24:19  laptop    203.0.113.9     default  local-llama   150 / 60    98ms  200     unpriced
```

### Rollups

```bash
$ eco stats --by model --since 24h

MODEL         REQS   TOK IN    TOK OUT   AVG MS  ERRS
gpt-4o-mini   1247   183,442   58,921    421     3
gpt-4o        342    98,124    32,187    1183    1
local-llama   89     12,443    4,891     102     0
```

### Security audit

```bash
$ eco audit --limit 10

TIME                       EVENT              IP              TOKEN     DETAIL
2026-07-24T02:18:44Z       auth_fail          203.0.113.9     -         invalid token
2026-07-24T02:18:50Z       lockout            203.0.113.9     -         IP banned 15m
2026-07-24T01:44:12Z       token_revoke       -               tok_abc   revoked
2026-07-24T01:12:03Z       rate_limit         198.51.100.4    tok_ci    token rate exceeded
2026-07-24T00:55:41Z       spend_cap          198.51.100.4    tok_ci    daily USD cap reached
```

### JSON everywhere

```bash
eco activity --json | jq '.[] | select(.status >= 400)'
eco stats --by token --json > report.json
eco audit --json | jq '.[] | select(.event == "auth_fail")'
```

Unknown-priced models render as `unpriced` — **never** a fake `$0`. All monetary figures trace to real recorded usage.

---

## ⚙️ Configuration

Location: `/etc/ecorouter/config.toml` (production) or `~/.ecorouter/config.toml` (local).
Secrets (provider keys, token hashes) live **separately** in `secrets.toml` (`0600`) or the SQLite DB — **never** in this file.

```toml
[server]
port       = 8080
host       = "127.0.0.1"     # loopback ALWAYS. Public access is via Caddy.
domain     = "eco.you.dev"
timeout_ms = 30000

[security]
require_tls        = true
max_body_bytes     = 10485760       # 10 MB
global_rate        = "600/min"      # ceiling across all tokens
auth_fail_lockout  = "5/1m -> 15m"  # 5 fails in 1 min => 15 min IP ban
global_daily_cap   = 0              # USD; 0 = disabled

[access]                             # empty allow = open ("anywhere")
allow = []
deny  = []

[providers.openai]
type     = "openai"                  # internal auth style (bearer)
base_url = "https://api.openai.com/v1"
# key stored in secrets, referenced by name — NEVER here

[routes.default]
mode         = "fallback"
models       = ["openai/gpt-4o", "openai/gpt-4o-mini"]
via          = ""
via_required = false

[savers.headroom]
url = "http://127.0.0.1:8787"

[defaults]
active_route  = "default"
saver_default = ""

[health]
window          = 20
error_threshold = 0.5
min_requests    = 5
cooldown_ms     = 60000
```

> The on-disk `type` field keeps internal values (`openai` / `anthropic` / `ollama`) for compatibility; the user-facing flag is `--auth bearer|anthropic-key|none`.

### File layout

```
/etc/ecorouter/config.toml       Public config (no secrets)
/var/lib/ecorouter/
├── secrets.toml                 0600, service-user owned (provider API keys)
├── ecorouter.db                 SQLite: tokens, activity, audit
├── pricing.toml                 User-editable model prices
├── eco.pid                      Daemon PID
├── eco.log                      Server log
└── Caddyfile.snippet            Generated by `eco init` for copy-paste
```

---

## 👤 Client Setup (what your users see)

The operator hands the consumer **exactly two things**:

```
Base URL:  https://eco.you.dev
Token:     eco_live_9f2a7c…
```

The consumer configures their existing agent — **nothing installed, no cert to trust**:

<table>
<tr>
<th>Any OpenAI-compatible client</th>
<th>Direct curl</th>
</tr>
<tr>
<td>

```bash
export OPENAI_BASE_URL="https://eco.you.dev/v1"
export OPENAI_API_KEY="eco_live_9f2a7c…"

# Now works with:
#   Claude Code
#   Codex CLI
#   Cursor
#   Aider
#   OpenAI SDKs
#   ... anything OpenAI-compatible
```

</td>
<td>

```bash
curl https://eco.you.dev/v1/chat/completions \
  -H "Authorization: Bearer eco_live_9f2a7c…" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-mini",
    "messages": [
      {"role":"user","content":"hi"}
    ]
  }'
```

</td>
</tr>
</table>

---

## 🔨 Build from Source

```bash
# Requires: Go 1.22+
git clone https://github.com/ganjarsantoso/ecorouter && cd ecorouter

make build                    # → bin/eco  (pure Go, CGO_ENABLED=0)
make test                     # run all tests
sudo make install             # → /usr/local/bin/eco
make release                  # linux-amd64, linux-arm64, windows-amd64
```

**Key dependencies (all pure Go, no CGO):**

| Concern | Library |
|---|---|
| CLI framework | `spf13/cobra` |
| Interactive prompts | `charmbracelet/huh` |
| Config / secrets / pricing | `BurntSushi/toml` |
| HTTP proxy | stdlib `net/http` |
| SQLite | `modernc.org/sqlite` (pure Go — keeps single-binary story) |
| Token hashing | `golang.org/x/crypto/argon2` |
| Rate limiting | `golang.org/x/time/rate` |
| Terminal color / input | `fatih/color`, `golang.org/x/term` |

Project layout:

```
cmd/eco/                 main entrypoint
internal/
├── cli/                 commands + interactive wizards + pickers
├── config/             TOML config
├── secrets/            0600 provider-key store
├── store/              SQLite: tokens, activity, audit
├── server/             proxy, auth, rate limit, lockout, access, concurrency
├── router/             single/fallback/round selection engine
├── health/             circuit breaker
├── cost/               pricing.toml loader + estimation
├── tui/                huh wrappers + TTY detection
├── output/             tables, colors, JSON
└── version/            build metadata
deploy/                  systemd unit, Caddyfile, example config
scripts/install-oci.sh   one-shot provisioning
prd/                     PRD + revision specs
```

---

## 📜 License

Apache-2.0.

---

<div align="center">

**Built for terminal-first operators who want a secure, honest, self-hosted LLM router —
now friendly enough for complete beginners.**

*One binary. One endpoint. Zero client install.*

</div>
