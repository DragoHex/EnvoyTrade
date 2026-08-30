# EnvoyTrade V1 — Implementation Plan

Copy-trade platform on Zerodha Kite Connect. One master, N followers, 1-master-per-follower. Single Go binary, single Postgres.

Verified against vendored SDK at `./gokiteconnect` (`github.com/zerodha/gokiteconnect/v4`).

**SDK facts that shape the design.** `gokiteconnect` is a third-party dependency consumed as-is — **not forked or patched**. Every seam below works around its shape from the service side, in `internal/kite`.
- `OrderParams` already carries `AlgoID` and `Tag` (`orders.go:88`) — SEBI algo-tagging needs no SDK change.
- `Client.SetHTTPClient(*http.Client)` (`connect.go:175`) — REST egress is fully injectable per account via a custom `http.Transport`. This is how the proxy vendor (algoip.in / staticip.in style) integrates. See §3.3.
- `ticker.New(...).Serve()` dials with `d := websocket.DefaultDialer; d.HandshakeTimeout = ...` (`ticker/ticker.go:296`) — that's gorilla's **package-level global `*Dialer`**, not a copy, and `Serve()` mutates it in place. Two consequences, both unfixable without editing the vendored file, which is off the table: (1) no way to give a `Ticker` instance its own proxy — `Dialer.Proxy` is fixed to `http.ProxyFromEnvironment` for every instance in the process; (2) every concurrent `Ticker` in the binary races on the same object's `HandshakeTimeout`. Design consequence: run at most one `Ticker` per process (the master's — see §3.4), and never rely on per-account WS proxying. Follower status comes from the IP-bound REST client instead.
- `RenewAccessToken(refreshToken, apiSecret)` exists (`user.go:192`) but requires the paid "refresh token" add-on; without it daily manual login is mandatory. See §3.2.

---

## 1. Package / module layout

```
cmd/envoytrade/main.go          wiring, config load, graceful shutdown
cmd/envoytrade-auth/main.go     daily re-auth CLI/web callback handler

internal/config/                typed config, env + file, no secrets inline
internal/domain/                pure types + rules, zero deps
  account.go  order.go  signal.go  sizing.go  killswitch.go
internal/store/                 Postgres, sqlc-generated + hand queries
  postgres/  migrations/  txn.go
internal/kite/                  wraps gokiteconnect
  client.go        per-account REST client factory (egress-bound)
  ticker.go        per-account WS listener + reconnect
  session.go       token lifecycle
  fake/            in-memory fake for tests
internal/queue/                 SEAM: dispatch abstraction
  queue.go         interface
  memchan/         V1 impl (buffered chan + outbox drain)
internal/listener/              master + follower order-update ingestion
internal/engine/                fan-out, sizing, dispatch decisions
internal/worker/                per-follower goroutine + rate limiter
internal/recon/                 reconciliation + drift detection loops
internal/obs/                   SEAM: logging, metrics, tracing facades
  log.go  metrics.go  trace.go
internal/admin/                 HTTP dashboard, read-mostly + kill-switch
```

### Seam interfaces (the only ones that must be stable)

```go
// internal/queue — swap memchan for Redis Streams / Kafka in V2.
type Publisher interface {
    Publish(ctx context.Context, ev domain.MasterFill) error
}
type Consumer interface {
    // Ack/Nack is explicit so at-least-once semantics survive the swap.
    Consume(ctx context.Context, fn func(context.Context, domain.MasterFill) error) error
}

// internal/kite — everything the engine needs; fake/ implements it.
type Broker interface {
    PlaceOrder(ctx context.Context, variety string, p kiteconnect.OrderParams) (string, error)
    OrderHistory(ctx context.Context, orderID string) ([]kiteconnect.Order, error)
    Margins(ctx context.Context) (kiteconnect.AllMargins, error)
}
type BrokerFactory interface { For(accountID string) (Broker, error) }

// internal/obs — facades, so OTel drops in without touching call sites.
type Metrics interface {
    Counter(name string, labels ...string) Counter
    Histogram(name string, labels ...string) Histogram
}
type Tracer interface { Start(ctx context.Context, name string) (context.Context, Span) }
```

**Rule:** `engine` and `worker` import `queue`, `kite`, `store`, `obs` interfaces only — never `memchan`, never `gokiteconnect` directly. `main.go` is the only place concrete impls are named. That single rule is what makes the V2 split mechanical.

**Recommendation:** interfaces defined in the *consumer* package (`engine` declares what it needs) rather than the provider. Alternative considered — one central `internal/ports` package — rejected: it becomes a god-package and couples unrelated modules.

---

## 2. Data model

Design choice up front: **append-only event log + derived state tables.** Alternative considered — mutate a single `orders` row in place, keep no history. Rejected: SEBI audit trail wants immutable per-transition records, and reconciliation needs the transition sequence to detect drift. In-place mutation makes both impossible to reconstruct after the fact.

```sql
-- accounts: master and followers in one table, discriminated by role.
CREATE TYPE account_role AS ENUM ('master', 'follower');
CREATE TABLE accounts (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  role            account_role NOT NULL,
  kite_user_id    text NOT NULL UNIQUE,      -- Zerodha client id, e.g. AB1234
  api_key         text NOT NULL,
  api_secret_ref  text NOT NULL,             -- pointer into secret store, NEVER the secret
  egress_ip       inet,                      -- whitelisted static IP for this account
  egress_proxy_url text,                     -- if proxying rather than IP-binding
  algo_id         text,                      -- broker-assigned, SEBI mandate
  status          text NOT NULL DEFAULT 'active',  -- active|paused|disabled
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);

-- Tokens rotate daily; separate table so accounts rows stay stable and
-- token history is auditable.
CREATE TABLE account_sessions (
  id             bigserial PRIMARY KEY,
  account_id     uuid NOT NULL REFERENCES accounts(id),
  access_token_ref text NOT NULL,
  refresh_token_ref text,
  obtained_at    timestamptz NOT NULL DEFAULT now(),
  expires_at     timestamptz NOT NULL,       -- Kite: ~06:00 IST next day
  revoked_at     timestamptz
);
CREATE UNIQUE INDEX one_live_session_per_account
  ON account_sessions (account_id) WHERE revoked_at IS NULL;

-- 1 master per follower, enforced structurally by PK on follower_id.
CREATE TABLE follow_links (
  follower_id     uuid PRIMARY KEY REFERENCES accounts(id),
  master_id       uuid NOT NULL REFERENCES accounts(id),
  capital_ratio   numeric(10,6) NOT NULL CHECK (capital_ratio > 0),
  max_qty_per_order integer,                 -- hard cap, defence in depth
  enabled         boolean NOT NULL DEFAULT true,
  effective_from  timestamptz NOT NULL DEFAULT now(),
  CHECK (follower_id <> master_id)
);
```
`follower_id` as the primary key *is* the 1-master constraint — no trigger, no application check. Alternative considered — surrogate `id` PK plus a unique index on `follower_id`; equivalent, but the surrogate buys nothing and lets a careless migration drop the index.

```sql
-- Master fill = the signal. Unique on kite order id + status so a WS
-- redelivery is idempotent at the DB layer.
CREATE TABLE master_fills (
  id              bigserial PRIMARY KEY,
  master_id       uuid NOT NULL REFERENCES accounts(id),
  kite_order_id   text NOT NULL,
  exchange        text NOT NULL,
  tradingsymbol   text NOT NULL,
  instrument_token bigint NOT NULL,
  transaction_type text NOT NULL,            -- BUY|SELL
  product         text NOT NULL,             -- CNC|MIS|NRML
  order_type      text NOT NULL,
  filled_quantity integer NOT NULL,
  average_price   numeric(18,4) NOT NULL,
  status          text NOT NULL,
  order_timestamp timestamptz NOT NULL,
  raw_payload     jsonb NOT NULL,            -- verbatim WS frame, audit
  received_at     timestamptz NOT NULL DEFAULT now(),
  dispatch_state  text NOT NULL DEFAULT 'pending', -- pending|dispatched|dead
  dispatched_at   timestamptz,
  UNIQUE (master_id, kite_order_id, filled_quantity, status)
);

-- One row per follower per master fill. Created BEFORE the API call.
CREATE TABLE follower_orders (
  id              bigserial PRIMARY KEY,
  master_fill_id  bigint NOT NULL REFERENCES master_fills(id),
  follower_id     uuid NOT NULL REFERENCES accounts(id),
  idempotency_tag text NOT NULL UNIQUE,      -- also sent as Kite `tag`
  algo_id         text NOT NULL,
  intended_qty    integer NOT NULL,
  lot_size        integer NOT NULL,
  placed_qty      integer,
  kite_order_id   text,
  terminal_status text,                      -- COMPLETE|REJECTED|CANCELLED
  filled_qty      integer NOT NULL DEFAULT 0,
  average_price   numeric(18,4),
  attempt_count   integer NOT NULL DEFAULT 0,
  last_error      text,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (master_fill_id, follower_id)
);

-- Append-only transition log. The SEBI audit trail.
CREATE TABLE order_events (
  id                bigserial PRIMARY KEY,
  follower_order_id bigint REFERENCES follower_orders(id),
  master_fill_id    bigint REFERENCES master_fills(id),
  account_id        uuid NOT NULL REFERENCES accounts(id),
  event_type        text NOT NULL,   -- signal_received|sized|api_request|api_response|ws_update|reconciled|dead_lettered
  from_status       text,
  to_status         text,
  algo_id           text,
  egress_ip         inet,
  http_status       integer,
  latency_ms        integer,
  payload           jsonb NOT NULL,  -- request or response, verbatim
  occurred_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON order_events (account_id, occurred_at);
CREATE INDEX ON order_events (follower_order_id, id);

-- Kill-switch: SEBI-required, and the operational panic button.
CREATE TABLE kill_switch (
  scope       text PRIMARY KEY,   -- 'global' or an account uuid as text
  engaged     boolean NOT NULL,
  reason      text,
  engaged_by  text,
  engaged_at  timestamptz NOT NULL DEFAULT now()
);

-- Instrument master, refreshed daily. Lot size lives here.
CREATE TABLE instruments (
  instrument_token bigint PRIMARY KEY,
  exchange        text NOT NULL,
  tradingsymbol   text NOT NULL,
  lot_size        integer NOT NULL,
  tick_size       numeric(10,4) NOT NULL,
  segment         text,
  expiry          date,
  refreshed_at    timestamptz NOT NULL DEFAULT now()
);
```

**Audit-trail completeness** — for every API-placed order the log yields: originating master fill, algo id, egress IP, exact request body, exact broker response, HTTP status, timestamps at ms, and every subsequent WS transition. That is what a SEBI/exchange query asks for.

**Secrets never in Postgres.** `*_ref` columns hold pointers only. See §8.

---

## 3. Kite Connect integration

### 3.1 Which SDK calls, where

| Concern | Call | Package |
|---|---|---|
| Login exchange | `Client.GenerateSession(requestToken, apiSecret)` | `kite/session.go` |
| Token renew (if entitled) | `Client.RenewAccessToken(refresh, secret)` | `kite/session.go` |
| Logout / revoke | `Client.InvalidateAccessToken()` | `kite/session.go` |
| Master fill detection | `ticker.OnOrderUpdate(func(kiteconnect.Order))` | `listener` |
| Follower placement | `Client.PlaceOrder(variety, OrderParams{AlgoID, Tag, …})` | `worker` |
| Reconciliation truth | `Client.GetOrderHistory(orderID)`, `Client.GetOrders()` | `recon` |
| Capital ratio inputs | `Client.GetMargins()`, `Client.GetPositions()` | `engine` (cached) |
| Lot sizes | `Client.GetInstruments()` → `instruments` table | daily job |

### 3.2 Token lifecycle

Kite access tokens die ~06:00 IST daily and the login step needs an interactive Zerodha 2FA — it cannot be fully automated without the paid refresh-token entitlement. Design for the manual case and let entitlement shortcut it:

1. **Pre-open auth window (07:00–09:00 IST).** `envoytrade-auth` serves a page listing every account with a live-session check and a "Login" link to `https://kite.zerodha.com/connect/login?api_key=…`.
2. Zerodha redirects to our callback with `request_token`; handler runs `GenerateSession`, writes secret to the store, inserts `account_sessions` row, revokes the prior one.
3. **Readiness gate.** Engine refuses to dispatch for any follower without a live session; master listener refuses to start without the master's. Dashboard shows a red/green board so the operator sees at 08:55 exactly who is un-authed.
4. **Mid-session death.** Kite returns `TokenException` on a 403. On that error: mark session revoked, engage the per-account kill switch, page the operator, and let other followers keep trading. Never retry a token error — it will never succeed.

**Recommendation:** treat daily auth as a human ritual with a machine checklist, and add refresh-token automation later behind the same `session.Provider` interface. Alternative considered — headless browser automation of the 2FA flow; rejected: brittle, likely a ToS violation, and a silent failure mode on the critical path.

### 3.3 Static IP for REST — dedicated proxy vendor (algoip.in / staticip.in), solved via `SetHTTPClient`

**Decision: buy per-account dedicated proxies from a vendor (algoip.in, staticip.in), not `net.Dialer.LocalAddr` binding on a self-managed host.** Confirmed by the user — this is the actual procurement path, not a hypothetical alternative. Each account gets a vendor-assigned host/port and a client id + secret used as HTTP Basic Auth to the proxy, e.g. (vendor's own Python example, translated below):

```
https://<client_id>:<client_secret>@dc46-mum-01.algoip.in:443
```

`Client.SetHTTPClient` takes this without touching the SDK — the proxy is just an `http.Transport.Proxy`:

```go
// internal/kite/proxy.go — all proxy-vendor knowledge lives here,
// nothing in gokiteconnect changes.
package kite

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// ProxyConfig is what the vendor dashboard hands you per account.
type ProxyConfig struct {
	Scheme string // "http" or "https" — vendor tells you which port wants which
	Host   string // e.g. "dc46-mum-01.algoip.in"
	Port   int
	ClientID     string
	ClientSecret string
}

func (p ProxyConfig) url() (*url.URL, error) {
	u := &url.URL{
		Scheme: p.Scheme,
		User:   url.UserPassword(p.ClientID, p.ClientSecret), // handles escaping
		Host:   fmt.Sprintf("%s:%d", p.Host, p.Port),
	}
	return url.Parse(u.String())
}

// RESTClientFor builds an *http.Client that egresses every request
// through this account's dedicated proxy IP. Go's http.Transport has
// supported an "https" scheme Proxy URL (TLS to the proxy itself, the
// vendor's HTTPS_PROXY case) natively since Go 1.10 — no extra library.
func RESTClientFor(cfg ProxyConfig) (*http.Client, error) {
	proxyURL, err := cfg.url()
	if err != nil {
		return nil, fmt.Errorf("bad proxy config: %w", err)
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:               http.ProxyURL(proxyURL),
			MaxIdleConnsPerHost: 4,
		},
		Timeout: 10 * time.Second,
	}, nil
}
```

Wired at account setup, unchanged from the rest of the plan:

```go
kc := kiteconnect.New(account.APIKey)
httpClient, err := kite.RESTClientFor(account.ProxyConfig)
if err != nil { return err }
kc.SetHTTPClient(httpClient)
kc.SetAccessToken(account.AccessToken)
```

`accounts.egress_proxy_url` (§2) stores the fully-formed vendor URL (or its component fields); `egress_ip` becomes informational only — the IP actually observed, not something the service binds to directly.

**Alternative considered — self-host N secondary IPs and use `net.Dialer.LocalAddr`.** Cheaper per-IP and removes a vendor hop, but shifts IP acquisition, routing, and Zerodha whitelisting logistics onto us, and static IPs in Indian DCs are the exact thing algoip.in/staticip.in exist to commoditize. Rejected in favor of buying the vendor's product — engineering effort goes into the `ProxyConfig` seam above either way, so the code cost of self-hosting later is small if a vendor ever needs replacing.

**Silent-misconfiguration guard, unchanged in intent:** on startup and hourly, each account's client hits a benign endpoint (e.g. `httpbin.org/ip` or Kite's own `/user/profile`) through its own proxy and logs the observed source IP; mismatch against the IP Zerodha has whitelisted for that account engages that account's kill switch. A wrong-source-IP rejection reaching the live order path is worse than not placing the order.

### 3.4 Static IP for WS — vendor proxy does not solve the ticker, by design

The same per-account proxy **cannot** be handed to `ticker.Serve()`: as noted above, `ticker/ticker.go:296` dials through gorilla's shared package-level `websocket.DefaultDialer`, whose `Proxy` field is fixed to `http.ProxyFromEnvironment` and is mutated in place by every `Ticker` instance. There is no per-instance override, and the vendored SDK is not to be modified. Consequences, in order of preference:

1. **Run exactly one `Ticker` — the master's.** Order-update listening is one connection for the whole system; it doesn't need a follower-specific IP, since (per the stated constraint) pure listening likely doesn't require whitelisting at all. **Confirm this with Zerodha before Milestone 2** — cheapest possible resolution, and everything below only matters if it fails.
2. **If master listening does need a whitelisted IP:** set `HTTPS_PROXY`/`HTTP_PROXY` process-wide env vars to the master account's vendor proxy before the ticker's `Dial` runs (gorilla's `DefaultDialer.Proxy` reads `http.ProxyFromEnvironment` lazily per dial). This works *only* because there is exactly one `Ticker` in the process — a second one would silently share the same env-derived proxy, which is why follower tickers are ruled out entirely rather than attempted.
3. **Followers never get their own ticker.** Follower order status comes from the reconciliation loop polling `GetOrderHistory` over each follower's already-proxied REST client (§5). This is not a compromise bolted on for the WS limitation — the reconciler must exist regardless as the audit-trail source of truth, so routing follower status through it costs nothing extra; it only adds 1–3 s of latency for status updates on the dashboard.

**Recommendation:** verify (1); fall back to (2) only if forced; (3) is the design regardless of how (1)/(2) resolve. Do not attempt per-follower WS proxying — it requires editing `gokiteconnect`, which is off the table.

---

## 4. Core engine logic

### 4.1 Durable-write-then-dispatch (transactional outbox)

The channel is a *latency optimisation*, never the source of truth.

```
ticker OnOrderUpdate
  └─ filter: status==COMPLETE, master account, not seen
  └─ TX: INSERT master_fills (dispatch_state='pending')   ← durable point
  └─ non-blocking send on chan (drop on full — outbox drainer covers it)

engine consumer
  └─ read fill, load enabled follow_links
  └─ TX: INSERT follower_orders (one per follower, idempotency_tag)
     ...   UPDATE master_fills SET dispatch_state='dispatched'
  └─ hand each follower_order to its worker

outbox drainer (every 2s)
  └─ SELECT * FROM master_fills WHERE dispatch_state='pending'
       AND received_at < now() - interval '2 seconds'
       FOR UPDATE SKIP LOCKED
  └─ re-inject into the engine
```

This is what makes the V2 queue swap trivial: the outbox already guarantees at-least-once, so replacing `memchan` with Redis Streams changes latency and process topology, not correctness. It also means a crash between listener and engine loses nothing.

**Idempotency.** `idempotency_tag` = short deterministic hash of `(master_fill_id, follower_id)`, ≤20 chars, passed as Kite's `tag`. Written to the DB before the API call, so a duplicated dispatch collides on the unique index instead of double-placing. Before any retry, first check `GetOrders()` for that tag — this is the one thing standing between a network timeout and a duplicate live order.

### 4.2 Sizing

```go
// Pure, table-tested, zero deps. This is the highest-risk function in
// the system: a bug here places real money in the market.
func SizeOrder(masterQty int, ratio decimal.Decimal, lotSize int, maxQty int) (int, SizingReason) {
    if lotSize <= 0 { return 0, ReasonBadInstrument }
    raw := decimal.NewFromInt(int64(masterQty)).Mul(ratio)
    lots := raw.Div(decimal.NewFromInt(int64(lotSize))).
                Floor().                     // always floor: never exceed intent
                IntPart()
    qty := int(lots) * lotSize
    if qty == 0 { return 0, ReasonBelowOneLot }
    if maxQty > 0 && qty > maxQty { return maxQty - maxQty%lotSize, ReasonCapped }
    return qty, ReasonOK
}
```

Decisions: **floor, not round** — overshooting the master's intent is strictly worse than undershooting. **Skip, don't upsize, below one lot** — record `ReasonBelowOneLot` on the `follower_order` and count it in a metric, because a follower who never trades due to a too-small ratio must be visible, not silent. Use `shopspring/decimal`, never `float64`, for the ratio multiply.

Capital ratio is a stored per-link constant in V1, not derived live from `GetMargins()`. Alternative considered — recompute from live margins per signal; rejected: adds an API call and a failure mode to the hot path, and makes position size non-reproducible after the fact, which breaks audit.

### 4.3 Per-follower worker isolation

One goroutine per follower, each with its own inbound buffered channel, its own `Broker` (own egress), its own rate limiter, and its own panic recovery. A follower's queue backing up, timing out, or panicking cannot touch another's — the engine's send to a full follower channel is non-blocking, and an overflow marks that `follower_order` dead-lettered rather than stalling fan-out.

```go
type Worker struct {
    followerID string
    in         chan Job            // buffered, cap ~64
    limiter    *rate.Limiter       // rate.NewLimiter(8, 2) — per account
    breaker    *circuitBreaker     // trip on 5 consecutive failures
    broker     kite.Broker
}
```

**Recommendation: goroutine-per-follower.** Follower count is small (tens), isolation is the whole point, and per-account rate limiting falls out naturally from a per-account limiter. Alternative considered — a shared bounded pool with account-keyed limiters; better at hundreds of accounts, but one slow account can occupy pool slots and delay others, which is exactly the failure this constraint forbids. Revisit past ~200 followers.

Limiter set to 8/s against the ~10/s documented cap — headroom for the reconciler's reads on the same account.

### 4.4 Retry policy

| Failure | Retry? | Policy |
|---|---|---|
| Network timeout, 5xx | Yes | 3 attempts, 200 ms → 400 ms → 800 ms + jitter. **Query `GetOrders()` by tag first** — the order may already be live. |
| `NetworkException` (Kite: order status unknown) | No blind retry | Reconcile via `GetOrderHistory`, then decide. |
| 429 rate limited | Yes | Respect limiter, back off 1 s, max 5 attempts. |
| `TokenException` (403) | No | Revoke session, engage per-account kill switch, page. |
| `InputException` (bad symbol/qty) | No | Terminal; dead-letter with the exact request logged. |
| Margin shortfall / `OrderException` | No | Terminal; alert — the follower is underfunded, no retry fixes that. |
| Kill switch engaged | No | Do not place. Record `killed` and move on. |

Hard rule: **stop retrying at market close**, and never retry a signal older than a configurable staleness bound (default 30 s). A late copy-trade is often worse than no copy-trade — the price has moved and the follower gets a fill the master never had.

---

## 5. Failure handling

**Reconciliation loop** (every 5 s during market hours, plus a full pass at close). For each `follower_order` without a terminal status: `GetOrderHistory(kite_order_id)` on that follower's client, append every unseen transition to `order_events`, update the derived row. Handles partial fills (`filled_qty < placed_qty`) and any WS update the ticker missed — which is the point: the reconciler, not the WS, is the authority. WS is the fast path; reconciliation is the correct one.

**Drift detection.** After a master fill reaches terminal state across all its follower orders, compute per follower:
`drift = filled_qty − intended_qty`, and `expected = floor(master_filled × ratio / lot) × lot`.
Alert when a follower is more than one lot away from expectation, when a follower is REJECTED while others succeeded (usually margin or token), or when the master partially filled and followers filled full size — the master's own partial must scale down follower intent, not be treated as a complete fill. This last case is the subtle one and deserves an explicit test.

**Dead-letter handling.** No separate DLQ table in V1 — `master_fills.dispatch_state='dead'` and `follower_orders.last_error` *are* the dead-letter store, queryable and joinable to the audit log. A dashboard page lists them with a manual replay button (replay re-runs sizing fresh, never reuses a stale quantity). Alternative considered — a dedicated `dead_letters` table; rejected as a second source of truth for the same fact.

**Alerting (V1, deliberately crude).** Log at ERROR + a webhook (Slack/Telegram) for: master ticker disconnected >30 s, any un-authed account after 09:10 IST, drift alert, dead-letter created, egress-IP mismatch, kill switch engaged. All behind one `obs.Alerter` interface so V2's Prometheus Alertmanager replaces the impl and nothing else.

**Kill switch.** Checked in three places: engine before fan-out (global), worker before each place (per-account), and admin API to engage. Engaging must be *fast and obvious* — a single dashboard button, no confirmation dialog chain. Also auto-engaged by: egress mismatch, 5 consecutive failures on an account, drift beyond a hard threshold.

---

## 6. Milestone sequencing

Estimates assume one experienced Go dev. Market-hours testing is the real bottleneck, not code.

| # | Milestone | Exit criterion | Est. |
|---|---|---|---|
| 0 | **Legal + access spike.** Confirm SEBI standing for the intended follower set, obtain algo id, confirm whether WS listening needs a whitelisted IP (§3.4), confirm refresh-token entitlement. | Written answers on all four. **Blocks 3 onward.** | 3–5 d, mostly waiting |
| 1 | **Skeleton + store.** Repo, config, migrations, `sqlc`, instrument sync job, `obs` facades. | `go test ./...` green; instruments table populated. | 3 d |
| 2 | **Master listener standalone.** Ticker + `OnOrderUpdate`, master fills persisted with raw payload, no fan-out. | A manual master order appears in `master_fills` within 1 s. | 3 d |
| 3 | **Single follower end-to-end.** Auth flow, egress-bound REST client, sizing, one worker, algo tag on every order. Smallest possible real quantities. | 1 master order → 1 correctly sized follower order, full audit chain. | 5 d |
| 4 | **N followers.** Worker-per-follower, per-account limiters, isolation tests, breakers, kill switch. | Force-fail one follower (bad token): others unaffected, alert fires. | 4 d |
| 5 | **Reconciliation + drift.** Recon loop, partial-fill handling, dead-letter, alerting. | Injected partial fill and rejection both reconcile with correct drift alerts. | 4 d |
| 6 | **Dashboard.** Auth board, live orders, drift view, dead-letter replay, kill-switch button. Server-rendered Go templates + HTMX. | Operator runs a full session from the dashboard alone. | 4 d |
| 7 | **Hardening.** Outbox drainer under crash tests, restart mid-session, load test, runbook. | Kill -9 mid-dispatch: zero lost, zero duplicate. | 4 d |

~30 working days plus Milestone 0's calendar time. Note the IP whitelist's once-per-week change limit means **every IP you will ever need should be requested during Milestone 0** — getting this wrong costs a week per mistake, and it is the single most common way this class of project slips.

---

## 7. Testing strategy

**Unit (the bulk).** `domain` is pure — table-test `SizeOrder` exhaustively: ratios producing exact lots, sub-lot, huge, tiny, zero lot size, cap interaction, master partial fill. This function is where a bug costs money.

**Fake broker.** `internal/kite/fake` implements `Broker` with programmable latency, error injection (each Kite exception type), partial fills, and duplicate WS delivery. The vendored SDK ships `mock_responses/` and uses `jarcoal/httpmock` — reuse those fixtures so the fake's payload shapes stay honest.

**Integration.** `testcontainers-go` Postgres, real migrations, real store. Golden test on the full chain: injected WS frame → assert every `order_events` row in order. That golden test is the audit-trail regression guard.

**Crash tests.** Kill the process between the `master_fills` insert and the engine's dispatch; assert the outbox drainer recovers it exactly once. Repeat at each seam.

**Sandbox reality check.** Kite has no true paper-trading sandbox for order placement. Plan accordingly: use `httpmock` against captured real responses for correctness, then validate against production with **1-share/1-lot orders in liquid instruments during market hours**, with a hard `max_qty_per_order` and a small real-money budget as the blast radius. Do not skip this — the first real order will surprise you, and it should surprise you cheaply.

**Load test.** Not throughput-bound (a human master places tens of orders a day), so test the *pathological* case instead: 50 signals in 1 s across 50 followers, one follower hanging for 30 s. Assert per-account limiters hold, no cross-follower delay, no goroutine leak (`goleak`), memory flat.

---

## 8. Deployment (V1)

**Topology.** Single VM (Mumbai region — colocate with Zerodha for latency), Postgres on the same host or a managed instance in the same region. Binary under systemd with `Restart=always`. Deliberately boring: no k8s, no containers-in-anger. One process, one DB, one host is a *feature* at this scale — it removes an entire class of failure from the critical path.

**Static IPs.** Provisioned as dedicated proxies from a vendor (algoip.in, staticip.in), one per account, per §3.3 — no secondary IPs on the VM itself. Buy 2–3 spare proxy slots beyond current follower count during Milestone 0, since Zerodha's once-a-week whitelist change limit makes spares nearly free insurance against onboarding delay. Store each account's vendor host/port/credentials via `ProxyConfig` (§3.3), resolved observed IP in `accounts.egress_ip` for the self-check, not used to bind anything directly. Verify each with the startup + hourly self-check in §3.3.

**Secrets.** API secrets and daily access tokens in a real secret manager (cloud KMS/Secrets Manager, or `sops`+age for a self-hosted start), referenced by the `*_ref` columns. Access tokens are short-lived but are bearer credentials to a live trading account — treat them like the money they control. Never in Postgres, never in logs (add a redacting log hook and test it), never in env vars visible via `/proc`.

**Backups.** Postgres PITR + nightly dump off-host. `order_events` is the audit trail; losing it is a regulatory problem, not just an ops one. Retention per SEBI record-keeping (5+ years — confirm in Milestone 0).

**Ops.** Systemd timer at 07:00 IST posts the auth checklist to the alert webhook. Daily instrument sync at 07:30. Recon full pass at 15:35 IST.

---

## 9. Risk register

| # | Risk | Impact | Mitigation |
|---|---|---|---|
| 1 | **SEBI/regulatory gap** — third-party followers without agent or RA registration | Existential; shut down, penalties | Milestone 0 blocks all follower onboarding beyond own/family accounts until written legal clearance. Keep the follower set to family until then. Engineering cannot mitigate this — do not let it slip to "later". |
| 2 | **IP whitelist wrong or late** — once/week change limit | Every order rejected for a week | Request all IPs plus spares in one Milestone-0 batch. Startup + hourly egress self-check catches misconfig before market open, not during. |
| 3 | **Token refresh fails mid-session** | That follower silently stops copying | No live session ⇒ dispatch refused, not attempted. Per-account kill switch on `TokenException`, immediate page. Auth board makes un-authed accounts visible before 09:15. |
| 4 | **Sizing bug places wrong quantity** | Direct financial loss | Pure function, exhaustive table tests, floor-not-round, `max_qty_per_order` hard cap per link, small real-money blast radius in Milestone 3. |
| 5 | **Duplicate order on retry** | Double position, real loss | Idempotency tag written pre-call; `GetOrders()` tag check before every retry; no blind retry on `NetworkException`; DB unique constraint as last line. |
| 6 | **Master ticker disconnects unnoticed** | Signals silently missed | SDK auto-reconnect + heartbeat monitor; alert on >30 s gap; post-reconnect `GetOrders()` sweep for fills during the gap. The sweep is the part people forget. |
| 7 | **Master partial fill treated as complete** | Followers overexposed vs master | Size against `filled_quantity`, never `quantity`; drift detection compares against master's actual fill; explicit test for this case. |
| 8 | **Proxy vendor (algoip.in/staticip.in) outage** — this is now the sole egress path per §3.3, not a fallback | All orders for every affected account fail; if the vendor's whole DC drops, all accounts fail simultaneously | Health-check each account's proxy pre-open (§3.3's self-check) so an outage is known before 09:00, not discovered on a rejected order. Ask the vendor whether accounts can be split across their PoPs/DCs, so one DC's outage doesn't take the whole book down. Keep vendor support contact and SLA in the runbook. |
| 9 | **WS ticker limited to one instance** (§3.4) — cannot proxy per-follower without editing the vendored SDK, which is ruled out | Follower status only via REST polling, not push | Not a workaround, a design decision: reconciliation loop is the status source of truth regardless (§5), so this costs 1–3 s latency, not correctness. Revisit only if that latency proves to matter in practice. |
| 10 | **Single-host failure during market hours** | Full outage | Accepted for V1 with eyes open. Fast rebuild via IaC + documented runbook. Proxy vendor IPs are vendor-side, not host-side, so a host rebuild doesn't touch Zerodha's whitelist — lower risk than the self-hosted-IP alternative would have carried. |

---

## 10. Do NOT build in V1

- Redis, Kafka, NATS, or any external broker. `memchan` + the Postgres outbox only.
- Multi-process or microservice split. One binary.
- OTel SDK, Collector, Jaeger, Loki, ELK. `obs` facades wrapping `slog` + `expvar` counters only.
- Prometheus, Grafana, metrics ingestion service.
- Many-to-many follow relationships, follower-selectable strategies, per-instrument filters.
- SL/target/bracket-order mirroring, GTT copying, position squaring-off logic.
- Follower self-service onboarding, billing, or a public API.
- SPA dashboard. Server-rendered templates + HTMX.
- Horizontal scaling, leader election, sharding.
- Historical backtesting or P&L attribution.

The seams that make each of these cheap later: `queue.Publisher`/`Consumer` (external broker + process split), `obs.*` (the whole telemetry stack), `kite.BrokerFactory` (multi-broker), `follow_links` as its own table (many-to-many is a PK change). If a V1 change would violate one of those seams, that is the signal to stop and reconsider — not to widen V1's scope.
