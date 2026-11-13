<div align="center">

# subrouterctl

**Run your whole SubRouter account from the terminal.**

One binary. Every documented account-management endpoint. No browser required.

[![Go](https://img.shields.io/badge/go-1.23%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-00b3a4)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos-6c7577)](https://github.com/)

</div>

---

## The problem it solves

SubRouter has a capable web console, but everything in it is a click. If you want
to rotate a key, find the cheapest route for a model, or pull yesterday's spend
into a spreadsheet, you are back to the browser and the mouse.

`subrouterctl` speaks to the same **account management API** the console uses,
using your **system access token**. The browser is needed exactly once, when
you authorize the CLI. After that, everything is scriptable.

```console
$ subrouterctl token list --kind fixed
┌────────┬──────────────────┬───────┬──────────┬──────────┬─────────┬──────────────────┬─────────┐
│     ID │ NAME             │ KIND  │ PROVIDER │ PROVIDERS│ EXPIRES │ KEY              │ STATUS  │
├────────┼──────────────────┼───────┼──────────┼──────────┼─────────┼──────────────────┼─────────┤
│ 392958 │ demo-fixed        │ fixed │ demo      │          │ -1      │ sk-Qk7******Rt2x │ enabled │
│ 392939 │ acme-fixed       │ fixed │ acme     │          │ -1      │ sk-Nm3******Hs6v │ enabled │
└────────┴──────────────────┴───────┴──────────┴──────────┴─────────┴──────────────────┴─────────┘
```

---

## Install

```bash
git clone https://github.com/Apothic-AI/subrouterctl
cd subrouterctl
make install          # -> $GOBIN/subrouterctl
```

Or build in place:

```bash
make build            # -> ./subrouterctl
```

---

## Quick start

```bash
# 1. authorize the CLI (one time, opens a URL you confirm in the browser)
subrouterctl auth login

# 2. see where you stand
subrouterctl account info

# 3. create a key that always routes through one provider
subrouterctl token create --type fixed --name "demo only" --provider demo
```

---

## The four kinds of key

SubRouter's console hides the most important idea behind four buttons.
`subrouterctl` names them explicitly, and `token create --type` picks one.

| `--type`   | Routing group   | What it does                                                        |
| ---------- | --------------- | ------------------------------------------------------------------- |
| `smart`    | `subrouter`     | Every subscribed source. Cheapest healthy route wins, auto failover. |
| `official` | `subrouter`     | Platform official channels only.                                     |
| `shared`   | `subrouter`     | Subscribed shared account pools only.                                |
| `fixed`    | `provider:<s>`  | One pinned provider. No cross-provider failover.                     |

```bash
# smart routing across two providers, price first
subrouterctl token create --type smart --name "router" \
  --providers demo,acme --route-preference price_first,stability_first

# pinned to a single provider
subrouterctl token create --type fixed --name "demo only" --provider demo

# shared account pools only, with a discount ceiling
subrouterctl token create --type shared --name "pools" --shared-max-discount 2500

# official channels only
subrouterctl token create --type official --name "official"
```

Pin a single model to a specific provider with `--model-route`:

```bash
subrouterctl token create --type smart --name "pinned" \
  --model-route "gpt-5.4=demo" --model-route "claude-sonnet-4-6=acme"
```

### Route preference criteria

`--route-preference` takes an ordered list. The first criterion that separates
two candidates wins.

```
stability_first   authenticity_first   provider_credit_first   first_token_first   price_first
```

`--sort-mode` picks how candidates of equal standing are ordered:
`token_price_first` (default) or `per_call_price_first`.

---

## Searching and sorting

Every directory, log feed and task list takes the same three flags.

| Flag              | Meaning                                                        |
| ----------------- | -------------------------------------------------------------- |
| `-q, --search`    | Free text search. Matched against names, slugs and categories.  |
| `--sort`          | Sort field, for example `price`, `rating`, `used`, `time`.      |
| `--order`         | `asc` (default) or `desc`.                                      |
| `--limit`         | Maximum rows to print. `0` prints everything.                    |
| `--page`          | Page number for the server-side paged endpoints.                 |

```bash
# cheapest routes for gpt models, most expensive first
subrouterctl model list --search gpt --sort price --order desc

# best rated providers
subrouterctl provider list --sort rating --order desc

# the keys that burned the most quota
subrouterctl token list --sort used --order desc

# logs for one model, most expensive call first
subrouterctl usage logs --model deepseek --sort cost --order desc
```

`--sort` is applied locally, so the CLI fetches every matching page first.
Search is pushed to the server where the endpoint supports it.

### Throughput

The usage log carries the time to first token and the request duration, and
`subrouterctl` shows the generation throughput the same way the SubRouter
console does in its **Time/first word** column: the completion tokens divided
by the total request duration.

```bash
$ subrouterctl usage logs --limit 3
┌────────────────────┬───────────┬────────────────────────┬──────────────┬───────┬────────┬─────────┬─────────┬──────┬─────────────────────────────┐
│ TIME              │ KEY       │ MODEL                  │ GROUP        │  COST │ TOKENS │ TIME(s) │ TTFT(s) │  TPS │ REQUEST                     │
├┼──────────────────┼───────────┼────────────────────────┼──────────────┼───────┼────────┼─────────┼─────────┼──────┼─────────────────────────────┤
│ 2026-11-13 08:36  │ demo-fixed │ deepseek-v4.1-flash-1017│ provider:demo │ $0.00 │ 275744 │    49.0 │    35.9 │ 47.6 │ 20261113133557142351560... │
└──────────────────┴───────────┴────────────────────────┴──────────────┴───────┴────────┴─────────┴─────────┴──────┴─────────────────────────────┘
```

`--sort tps` orders by that figure and `--sort ttft` by the time to first
token.

```bash
subrouterctl usage logs --sort tps --order desc
subrouterctl usage logs --sort ttft --order asc
```

One log in full, including every field the platform returned and the whole
performance payload it sends in the `other` field:

```bash
$ subrouterctl usage log 20261113133557142351560demo000
Log 20261113133557142351560demo000
────────────────────────────────────────────────────────────
Id                   1042
Created              2026-11-13 08:36:46
Key                  demo-fixed
Model                deepseek-v4.1-flash-1017
Group                provider:demo
IP                   203.0.113.7
Prompt tokens        273410
Completion tokens    2334
Duration             49.0s
Time to first token  35.9s
Throughput           47.6 t/s
Streaming            yes
Cost                 $0.00
Upstream request id  00000000-0000-4000-8000-000000000000

  performance payload
┌─────────────────────────┬─────────────────────────┐
│ FIELD                   │ VALUE                   │
├┼─────────────────────────┼─────────────────────────┤
│ frt                     │ 35949                   │
│ cache_tokens            │ 266368                  │
│ provider_slug           │ demo                     │
│ site_input_price        │ 0.016911764705882355    │
└─────────────────────────┴─────────────────────────┘
```

The JSON and YAML output carries every field the API returned, so nothing is
lost between the platform and a script.

### Single listings

Every directory entry can be opened on its own. A model is addressed by its
numeric id, a provider by its slug.

```bash
$ subrouterctl model show 27613
Model glm-5.3-prime
──────────────────────────────────────
Id               27613
Provider         Globe Labs
Provider slug    globe
Upstream model   glm-5.3-prime
Category         chat
Billing mode     ratio
Input price      0.4800 CNY
Output price     1.5086 CNY
Official input   1.4000
Cache read       0.0960
Context length   1000000
Average latency  1432 ms
Last call        2026-11-13 09:30:24
Endpoints        {"openai":{"path":"/v1/chat/completions","method":"POST"}, ...}
Description      GLM-5.3 high speed inference version
```

```bash
$ subrouterctl provider show demo
Provider Demo
──────────────────────────────
Id            101
Slug          demo
Status        active
Rating        5.00
Uptime        99.62%
Models        12 listed, 12 public
Credit score  500
Max RPM       100000
Subscribers   666
Description   Domestic Model-Openai

┌──────────────────────────────────────────────────────┐
│ MODEL                    │ PROVIDER │   IN/M │  OUT/M │
├┼─────────────────────────┼──────────┼────────┼────────┤
│ deepseek-v4.1-flash      │ Demo     │ 0.1150 │  0.4600 │
└┴─────────────────────────┴──────────┴────────┴────────┘
```

`provider models <slug>` prints only the catalogue, and it accepts the same
search and sort flags:

```bash
subrouterctl provider models demo --search deepseek --sort input --order desc
```

### Time filters

`--from` and `--to` accept a date, a relative age or a unix timestamp.

```bash
subrouterctl usage logs  --from 2026-11-01 --to 2026-11-07
subrouterctl usage stats --from 24h --to now
subrouterctl usage data  --from 7d  --to now
```

The platform refuses windows longer than 31 days.

---

## Command map

```
subrouterctl
├── auth            login · status
├── account         info · update · groups · models · subrouter-models
│                   topup-info · topups · redeem · balance
│                   notifications · aff · status
├── usage           logs · log · stats · data · export
├── token           list · show · create · update · delete · delete-batch · types
├── model           list · show · routing · pricing · options
├── provider        list · show · models · subs · subscribe · unsubscribe
│                   subscribe-all · unsubscribe-all · sub-config · shared
├── shared          list · subscribe · cancel
├── subscription    list · plans · preference · pay
├── task            list · show
├── merchant        self · models · model-create · model-update · model-batch
│                   price-preview · channels · channel-test · earnings · payouts · register
└── station         self · dashboard · models · global-models · packages
                    customers · key-groups · shared · official-channels
                    logs · earnings
```

`merchant` and `station` cover the provider (商家) and distributor (分站站长)
roles. A regular account receives `403 无权进行此操作` from them, which is the
expected answer.

---

## Endpoint coverage

Every endpoint in the
[Account Management API reference](https://subrouter.ai/docs?tab=account-api)
has a command.

| Documented endpoint                                    | Command                              |
| ------------------------------------------------------ | ------------------------------------ |
| `GET /api/status`                                       | `account status`                     |
| `GET /api/user/self`                                    | `account info`                       |
| `PUT /api/user/self`                                    | `account update`                     |
| `GET /api/user/topup/info`                              | `account topup-info`                 |
| `GET /api/user/topup/self`                              | `account topups`                     |
| `GET /api/log/self`                                     | `usage logs`                         |
| `GET /api/log/self` (by request id)                    | `usage log <request-id>`             |
| `GET /api/log/self/stat`                                | `usage stats`                        |
| `GET /api/data/self`                                    | `usage data`                         |
| `GET /api/subscription/plans`                           | `subscription plans`                 |
| `GET /api/subscription/self`                            | `subscription list`                  |
| `PUT /api/subscription/self/preference`                 | `subscription preference`            |
| `POST /api/subscription/{stripe,creem,epay}/pay`        | `subscription pay`                   |
| `GET /api/marketplace/providers`                        | `provider list`                      |
| `GET /api/marketplace/subscriptions`                    | `provider subs`                      |
| `POST /api/marketplace/subscribe`                       | `provider subscribe`                 |
| `POST /api/marketplace/subscribe-all`                   | `provider subscribe-all`             |
| `DELETE /api/marketplace/subscribe/{provider_id}`       | `provider unsubscribe`               |
| `DELETE /api/marketplace/subscriptions`                 | `provider unsubscribe-all`           |
| `GET /api/marketplace/subscription-config`              | `provider sub-config`                |
| `PUT /api/marketplace/subscription-config`              | `provider sub-config --auto-subscribe`|
| `GET /api/shared-subscriptions/plans`                   | `shared list`                        |
| `POST /api/shared-subscriptions/plans/{id}/subscribe`   | `shared subscribe`                   |
| `DELETE /api/shared-subscriptions/plans/{id}/subscribe` | `shared cancel`                      |
| `GET /api/marketplace/models`                           | `model list`                         |
| `GET /api/marketplace/models/{id}`                      | `model show`                         |
| `GET /api/marketplace/providers/{slug}`                 | `provider show`                      |
| `GET /api/user/self/groups`                             | `account groups`                     |
| `GET /api/user/models`                                  | `account models`                     |
| `GET /api/user/self/subrouter/models`                   | `account subrouter-models`           |
| `GET /api/token/`                                       | `token list`                         |
| `POST /api/token/`                                      | `token create`                       |
| `PUT /api/token/`                                       | `token update`                       |
| `GET /api/token/{id}`                                   | `token show`                         |
| `DELETE /api/token/{id}`                                | `token delete`                       |
| `POST /api/token/batch`                                 | `token delete-batch`                 |

Beyond the reference, the CLI also reaches the async task records
(`GET /api/task/self`), the referral and top-up endpoints, the shared-provider
routes, the marketplace pricing sheet, the log CSV export and the full
merchant and station surfaces.

---

## Output formats

```bash
subrouterctl account info                        # bordered table
subrouterctl -o json account info                # machine readable
subrouterctl -o yaml token list --limit 3        # machine readable
```

Every list command honours `--output`, and the JSON and YAML payloads use the
same field names as the platform's own API.

---

## Caching

By default nothing is cached. Every command reads live data, so a balance is
never stale and a key list never lies about a key.

The public directories are slow to fetch. The model directory holds around
2100 models spread over 43 pages, and a sorted listing needs all of them. When
you would rather have speed than freshness, opt in:

```bash
subrouterctl --cache-ttl 5m model list --sort price --order desc
```

| Flag            | Meaning                                                                  |
| --------------- | ------------------------------------------------------------------------ |
| `--cache-ttl`   | Serve public directory responses from disk for this long. `0` disables it. |
| `--cache-dir`   | Where the cache lives. Default `~/.cache/subrouterctl`.                   |
| `--cache-clear` | Remove every cached entry and exit.                                      |

Only the public marketplace responses are ever cached: the model directory, the
provider directory, the price sheet and the site configuration. Your account,
your keys, your balances, your logs and your tasks are always fetched live.

A sorted listing needs every page, so those pages are now requested in
parallel. The full 2100 model sort went from about 50 seconds to under 10,
and a warm cache brings it down to a second.

## Credentials

`subrouterctl auth login` runs the device authorization flow and writes
`~/.subrouter/credentials.json` with mode `0600`. It never prints the secrets
it stores.

```jsonc
{
  "key":          "sk-…",          // relay key: model calls
  "access_token": "…",             // management credential
  "user_id":      10001,           // numeric account id
  "base_url":     "https://subrouter.ai"
}
```

Every value can also come from the environment, which is what you want in CI.

```bash
export SUBROUTER_ACCESS_TOKEN=…
export SUBROUTER_USER_ID=10001
export SUBROUTER_BASE_URL=https://subrouter.ai
```

| Flag               | Environment variable    | Meaning                            |
| ------------------ | ----------------------- | ---------------------------------- |
| `--base-url`       | `SUBROUTER_BASE_URL`    | Site to talk to                    |
| `--token`          | `SUBROUTER_ACCESS_TOKEN`| System access token                |
| `--user-id`        | `SUBROUTER_USER_ID`     | Numeric account id                 |
| `--api-key`        | `SUBROUTER_API_KEY`     | `sk-` relay key                    |
| `--quota-per-unit` | `SUBROUTER_QUOTA_PER_UNIT` | Quota units per 1 USD (500000)  |
| `--show-secrets`   | –                       | Print secrets in full              |
| `--cache-ttl`      | –                       | Cache public directory responses   |
| `--cache-dir`      | `SUBROUTER_CACHE_DIR`   | Where the cache lives               |
| `--no-color`       | –                       | Plain output                       |
| `--timeout`        | –                       | Request timeout in seconds         |
| `-y, --yes`        | –                       | Skip confirmation prompts          |

### Two credentials, two jobs

The platform has two distinct token types and mixing them up is the single
most common mistake:

| Credential                    | Header                          | Use                             |
| ----------------------------- | ------------------------------- | ------------------------------- |
| `sk-…` relay key              | `Authorization: Bearer sk-…`    | Model calls, `/v1/*`            |
| System access token + user id | `Authorization: Bearer …` **and** `New-Api-User: <id>` | Account management, `/api/*` |

`subrouterctl` picks the right one for you and refuses to send the wrong pair.

---

## Safety

- Secrets are masked in every table by default. `--show-secrets` reveals them.
- Deleting a key, unsubscribing a provider and changing billing settings ask
  for confirmation. `-y` skips the prompt in scripts.
- Quota limits accept either USD (`--quota-usd 10`) or raw platform units
  (`--quota-units 5000000`).
- Payment, top-up and payout stay in the browser console. `subscription pay`
  only creates a checkout session and prints the URL.

---

## Development

```bash
make build     # build the binary
make test      # unit tests
make smoke     # live smoke test against your account
make lint      # gofmt + go vet
make help      # list every make target
```

The smoke test creates and removes a few throwaway API keys, so it needs a
real system access token. It restores the account to its original state.

```
passed 65, failed 0
```

---

<div align="center">

<sub>SubRouter is a trademark of its respective owner. This tool is an
independent client and is not affiliated with the platform.</sub>

</div>
