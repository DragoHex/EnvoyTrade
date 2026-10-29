-- name: UpsertInstrument :exec
INSERT INTO instruments (instrument_token, broker, exchange, tradingsymbol, lot_size, tick_size, segment, security_id, exchange_segment, expiry, refreshed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
ON CONFLICT (instrument_token) DO UPDATE SET
  broker = EXCLUDED.broker, exchange = EXCLUDED.exchange, tradingsymbol = EXCLUDED.tradingsymbol,
  lot_size = EXCLUDED.lot_size, tick_size = EXCLUDED.tick_size,
  segment = EXCLUDED.segment, security_id = EXCLUDED.security_id,
  exchange_segment = EXCLUDED.exchange_segment, expiry = EXCLUDED.expiry, refreshed_at = now();

-- name: InstrumentLotSize :one
SELECT lot_size FROM instruments WHERE exchange = $1 AND tradingsymbol = $2 LIMIT 1;

-- name: PruneInstrumentsByBroker :execrows
DELETE FROM instruments WHERE broker = $1;

-- name: ActiveFOInstrumentsForCache :many
SELECT 
    broker,
    exchange,
    tradingsymbol,
    lot_size,
    COALESCE(security_id, '')::text AS security_id,
    COALESCE(exchange_segment, '')::text AS exchange_segment,
    expiry
FROM instruments
WHERE expiry IS NOT NULL
  AND exchange = ANY(ARRAY['NFO', 'BFO', 'MCX']::text[])
  AND expiry >= CURRENT_DATE
  AND (
      (expiry >= date_trunc('month', CURRENT_DATE) AND expiry < (date_trunc('month', CURRENT_DATE) + INTERVAL '1 month'))
      OR expiry <= (CURRENT_DATE + INTERVAL '21 days')
  )
ORDER BY expiry ASC, tradingsymbol ASC
LIMIT 10000;
