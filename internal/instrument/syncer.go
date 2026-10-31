package instrument

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"envoytrade/internal/domain"

	"github.com/DragoHex/godhan"
	"github.com/shopspring/decimal"
)

// SyncerStore defines store methods needed by Syncer.
type SyncerStore interface {
	PruneInstrumentsByBroker(ctx context.Context, broker string) (int64, error)
	BatchInsertInstruments(ctx context.Context, instruments []domain.Instrument) error
	VacuumInstruments(ctx context.Context) error
}

// HTTPClient narrows the HTTP client interface for easy mocking.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Syncer manages streaming downloads and transactional replacements of instrument masters.
type Syncer struct {
	store      SyncerStore
	httpClient HTTPClient
	logger     *slog.Logger
	dhanURL    string
	kiteBaseURL string
}

// NewSyncer constructs an initialized Syncer.
func NewSyncer(store SyncerStore, logger *slog.Logger) *Syncer {
	return &Syncer{
		store:       store,
		httpClient:  &http.Client{Timeout: 90 * time.Second},
		logger:      logger,
		dhanURL:     godhan.DefaultScripMasterURL,
		kiteBaseURL: "https://api.kite.trade/instruments",
	}
}

func (s *Syncer) log() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}
	return slog.Default()
}

// SetDhanURL overrides the default Dhan scrip master CSV endpoint (useful for tests).
func (s *Syncer) SetDhanURL(url string) {
	s.dhanURL = url
}

// SetKiteBaseURL overrides the default Kite instruments base endpoint (useful for tests).
func (s *Syncer) SetKiteBaseURL(url string) {
	s.kiteBaseURL = url
}

// SetHTTPClient overrides the default HTTP client (useful for tests).
func (s *Syncer) SetHTTPClient(client HTTPClient) {
	s.httpClient = client
}

// mapDhanExchangeSegment maps exchange and segment codes to Dhan's ExchangeSegment.
func mapDhanExchangeSegment(exchange, segment string) string {
	exch := strings.ToUpper(strings.TrimSpace(exchange))
	seg := strings.ToUpper(strings.TrimSpace(segment))

	switch exch {
	case "NSE":
		switch seg {
		case "D":
			return "NSE_FNO"
		case "C":
			return "NSE_CURRENCY"
		case "M":
			return "MCX_COMM"
		default:
			return "NSE_EQ"
		}
	case "BSE":
		switch seg {
		case "D":
			return "BSE_FNO"
		case "C":
			return "BSE_CURRENCY"
		case "M":
			return "MCX_COMM"
		default:
			return "BSE_EQ"
		}
	case "MCX":
		return "MCX_COMM"
	default:
		if seg == "D" {
			return exch + "_FNO"
		}
		if seg == "M" {
			return "MCX_COMM"
		}
		return exch + "_EQ"
	}
}

// isDhanEquity reports whether a Dhan scrip record is a cash equity or index instrument.
// In Dhan's scrip master:
//   - Cash equities strictly have SEM_INSTRUMENT_NAME == "EQUITY" and SEM_SEGMENT == "E" (or "G" for EGRs).
//   - Indices have SEM_INSTRUMENT_NAME == "INDEX" and SEM_SEGMENT == "I".
//   - Derivatives strictly have SEM_INSTRUMENT_NAME starting with "OPT" or "FUT" (OPTSTK, OPTIDX, OPTFUT, OPTCUR, FUTSTK, FUTIDX, FUTCOM, FUTCUR).
func isDhanEquity(segment, instrumentName string) bool {
	seg := strings.ToUpper(strings.TrimSpace(segment))
	name := strings.ToUpper(strings.TrimSpace(instrumentName))
	return seg == "E" || seg == "G" || seg == "I" || name == "EQUITY" || name == "INDEX"
}

// SyncDhan streams Dhan scrip master CSV, parses all rows, and upon 100% success replaces Dhan records.
func (s *Syncer) SyncDhan(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.dhanURL, nil)
	if err != nil {
		return fmt.Errorf("create dhan request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch dhan scrips: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected http status %d fetching dhan scrips", resp.StatusCode)
	}

	rawInsts, err := godhan.ParseInstruments(resp.Body)
	if err != nil {
		return fmt.Errorf("parse dhan instruments: %w", err)
	}
	if len(rawInsts) == 0 {
		return errors.New("dhan returned 0 instruments, skipping replacement")
	}

	// Map to domain.Instrument, filtering out equities to match Kite Zerodha (saving only non-equity/derivatives/commodities)
	instruments := make([]domain.Instrument, 0, len(rawInsts))
	for _, inst := range rawInsts {
		if isDhanEquity(inst.Segment, inst.InstrumentName) {
			continue
		}

		exSeg := mapDhanExchangeSegment(inst.Exchange, inst.Segment)

		secIDInt, _ := strconv.ParseInt(inst.SecurityID, 10, 64)
		// Offset token to avoid collisions with Kite tokens (token >= 10^12)
		token := 1000000000000 + secIDInt
		if token <= 1000000000000 {
			token = 1000000000000 + int64(len(instruments)+1)
		}

		lot := int(inst.LotSize)
		if lot <= 0 {
			lot = 1
		}

		var exp *time.Time
		if inst.ExpiryDate != "" {
			layout := "2006-01-02 15:04:05"
			if t, parseErr := time.Parse(layout, inst.ExpiryDate); parseErr == nil {
				exp = &t
			} else if t, parseErr := time.Parse("2006-01-02", inst.ExpiryDate); parseErr == nil {
				exp = &t
			}
		}

		canonicalExchange := strings.ToUpper(strings.TrimSpace(inst.Exchange))
		segment := strings.TrimSpace(inst.Segment)
		if segment == "D" {
			switch canonicalExchange {
			case "NSE":
				canonicalExchange = "NFO"
			case "BSE":
				canonicalExchange = "BFO"
			}
		}

		instruments = append(instruments, domain.Instrument{
			InstrumentToken: token,
			Broker:          "dhan",
			Exchange:        canonicalExchange,
			Tradingsymbol:   strings.TrimSpace(inst.TradingSymbol),
			LotSize:         lot,
			Segment:         segment,
			SecurityID:      inst.SecurityID,
			ExchangeSegment: exSeg,
			Expiry:          exp,
		})
	}

	if len(instruments) == 0 {
		return errors.New("dhan returned 0 non-equity instruments, skipping replacement")
	}

	// 100% Successful parse: Prune existing dhan records
	pruned, err := s.store.PruneInstrumentsByBroker(ctx, "dhan")
	if err != nil {
		return fmt.Errorf("prune dhan instruments: %w", err)
	}
	s.log().Info("dhan instruments pruned", "pruned", pruned)

	// Batch insert in chunks of 5000
	const chunkSize = 5000
	for i := 0; i < len(instruments); i += chunkSize {
		end := i + chunkSize
		if end > len(instruments) {
			end = len(instruments)
		}
		if err := s.store.BatchInsertInstruments(ctx, instruments[i:end]); err != nil {
			return fmt.Errorf("batch insert dhan instruments chunk [%d:%d]: %w", i, end, err)
		}
	}

	// Vacuum to clean dead tuples and refresh query statistics
	if err := s.store.VacuumInstruments(ctx); err != nil {
		s.log().Warn("vacuum instruments after dhan sync failed", "error", err)
	}

	s.log().Info("dhan instruments successfully synced", "count", len(instruments))
	return nil
}

// SyncKite streams Zerodha Kite instruments for specified exchanges and replaces Kite records.
func (s *Syncer) SyncKite(ctx context.Context, exchanges []string) error {
	if len(exchanges) == 0 {
		exchanges = []string{"MCX", "NFO", "BFO"}
	}

	allInstruments := make([]domain.Instrument, 0, 50000)

	for _, exch := range exchanges {
		url := fmt.Sprintf("%s/%s", s.kiteBaseURL, exch)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create kite request for %s: %w", exch, err)
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("fetch kite instruments for %s: %w", exch, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected http status %d fetching kite %s", resp.StatusCode, exch)
		}

		reader := csv.NewReader(resp.Body)
		// Read header
		header, err := reader.Read()
		if err != nil {
			return fmt.Errorf("read kite header %s: %w", exch, err)
		}

		colMap := make(map[string]int, len(header))
		for i, h := range header {
			colMap[strings.TrimSpace(h)] = i
		}

		for {
			row, err := reader.Read()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return fmt.Errorf("read kite row %s: %w", exch, err)
			}

			tokenIdx := colMap["instrument_token"]
			symIdx := colMap["tradingsymbol"]
			lotIdx := colMap["lot_size"]
			tickIdx := colMap["tick_size"]
			segIdx := colMap["segment"]
			expIdx := colMap["expiry"]

			token, _ := strconv.ParseInt(row[tokenIdx], 10, 64)
			sym := row[symIdx]
			lot, _ := strconv.Atoi(row[lotIdx])
			if lot <= 0 {
				lot = 1
			}
			tick, _ := decimal.NewFromString(row[tickIdx])
			seg := row[segIdx]

			var exp *time.Time
			if expIdx < len(row) && row[expIdx] != "" {
				if t, parseErr := time.Parse("2006-01-02", row[expIdx]); parseErr == nil {
					exp = &t
				}
			}

			allInstruments = append(allInstruments, domain.Instrument{
				InstrumentToken: token,
				Broker:          "kite",
				Exchange:        exch,
				Tradingsymbol:   sym,
				LotSize:         lot,
				TickSize:        tick,
				Segment:         seg,
				Expiry:          exp,
			})
		}
	}

	if len(allInstruments) == 0 {
		return errors.New("kite returned 0 instruments across exchanges, skipping replacement")
	}

	// 100% Successful parse across all exchanges: Prune existing kite records
	pruned, err := s.store.PruneInstrumentsByBroker(ctx, "kite")
	if err != nil {
		return fmt.Errorf("prune kite instruments: %w", err)
	}
	s.log().Info("kite instruments pruned", "pruned", pruned)

	// Batch insert in chunks of 5000
	const chunkSize = 5000
	for i := 0; i < len(allInstruments); i += chunkSize {
		end := i + chunkSize
		if end > len(allInstruments) {
			end = len(allInstruments)
		}
		if err := s.store.BatchInsertInstruments(ctx, allInstruments[i:end]); err != nil {
			return fmt.Errorf("batch insert kite instruments chunk [%d:%d]: %w", i, end, err)
		}
	}

	// Vacuum
	if err := s.store.VacuumInstruments(ctx); err != nil {
		s.log().Warn("vacuum instruments after kite sync failed", "error", err)
	}

	s.log().Info("kite instruments successfully synced", "count", len(allInstruments))
	return nil
}

// NextRunDuration calculates the duration until the next target hour:minute in IST (Asia/Kolkata).
func NextRunDuration(now time.Time, targetHour, targetMinute int) time.Duration {
	istLocation, locErr := time.LoadLocation("Asia/Kolkata")
	if locErr != nil {
		istLocation = time.FixedZone("IST", 5*3600+1800)
	}

	nowIST := now.In(istLocation)
	nextRun := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), targetHour, targetMinute, 0, 0, istLocation)
	if !nowIST.Before(nextRun) {
		nextRun = nextRun.Add(24 * time.Hour)
	}
	return nextRun.Sub(nowIST)
}

// StartDailyRoutine starts an in-process daily ticker loop targeting targetHour:targetMinute in IST.
// Calls onComplete() after each successful sync cycle.
func (s *Syncer) StartDailyRoutine(ctx context.Context, targetHour, targetMinute int, onComplete func(context.Context) error) {
	go func() {
		for {
			waitDur := NextRunDuration(time.Now(), targetHour, targetMinute)
			s.log().Info("instrument syncer: scheduled next run",
				"target_ist", fmt.Sprintf("%02d:%02d IST", targetHour, targetMinute),
				"wait_hours", fmt.Sprintf("%.2f", waitDur.Hours()),
			)

			timer := time.NewTimer(waitDur)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				s.log().Info("instrument syncer: beginning daily refresh")
				
				// Run Dhan sync
				if err := s.SyncDhan(ctx); err != nil {
					s.log().Error("daily sync dhan failed", "error", err)
				}
				
				// Run Kite sync
				if err := s.SyncKite(ctx, nil); err != nil {
					s.log().Error("daily sync kite failed", "error", err)
				}

				// Trigger cache refresh
				if onComplete != nil {
					if err := onComplete(ctx); err != nil {
						s.log().Error("instrument syncer: onComplete callback failed", "error", err)
					}
				}
			}
		}
	}()
}
