# Phase 5: Instrument Master Sync & Cache

This document specifies Phase 5 of the Angel One SmartAPI integration for EnvoyTrade.

---

## 1. Scope & Objective
Ingest, parse, and synchronize Angel One's master instruments catalog (`OpenAPIScripMaster.json`) into PostgreSQL and the in-memory cache (`instrument.Cache`). Stream the large JSON dataset without heap bloat, filter out cash equities to preserve storage, and maintain forward/reverse symbol translation indexes.

---

## 2. Ingestion Architecture (`internal/instrument`)

### 2.1 Streaming Ingestion Without Heap Bloat
- **Endpoint**: `https://margincalculator.angelbroking.com/OpenAPI_File/files/OpenAPIScripMaster.json` (~50 MB uncompressed, ~150,000 instruments).
- **Decoder**: Streams directly from `resp.Body` via `json.NewDecoder`. Iterates array tokens using `dec.More()` without reading the entire payload into RAM:
```go
dec := json.NewDecoder(resp.Body)
// Consume opening bracket '['
// Loop dec.More(): dec.Decode(&inst)
```

### 2.2 Cash Equity Filtering
To keep the `instruments` table compact and avoid bloating indexes, plain cash equity scrips (equities without expiry or `instrumenttype == ""`) are filtered out, saving all non-equity derivatives and commodities (`NFO`, `BFO`, `MCX`).

### 2.3 Transactional Pruning & Chunked UNNEST
1. **100% Success Guard**: Pruning (`DELETE FROM instruments WHERE broker = 'angelone'`) executes **only after** 100% of the JSON feed has been downloaded and parsed into valid domain records.
2. **Chunked Insertion**: Records are inserted in chunks of 5,000 using PostgreSQL parameterized `UNNEST` queries (`store.BatchInsertInstruments`).
3. **Vacuum & Analyze**: Executes `VACUUM (ANALYZE) instruments` immediately following the sync to reclaim storage and update planner statistics.

### 2.4 In-Memory Cache Auto-Load (`instrument.Cache`)
- Automatically loads active F&O derivative contracts for the current calendar month and next 3 weeks rolling into `instrument.Cache` (10,000 pre-allocated capacity).
- Maintains:
  - **Forward Lookup**: `(Exchange, Tradingsymbol)` &rarr; `(ExchangeSegment, SecurityToken, LotSize)`
  - **Reverse Lookup**: `(ExchangeSegment, SecurityToken)` &rarr; `(Exchange, Tradingsymbol)`

---

## 3. Implementation Details

### 3.1 Modified / New Files
- `internal/instrument/syncer.go`: `SyncAngelOne(ctx context.Context) error`, `SetAngelOneURL(url string)` for test injection, and registration in `StartDailyRoutine` (05:00 AM IST).
- `internal/instrument/syncer_test.go`: Pure TDD tests for streaming JSON parsing, failure isolation, and chunked batch insertion.

---

## 4. TDD Test Matrix

| Test Case | Scenario | Expected Behavior |
|---|---|---|
| `TestSyncAngelOne_StreamSuccess` | Valid mock `OpenAPIScripMaster.json` | Streams and parses contracts; batch inserts derivatives; vacuums table. |
| `TestSyncAngelOne_FilterCashEquities` | Feed contains mixture of cash equity and F&O | Drops cash equities; preserves F&O and commodities. |
| `TestSyncAngelOne_FailureIsolation` | Broker endpoint returns HTTP 500 | Fails with error; does NOT prune or alter existing DB instruments. |
| `TestSyncAngelOne_MalformedJSON` | Corrupted JSON stream midway | Aborts gracefully; leaves existing instruments intact. |
| `TestSyncAngelOne_CacheReload` | Sync completes successfully | Triggers `onComplete` callback; auto-loads active contracts into cache. |
| `TestSyncAngelOne_BatchChunking` | Feed contains >10,000 instruments | Inserts in exact chunks of 5,000 via UNNEST. |
