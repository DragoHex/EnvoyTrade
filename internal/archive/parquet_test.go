package archive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/zstd"
)

func TestMasterFillParquet_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "master_fills.parquet")

	now := time.Now().UTC().Truncate(time.Microsecond)
	nowMicros := TimeToMicros(now)
	dispatched := nowMicros + 1000

	expected := []MasterFillParquet{
		{
			ID:              1,
			MasterID:        "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
			BrokerOrderID:   "BO12345",
			Exchange:        "NFO",
			Tradingsymbol:   "NIFTY26OCT25000CE",
			InstrumentToken: 12345678,
			TransactionType: "BUY",
			Product:         "NRML",
			OrderType:       "MARKET",
			FilledQuantity:  75,
			AveragePrice:    "142.8500",
			Status:          "COMPLETE",
			OrderTimestamp:  nowMicros,
			RawPayload:      `{"order_id":"BO12345","status":"COMPLETE"}`,
			ReceivedAt:      nowMicros,
			DispatchState:   "dispatched",
			DispatchedAt:    &dispatched,
		},
		{
			ID:              2,
			MasterID:        "b2c3d4e5-f6a7-8b9c-0d1e-2f3a4b5c6d7e",
			BrokerOrderID:   "BO12346",
			Exchange:        "NSE",
			Tradingsymbol:   "RELIANCE",
			InstrumentToken: 738561,
			TransactionType: "SELL",
			Product:         "CNC",
			OrderType:       "LIMIT",
			FilledQuantity:  10,
			AveragePrice:    "2950.5000",
			Status:          "REJECTED",
			OrderTimestamp:  nowMicros,
			RawPayload:      `{"error":"RMS limit"}`,
			ReceivedAt:      nowMicros,
			DispatchState:   "dead",
			DispatchedAt:    nil, // Edge case: nil DispatchedAt
		},
	}

	// Positive: Write with ZSTD compression
	err := parquet.WriteFile(filePath, expected, parquet.Compression(&zstd.Codec{}))
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Read back and compare
	actual, err := parquet.ReadFile[MasterFillParquet](filePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if len(actual) != len(expected) {
		t.Fatalf("expected %d rows, got %d", len(expected), len(actual))
	}

	for i := range expected {
		exp := expected[i]
		act := actual[i]

		if exp.ID != act.ID || exp.MasterID != act.MasterID || exp.BrokerOrderID != act.BrokerOrderID ||
			exp.Exchange != act.Exchange || exp.Tradingsymbol != act.Tradingsymbol ||
			exp.FilledQuantity != act.FilledQuantity || exp.AveragePrice != act.AveragePrice ||
			exp.Status != act.Status || exp.DispatchState != act.DispatchState {
			t.Errorf("row %d mismatch: expected %+v, got %+v", i, exp, act)
		}

		if (exp.DispatchedAt == nil && act.DispatchedAt != nil) ||
			(exp.DispatchedAt != nil && act.DispatchedAt == nil) ||
			(exp.DispatchedAt != nil && act.DispatchedAt != nil && *exp.DispatchedAt != *act.DispatchedAt) {
			t.Errorf("row %d DispatchedAt mismatch: expected %v, got %v", i, exp.DispatchedAt, act.DispatchedAt)
		}
	}
}

func TestFollowerOrderParquet_RoundTrip_Nullables(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "follower_orders.parquet")

	nowMicros := TimeToMicros(time.Now().UTC())
	masterFillID := int64(100)
	placedQty := int32(50)
	brokerOrderID := "FO_BROKER_999"
	terminalStatus := "COMPLETE"
	avgPrice := "142.9000"
	lastErr := "broker timeout"

	// Edge case 1: Fully populated row
	// Edge case 2: All optional/nullable pointer fields nil
	expected := []FollowerOrderParquet{
		{
			ID:              10,
			MasterFillID:    &masterFillID,
			FollowerID:      "11111111-1111-1111-1111-111111111111",
			IdempotencyTag:  "tag-100-111",
			IntendedQty:     50,
			LotSize:         25,
			SizingReason:    0,
			PlacedQty:       &placedQty,
			BrokerOrderID:   &brokerOrderID,
			TerminalStatus:  &terminalStatus,
			FilledQty:       50,
			AveragePrice:    &avgPrice,
			AttemptCount:    1,
			LastError:       &lastErr,
			Origin:          "copy_trade",
			Tradingsymbol:   "NIFTY26OCT25000CE",
			Exchange:        "NFO",
			Product:         "NRML",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			CreatedAt:       nowMicros,
			UpdatedAt:       nowMicros,
		},
		{
			ID:              11,
			MasterFillID:    nil, // nil master fill
			FollowerID:      "22222222-2222-2222-2222-222222222222",
			IdempotencyTag:  "tag-manual-222",
			IntendedQty:     0,
			LotSize:         25,
			SizingReason:    1,
			PlacedQty:       nil, // nil placed qty
			BrokerOrderID:   nil, // nil broker order id
			TerminalStatus:  nil, // nil terminal status
			FilledQty:       0,
			AveragePrice:    nil, // nil avg price
			AttemptCount:    0,
			LastError:       nil, // nil last error
			Origin:          "manual",
			Tradingsymbol:   "NIFTY26OCT25000CE",
			Exchange:        "NFO",
			Product:         "NRML",
			TransactionType: "BUY",
			OrderType:       "MARKET",
			CreatedAt:       nowMicros,
			UpdatedAt:       nowMicros,
		},
	}

	err := parquet.WriteFile(filePath, expected, parquet.Compression(&zstd.Codec{}))
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	actual, err := parquet.ReadFile[FollowerOrderParquet](filePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if len(actual) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(actual))
	}

	// Verify row 1 (all populated)
	if actual[0].MasterFillID == nil || *actual[0].MasterFillID != 100 {
		t.Errorf("row 1 MasterFillID mismatch: %v", actual[0].MasterFillID)
	}
	if actual[0].AveragePrice == nil || *actual[0].AveragePrice != avgPrice {
		t.Errorf("row 1 AveragePrice mismatch: %v", actual[0].AveragePrice)
	}

	// Verify row 2 (all nils preserved)
	if actual[1].MasterFillID != nil {
		t.Errorf("row 2 MasterFillID should be nil, got: %v", actual[1].MasterFillID)
	}
	if actual[1].PlacedQty != nil {
		t.Errorf("row 2 PlacedQty should be nil, got: %v", actual[1].PlacedQty)
	}
	if actual[1].BrokerOrderID != nil {
		t.Errorf("row 2 BrokerOrderID should be nil, got: %v", actual[1].BrokerOrderID)
	}
	if actual[1].AveragePrice != nil {
		t.Errorf("row 2 AveragePrice should be nil, got: %v", actual[1].AveragePrice)
	}
}

func TestOrderEventParquet_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "order_events.parquet")

	nowMicros := TimeToMicros(time.Now().UTC())
	followerOrderID := int64(42)
	masterFillID := int64(10)
	fromStatus := "OPEN"
	toStatus := "COMPLETE"
	httpStatus := int32(200)
	latencyMS := int32(18)

	expected := []OrderEventParquet{
		{
			ID:              1,
			FollowerOrderID: &followerOrderID,
			MasterFillID:    &masterFillID,
			AccountID:       "33333333-3333-3333-3333-333333333333",
			EventType:       "status_update",
			FromStatus:      &fromStatus,
			ToStatus:        &toStatus,
			HTTPStatus:      &httpStatus,
			LatencyMS:       &latencyMS,
			Payload:         `{"event":"fill","qty":75}`,
			OccurredAt:      nowMicros,
		},
		{
			ID:              2,
			FollowerOrderID: nil,
			MasterFillID:    nil,
			AccountID:       "33333333-3333-3333-3333-333333333333",
			EventType:       "audit_log",
			FromStatus:      nil,
			ToStatus:        nil,
			HTTPStatus:      nil,
			LatencyMS:       nil,
			Payload:         `{}`,
			OccurredAt:      nowMicros,
		},
	}

	err := parquet.WriteFile(filePath, expected, parquet.Compression(&zstd.Codec{}))
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	actual, err := parquet.ReadFile[OrderEventParquet](filePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if len(actual) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(actual))
	}

	if *actual[0].LatencyMS != 18 {
		t.Errorf("expected LatencyMS 18, got %v", *actual[0].LatencyMS)
	}
	if actual[1].LatencyMS != nil {
		t.Errorf("expected LatencyMS nil, got %v", actual[1].LatencyMS)
	}
}

func TestTimeConversionEdgeCases(t *testing.T) {
	// Epoch
	epoch := time.Unix(0, 0).UTC()
	micros := TimeToMicros(epoch)
	if micros != 0 {
		t.Errorf("expected 0, got %d", micros)
	}
	back := MicrosToTime(micros)
	if !back.Equal(epoch) {
		t.Errorf("expected %v, got %v", epoch, back)
	}

	// Sub-millisecond precision
	t1 := time.Date(2026, 10, 23, 14, 30, 45, 123456000, time.UTC)
	micros1 := TimeToMicros(t1)
	back1 := MicrosToTime(micros1)
	if !back1.Equal(t1) {
		t.Errorf("expected %v, got %v", t1, back1)
	}
}

func TestEmptyParquetFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "empty.parquet")

	var empty []MasterFillParquet
	err := parquet.WriteFile(filePath, empty)
	if err != nil {
		t.Fatalf("expected write empty slice to succeed, got %v", err)
	}

	read, err := parquet.ReadFile[MasterFillParquet](filePath)
	if err != nil {
		t.Fatalf("expected read empty file to succeed, got %v", err)
	}
	if len(read) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(read))
	}
}

func TestNonExistentParquetFile(t *testing.T) {
	_, err := parquet.ReadFile[MasterFillParquet]("/non/existent/path/data.parquet")
	if err == nil {
		t.Fatalf("expected error reading nonexistent file, got nil")
	}
	if !os.IsNotExist(err) && err == nil {
		t.Errorf("expected file not found error, got %v", err)
	}
}

func TestWriteParquetWithMerge_CorruptedExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "corrupted.parquet")

	// Write invalid/corrupted bytes to simulate corrupted parquet file
	if err := os.WriteFile(filePath, []byte("NOT_A_VALID_PARQUET_FILE_DATA"), 0644); err != nil {
		t.Fatalf("write corrupted file failed: %v", err)
	}

	newRows := []MasterFillParquet{
		{
			ID:            101,
			MasterID:      "m1",
			BrokerOrderID: "BO1",
		},
	}

	err := writeParquetWithMerge(filePath, newRows, func(r MasterFillParquet) int64 { return r.ID })
	if err == nil {
		t.Fatalf("expected error when reading corrupted parquet file, got nil")
	}

	if !strings.Contains(err.Error(), "read existing parquet file") {
		t.Errorf("expected error message to contain 'read existing parquet file', got: %v", err)
	}

	// Verify the original corrupted file was NOT overwritten with newRows
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	if string(raw) != "NOT_A_VALID_PARQUET_FILE_DATA" {
		t.Errorf("file was unexpectedly overwritten: %s", string(raw))
	}
}

func TestWriteParquetWithMerge_MergeExistingRows(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "merge.parquet")

	// 1. Initial write
	initialRows := []MasterFillParquet{
		{ID: 10, MasterID: "m1", Tradingsymbol: "NIFTY"},
		{ID: 20, MasterID: "m1", Tradingsymbol: "BANKNIFTY"},
	}
	if err := writeParquetWithMerge(filePath, initialRows, func(r MasterFillParquet) int64 { return r.ID }); err != nil {
		t.Fatalf("initial write failed: %v", err)
	}

	// 2. Incremental write with one overlapping ID (update) and one new ID
	incrementalRows := []MasterFillParquet{
		{ID: 20, MasterID: "m1", Tradingsymbol: "BANKNIFTY_UPDATED"},
		{ID: 30, MasterID: "m1", Tradingsymbol: "FINNIFTY"},
	}
	if err := writeParquetWithMerge(filePath, incrementalRows, func(r MasterFillParquet) int64 { return r.ID }); err != nil {
		t.Fatalf("incremental merge write failed: %v", err)
	}

	// 3. Read back and verify merge
	actual, err := parquet.ReadFile[MasterFillParquet](filePath)
	if err != nil {
		t.Fatalf("read merged file failed: %v", err)
	}

	if len(actual) != 3 {
		t.Fatalf("expected 3 merged rows, got %d", len(actual))
	}

	// Check IDs are sorted: 10, 20, 30
	if actual[0].ID != 10 || actual[1].ID != 20 || actual[2].ID != 30 {
		t.Errorf("rows not sorted by ID: %+v", actual)
	}
	if actual[1].Tradingsymbol != "BANKNIFTY_UPDATED" {
		t.Errorf("expected ID 20 to be updated, got: %s", actual[1].Tradingsymbol)
	}
}

func TestRestorer_InvalidDateRange(t *testing.T) {
	restorer := NewRestorer(nil, nil)
	ctx := context.Background()

	from := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	upto := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)

	_, err := restorer.Restore(ctx, "/tmp", &from, &upto)
	if err == nil {
		t.Fatalf("expected error for inverted date range, got nil")
	}

	if !strings.Contains(err.Error(), "from date (2026-10-25) cannot be after upto date (2026-10-20)") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRestorer_DiscoverDatePartitions(t *testing.T) {
	tmpDir := t.TempDir()
	restorer := NewRestorer(nil, nil)

	// Create test partition folders across tables
	_ = os.MkdirAll(filepath.Join(tmpDir, "master_fills", "date=2026-10-20"), 0755)
	_ = os.MkdirAll(filepath.Join(tmpDir, "master_fills", "date=2026-10-22"), 0755)
	_ = os.MkdirAll(filepath.Join(tmpDir, "follower_orders", "date=2026-10-21"), 0755)
	_ = os.MkdirAll(filepath.Join(tmpDir, "order_events", "date=2026-10-23"), 0755)

	// Case 1: Discover all
	allDates, err := restorer.discoverDatePartitions(tmpDir, "", "")
	if err != nil {
		t.Fatalf("discover all failed: %v", err)
	}
	expectedAll := []string{"2026-10-20", "2026-10-21", "2026-10-22", "2026-10-23"}
	if len(allDates) != len(expectedAll) {
		t.Fatalf("expected %v, got %v", expectedAll, allDates)
	}
	for i := range expectedAll {
		if allDates[i] != expectedAll[i] {
			t.Errorf("index %d: expected %s, got %s", i, expectedAll[i], allDates[i])
		}
	}

	// Case 2: Discover filtered range [2026-10-21, 2026-10-22]
	filtered, err := restorer.discoverDatePartitions(tmpDir, "2026-10-21", "2026-10-22")
	if err != nil {
		t.Fatalf("discover filtered failed: %v", err)
	}
	expectedFiltered := []string{"2026-10-21", "2026-10-22"}
	if len(filtered) != len(expectedFiltered) {
		t.Fatalf("expected %v, got %v", expectedFiltered, filtered)
	}
	for i := range expectedFiltered {
		if filtered[i] != expectedFiltered[i] {
			t.Errorf("index %d: expected %s, got %s", i, expectedFiltered[i], filtered[i])
		}
	}
}

