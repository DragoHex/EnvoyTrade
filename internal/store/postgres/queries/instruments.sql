-- name: UpsertInstrument :exec
INSERT INTO instruments (instrument_token, exchange, tradingsymbol, lot_size, tick_size, segment, expiry, refreshed_at)
VALUES ($1,$2,$3,$4,$5,$6,$7, now())
ON CONFLICT (instrument_token) DO UPDATE SET
  exchange = EXCLUDED.exchange, tradingsymbol = EXCLUDED.tradingsymbol,
  lot_size = EXCLUDED.lot_size, tick_size = EXCLUDED.tick_size,
  segment = EXCLUDED.segment, expiry = EXCLUDED.expiry, refreshed_at = now();

-- name: InstrumentLotSize :one
SELECT lot_size FROM instruments WHERE exchange = $1 AND tradingsymbol = $2;
