package archive

import "time"

// MasterFillParquet represents a row in master_fills formatted for Parquet storage.
type MasterFillParquet struct {
	ID              int64   `parquet:"id"`
	MasterID        string  `parquet:"master_id"`
	BrokerOrderID   string  `parquet:"broker_order_id"`
	Exchange        string  `parquet:"exchange"`
	Tradingsymbol   string  `parquet:"tradingsymbol"`
	InstrumentToken int64   `parquet:"instrument_token"`
	TransactionType string  `parquet:"transaction_type"`
	Product         string  `parquet:"product"`
	OrderType       string  `parquet:"order_type"`
	FilledQuantity  int32   `parquet:"filled_quantity"`
	AveragePrice    string  `parquet:"average_price"`
	Status          string  `parquet:"status"`
	OrderTimestamp  int64   `parquet:"order_timestamp,timestamp(microsecond)"`
	RawPayload      string  `parquet:"raw_payload"`
	ReceivedAt      int64   `parquet:"received_at,timestamp(microsecond)"`
	DispatchState   string  `parquet:"dispatch_state"`
	DispatchedAt    *int64  `parquet:"dispatched_at,timestamp(microsecond)"`
}

// FollowerOrderParquet represents a row in follower_orders formatted for Parquet storage.
type FollowerOrderParquet struct {
	ID              int64   `parquet:"id"`
	MasterFillID    *int64  `parquet:"master_fill_id"`
	FollowerID      string  `parquet:"follower_id"`
	IdempotencyTag  string  `parquet:"idempotency_tag"`
	IntendedQty     int32   `parquet:"intended_qty"`
	LotSize         int32   `parquet:"lot_size"`
	SizingReason    int32   `parquet:"sizing_reason"`
	PlacedQty       *int32  `parquet:"placed_qty"`
	BrokerOrderID   *string `parquet:"broker_order_id"`
	TerminalStatus  *string `parquet:"terminal_status"`
	FilledQty       int32   `parquet:"filled_qty"`
	AveragePrice    *string `parquet:"average_price"`
	AttemptCount    int32   `parquet:"attempt_count"`
	LastError       *string `parquet:"last_error"`
	Origin          string  `parquet:"origin"`
	Tradingsymbol   string  `parquet:"tradingsymbol"`
	Exchange        string  `parquet:"exchange"`
	Product         string  `parquet:"product"`
	TransactionType string  `parquet:"transaction_type"`
	OrderType       string  `parquet:"order_type"`
	CreatedAt       int64   `parquet:"created_at,timestamp(microsecond)"`
	UpdatedAt       int64   `parquet:"updated_at,timestamp(microsecond)"`
}

// OrderEventParquet represents a row in order_events formatted for Parquet storage.
type OrderEventParquet struct {
	ID              int64   `parquet:"id"`
	FollowerOrderID *int64  `parquet:"follower_order_id"`
	MasterFillID    *int64  `parquet:"master_fill_id"`
	AccountID       string  `parquet:"account_id"`
	EventType       string  `parquet:"event_type"`
	FromStatus      *string `parquet:"from_status"`
	ToStatus        *string `parquet:"to_status"`
	HTTPStatus      *int32  `parquet:"http_status"`
	LatencyMS       *int32  `parquet:"latency_ms"`
	Payload         string  `parquet:"payload"`
	OccurredAt      int64   `parquet:"occurred_at,timestamp(microsecond)"`
}

// TimeToMicros converts time.Time to microseconds since Unix epoch.
func TimeToMicros(t time.Time) int64 {
	return t.UnixNano() / 1000
}

// MicrosToTime converts microseconds since Unix epoch to UTC time.Time.
func MicrosToTime(micros int64) time.Time {
	return time.Unix(micros/1_000_000, (micros%1_000_000)*1_000).UTC()
}

// Summary of an archival run.
type ArchiveSummary struct {
	CutoffDate            string
	MasterFillsArchived   int
	FollowerOrdersArchived int
	OrderEventsArchived   int
	PartitionDates        []string
	FilesWritten          []string
}

// Summary of a restore run.
type RestoreSummary struct {
	FromDate              string
	UptoDate              string
	MasterFillsRestored   int
	FollowerOrdersRestored int
	OrderEventsRestored   int
	PartitionsRestored    []string
}
