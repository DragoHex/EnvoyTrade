# Phase 5: Instrument Master Sync & Unified Cache

This document specifies Phase 5 of the Kotak Neo integration for EnvoyTrade.

---

## 1. Scope & Objective
Download, parse, and ingest Kotak Neo's scrip master CSV files daily at `05:00:00 AM IST`. Enforce the all-or-nothing transactional prune guard, execute chunked batch inserts of 5,000 records using PostgreSQL parameterized `UNNEST` queries, run `VACUUM ANALYZE instruments`, and pre-load active F&O contracts into the pre-allocated 10,000-capacity in-memory cache (`instrument.Cache`).

---

## 2. Technical Design

### 2.1 File Discovery & Streaming Download
- Dynamic discovery via `GET {BASE_URL}/script-details/1.0/masterscrip/file-paths` or downloading segment CSVs:
  - `nse_cm.csv`, `nse_fo.csv`, `bse_cm.csv`, `bse_fo.csv`, `mcx_fo.csv`.
- Stream row-by-row with `csv.Reader` directly from HTTP response body.
- Never buffer entire unparsed CSV files into memory.

### 2.2 Field Extraction & Token Offset
- `pSymbol` $\to$ integer token / security ID.
  - To prevent token collision with Kite and Dhan, offset Kotak tokens by $20 \times 10^{11}$ ($20,000,000,000,000 + \text{token}$).
- `pTrdSymbol` $\to$ canonical `Tradingsymbol`.
- `lLotSize` $\to$ canonical `LotSize`.
- `pInstType` $\to$ `InstrumentType` (`FUTSTK`, `OPTSTK`, `EQ`).
- `pExpiryDate` $\to$ `Expiry`.
- `dStrikePrice` $\to$ `StrikePrice`.

### 2.3 Transactional Pruning (All-or-Nothing Guard)
- **Strict Rule**: `DELETE FROM instruments WHERE broker = 'kotakneo'` is executed **strictly only after 100% of all segment files are successfully downloaded and parsed**.
- If any segment file download fails or times out, the entire operation aborts without mutating existing database records.

### 2.4 Chunked Insertion & Storage Reclaim
- Batch insert in chunks of 5,000 using `store.BatchInsertInstruments` (`UNNEST($1::text[], ...)`).
- Execute `VACUUM (ANALYZE) instruments` immediately after batch insert.

### 2.5 In-Memory Cache Pre-Load (`instrument.Cache`)
- Automatically executed on startup and after the 05:00 AM daily sync.
- Loads active F&O derivatives for the current calendar month and next 3 weeks rolling via `store.ActiveFOInstrumentsForCache(ctx)`.
- Re-uses allocated backing memory using `clear(map)`.
- Pre-allocated 10,000 entry ceiling with DB fallback on miss.

---

## 3. Test-Driven Development (TDD) Test Matrix

| Case Type | Test Name | Scenario / Setup | Expected Outcome |
|---|---|---|---|
| **Positive** | `TestKotakSync_StreamingParse_Success` | Mock HTTP server streams sample `nse_fo.csv` and `nse_cm.csv`. | Correctly parses all records, maps lot sizes and strike prices, sets broker tag `'kotakneo'`. |
| **Positive** | `TestKotakSync_AllOrNothingGuard_Success` | Full stream finishes 100% without error. | Database prune query is executed followed by chunked `UNNEST` batch inserts. |
| **Positive** | `TestKotakSync_VacuumAnalyze_Invoked` | Batch insertion completes. | Verifies `VACUUM ANALYZE instruments` was triggered. |
| **Negative** | `TestKotakSync_StreamFailure_PreservesDB` | HTTP connection drops halfway through `nse_fo.csv`. | Operation aborts immediately; database prune query is NEVER called. |
| **Negative** | `TestKotakSync_HTTP500_FailsGracefully` | Endpoint returns HTTP 500. | Returns error, leaving previous instruments untouched. |
| **Edge Case** | `TestKotakSync_MalformedRow_Skipped` | CSV row has missing column or corrupt lot size. | Corrupt row skipped and logged; valid rows continue parsing. |
| **Edge Case** | `TestKotakCache_PreloadActiveFO` | Cache loads active F&O contracts for current month and next 3 weeks. | Maps forward `(exchange, tradingsymbol) -> (securityID, lotSize)` and reverse `(segment:securityID) -> (exchange, tradingsymbol)`. |
