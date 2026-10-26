# Portfolio Greeks & Performance Analytics Plan (Alpha, Beta, Delta, Gamma)

This document specifies the technical design, data architecture, mathematical models, and implementation roadmap for tracking **Alpha ($\alpha$), Beta ($\beta$), Delta ($\Delta$), and Gamma ($\Gamma$)** across flexible historical horizons (**1 Week, 1 Month, 1 Year**) for master trading accounts and groups in EnvoyTrade.

---

## 1. Executive Summary & Goals

EnvoyTrade copy-trades on Indian markets (NSE/BSE/MCX) via Kite Connect. As trading groups execute discretionary and systematic strategies (F&O options writing, directional futures, equity baskets), traders need quantitative feedback on:
1. **Manager Edge (Alpha)**: Is the strategy outperforming the NIFTY 50 benchmark on a risk-adjusted basis?
2. **Systemic Market Exposure (Beta)**: How volatile and correlated is the account relative to the broader market?
3. **Directional Net Position (Delta)**: What is the net directional exposure of open positions (in equivalent benchmark shares/contracts)?
4. **Tail Risk & Convexity (Gamma)**: What is the rate of change of Delta (risk acceleration during sudden market moves, critical for short options strategies)?

### Core Invariant: Zero Load During Market Hours
- Live copy-trade execution, WebSocket order postbacks, and worker fan-out must **never be delayed** by analytics computation.
- All heavy metrics are derived from a **single daily End-of-Day (EOD) snapshot** captured after market close (15:35 IST).
- 1 Year of historical analytics per account requires only **~250 rows (~25 KB)** of storage.

---

## 2. Mathematical Definitions & Formulas

### 2.1. Daily Return Time-Series
For any account $p$ on trading day $t$:
$$R_{p, t} = \frac{\text{Daily P\&L}_t}{\text{EOD Equity}_{t-1}}$$

For the benchmark index $m$ (NIFTY 50) on trading day $t$:
$$R_{m, t} = \frac{\text{Close}_t - \text{Close}_{t-1}}{\text{Close}_{t-1}}$$

---

### 2.2. Beta ($\beta$) — Market Volatility Sensitivity
Measures systematic risk relative to the market:
$$\beta = \frac{\text{Cov}(R_p, R_m)}{\text{Var}(R_m)} = \frac{\sum_{t=1}^{N} (R_{p,t} - \bar{R}_p)(R_{m,t} - \bar{R}_m)}{\sum_{t=1}^{N} (R_{m,t} - \bar{R}_m)^2}$$

- **$\beta = 1.0$**: Portfolio moves exactly in tandem with NIFTY 50.
- **$0 < \beta < 1.0$**: Defensive strategy (less volatile than the market).
- **$\beta > 1.0$**: Aggressive/leveraged strategy.
- **$\beta \approx 0$**: Market-neutral strategy (e.g. delta-hedged options strangles/iron condors).

---

### 2.3. Alpha ($\alpha$) — Annualized Jensen's Alpha
Measures true excess return after compensating for market risk:
$$\alpha_{\text{daily}} = (\bar{R}_p - R_f) - \beta \times (\bar{R}_m - R_f)$$
$$\alpha_{\text{annualized}} = \alpha_{\text{daily}} \times 252$$

*(Where $R_f$ is the daily risk-free rate, defaulted to RBI 91-day T-Bill yield $\approx 6.5\%$ p.a., or $R_f \approx 0$ for excess return over benchmark).*

---

### 2.4. Delta ($\Delta$) — Directional Exposure
- **Point-in-Time (Live)**: Sum of individual instrument deltas multiplied by position size:
  $$\Delta_{\text{net}} = \sum_{i} (\Delta_i \times \text{Quantity}_i)$$
  - For Equity / Stock Futures: $\Delta_i = 1.0$ (or $-1.0$ for short).
  - For Options: $\Delta_i = \mathcal{N}(d_1)$ (Call) or $\mathcal{N}(d_1) - 1$ (Put) via standard Black-Scholes.
- **Historical Periods (1W, 1M, 1Y)**: **Average Net Delta Exposure** $\bar{\Delta}$:
  $$\bar{\Delta} = \frac{1}{N} \sum_{t=1}^{N} \Delta_{\text{net}, t}$$
  Provides insight into whether the strategy stayed predominantly long, short, or market-neutral.

---

### 2.5. Gamma ($\Gamma$) — Acceleration & Convexity
- **Point-in-Time (Live)**: Rate of change of Delta with respect to underlying spot price $S$:
  $$\Gamma_i = \frac{\mathcal{N}'(d_1)}{S \sigma \sqrt{T}}$$
  $$\Gamma_{\text{net}} = \sum_{i} (\Gamma_i \times \text{Quantity}_i)$$
- **Historical Periods (1W, 1M, 1Y)**: **Average Net Gamma Exposure** $\bar{\Gamma}$:
  $$\bar{\Gamma} = \frac{1}{N} \sum_{t=1}^{N} \Gamma_{\text{net}, t}$$
  - $\bar{\Gamma} < 0$: Typical for option sellers (theta decay positive, but tail risk during large gaps).
  - $\bar{\Gamma} > 0$: Typical for option buyers / breakout traders.

---

## 3. Minimal Data Architecture

### 3.1. What EnvoyTrade Already Has
- Realized & unrealized MTM per account in `account_margins` and `account_positions`.
- Historical trade fills in `master_fills` and `follower_orders`.
- Open positions quantities and product types in `account_positions`.

### 3.2. What Needs to Be Ingested Externally
- **NIFTY 50 Daily Closing Price**: 1 number per trading day (fetched from Kite Connect `NSE:NIFTY 50` quote or historical candles at 15:35 IST).

### 3.3. Database Migration: `daily_account_snapshots`
```sql
CREATE TABLE IF NOT EXISTS daily_account_snapshots (
    id              bigserial PRIMARY KEY,
    account_id      uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    snapshot_date   date NOT NULL,
    eod_equity      numeric(18,2) NOT NULL,
    daily_pnl       numeric(18,2) NOT NULL,
    benchmark_close numeric(18,2) NOT NULL,  -- NIFTY 50 close
    net_delta       numeric(12,4) NOT NULL DEFAULT 0,
    net_gamma       numeric(12,6) NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (account_id, snapshot_date)
);

CREATE INDEX IF NOT EXISTS idx_daily_snapshots_lookup 
ON daily_account_snapshots (account_id, snapshot_date DESC);
```

#### Storage Profile
- 1 row per trading day per account $\approx 100$ bytes.
- 1 Year of history for 10 accounts $\approx 2,500$ rows $\approx 250$ KB total storage.

---

## 4. Pipeline & Service Architecture

```
[ Market 09:15 - 15:30 IST ]
  → Zero analytics overhead. Live copy-trade engine executes unaffected.

[ Post-Market 15:35 IST Cron ]
  → internal/recon/eod_snapshot.go
     1. Read today's closing equity & P&L from account_margins.
     2. Fetch closing quote for NSE:NIFTY 50 via Kite broker client.
     3. Compute EOD Net Delta & Gamma from open account_positions (Black-Scholes).
     4. INSERT INTO daily_account_snapshots ... ON CONFLICT DO UPDATE.

[ HTTP Query GET /api/groups/{id}/greeks?timeframe=1w|1m|1y ]
  → internal/httpapi/greeks.go
     1. Query last N rows from daily_account_snapshots:
        - 1W: 5 trading days
        - 1M: 21 trading days
        - 1Y: 252 trading days
     2. Compute Cov/Var for Beta, mean excess return for Alpha.
     3. Return lightweight JSON payload.
```

---

## 5. API Design

### `GET /api/groups/{id}/greeks?timeframe={1w|1m|1y}`

#### Response (200 OK)
```json
{
  "timeframe": "1m",
  "tradingDays": 21,
  "benchmark": "NSE:NIFTY 50",
  "alpha": 0.142,
  "beta": 0.38,
  "avgNetDelta": 12.4,
  "avgNetGamma": -0.0042,
  "currentDelta": 8.0,
  "currentGamma": -0.0035,
  "benchmarkReturn": 0.024,
  "portfolioReturn": 0.058
}
```

---

## 6. UI & Dashboard Integration

### 6.1. Location: Hero Right Card or Analytics Page
The metrics can be embedded in the Right Card of the Dashboard hero container (utilizing previously empty space) with a dedicated timeframe toggle:

```
+-----------------------------------------------------------+-----------------------------------+
|  LEFT CARD: Social MTM Card (16:9)                        |  RIGHT CARD: Risk & Greek Stats   |
|                                                           |                                   |
|  [Logo] EnvoyTrade                  (Avatar) Jane Doe     |  PERFORMANCE & RISK GREEKS        |
|                                           @envoytrade     |  [ 1W ]  [ 1M (Active) ]  [ 1Y ]  |
|                                                           |  -------------------------------- |
|                   [ • TOTAL MTM TODAY ]                   |  ┌──────────────┬───────────────┐ |
|                        +₹42,500.00                        |  │ ALPHA (α)    │ BETA (β)      │ |
|             Masters: +₹32.5k • Followers: +₹10k           |  │ +14.2%       │ 0.38          │ |
|                                                           |  │ Outperforming│ Low Market Cor│ |
|                                                           |  ├──────────────┼───────────────┤ |
|  🛡️ Verified Trades         Taken @ 26 Oct 2026, 07:15 AM |  │ DELTA (Δ)    │ GAMMA (Γ)     │ |
|                                                           |  │ +12.4        │ -0.0042       │ |
|                                                           |  │ Mild Long    │ Net Option Sell│ |
|                                                           |  └──────────────┴───────────────┘ |
|                                                           |  Benchmark: NIFTY 50 • 21 Days    |
+-----------------------------------------------------------+-----------------------------------+
```

---

## 7. Implementation Roadmap

| Phase | Milestone | Description |
|---|---|---|
| **Phase 1: Storage** | Database Migration | Add `daily_account_snapshots` table and sqlc query models. |
| **Phase 2: Ingestion** | EOD Snapshot Daemon | Implement 15:35 IST daily runner in `internal/recon` fetching NIFTY 50 close and writing snapshots. |
| **Phase 3: Analytics Engine** | Quantitative Functions | Implement pure mathematical functions in `internal/domain/greeks.go` for Covariance, Beta, Jensen's Alpha, and Black-Scholes Greeks. |
| **Phase 4: API Endpoint** | `GET /api/groups/{id}/greeks` | Expose cached timeframe calculations in `internal/httpapi`. |
| **Phase 5: Frontend Component** | Right Card / Analytics Grid | Build SolidJS metric tiles with `1W / 1M / 1Y` toggle pills and tooltips explaining each metric. |
