package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// NewRouter constructs the full HTTP handler with Kite endpoints, Admin API, and WebSocket routing.
func NewRouter(cfg *Config, eng *OrderEngine, hub *WSHub, postback *PostbackDispatcher) http.Handler {
	mux := http.NewServeMux()

	// -------------------------------------------------------------
	// Kite Connect Endpoints
	// -------------------------------------------------------------

	// Place order
	mux.HandleFunc("POST /orders/{variety}", func(w http.ResponseWriter, r *http.Request) {
		variety := r.PathValue("variety")
		if variety == "" {
			variety = "regular"
		}
		userID := UserIDFromContext(r.Context())
		if userID == "" {
			writeKiteError(w, http.StatusForbidden, "TokenException", "User not found in context")
			return
		}

		params, err := parseOrderParams(r)
		if err != nil {
			writeKiteError(w, http.StatusBadRequest, "InputException", err.Error())
			return
		}

		resp, err := eng.PlaceOrder(userID, variety, params)
		if err != nil {
			errType := "InputException"
			if strings.HasPrefix(err.Error(), "OrderException") {
				errType = "OrderException"
			}
			writeKiteError(w, http.StatusBadRequest, errType, err.Error())
			return
		}

		// Handle delayed mode
		if cfg.ExecutionMode == "delayed" {
			go func(orderID string, exchange, symbol string) {
				time.Sleep(300 * time.Millisecond)
				inst, ok := eng.GetInstrument(exchange, symbol)
				price := 0.0
				if ok {
					price = inst.LTP
				}
				_ = eng.FillOrder(orderID, price)
			}(resp.OrderID, params.Exchange, params.Tradingsymbol)
		}

		writeKiteSuccess(w, resp)
	})

	// Get orders
	mux.HandleFunc("GET /orders", func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		orders := eng.GetOrders(userID)
		if orders == nil {
			orders = []*Order{}
		}
		writeKiteSuccess(w, orders)
	})

	// Get order history
	mux.HandleFunc("GET /orders/{order_id}", func(w http.ResponseWriter, r *http.Request) {
		orderID := r.PathValue("order_id")
		history := eng.GetOrderHistory(orderID)
		if history == nil {
			history = []Order{}
		}
		writeKiteSuccess(w, history)
	})

	// Cancel order
	mux.HandleFunc("DELETE /orders/{variety}/{order_id}", func(w http.ResponseWriter, r *http.Request) {
		orderID := r.PathValue("order_id")
		if err := eng.CancelOrder(orderID); err != nil {
			writeKiteError(w, http.StatusBadRequest, "OrderException", err.Error())
			return
		}
		writeKiteSuccess(w, OrderResponse{OrderID: orderID})
	})

	// Positions (net & day)
	mux.HandleFunc("GET /portfolio/positions", func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		if userID == "" {
			writeKiteError(w, http.StatusForbidden, "TokenException", "User not found in context")
			return
		}
		net, day := eng.GetPositions(userID)
		writeKiteSuccess(w, map[string]any{
			"net": net,
			"day": day,
		})
	})

	// Holdings
	mux.HandleFunc("GET /portfolio/holdings", func(w http.ResponseWriter, r *http.Request) {
		userID := UserIDFromContext(r.Context())
		if userID == "" {
			writeKiteError(w, http.StatusForbidden, "TokenException", "User not found in context")
			return
		}
		if u, ok := cfg.Users[userID]; ok && len(u.Holdings) > 0 {
			writeKiteSuccess(w, u.Holdings)
			return
		}
		writeKiteSuccess(w, []any{})
	})

	// Margins
	mux.HandleFunc("GET /user/margins", func(w http.ResponseWriter, r *http.Request) {
		writeKiteSuccess(w, map[string]any{
			"equity": map[string]any{
				"enabled": true,
				"net":     10000000.0,
				"available": map[string]any{
					"cash":         10000000.0,
					"live_balance": 10000000.0,
				},
				"utilised": map[string]any{},
			},
			"commodity": map[string]any{
				"enabled": true,
				"net":     10000000.0,
				"available": map[string]any{
					"cash":         10000000.0,
					"live_balance": 10000000.0,
				},
				"utilised": map[string]any{},
			},
		})
	})

	// Session token exchange
	mux.HandleFunc("POST /session/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		apiKey := r.FormValue("api_key")
		if apiKey == "" {
			var body struct {
				APIKey string `json:"api_key"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			apiKey = body.APIKey
		}

		var matchedUser string
		var matchedToken string
		for uid, u := range cfg.Users {
			if u.APIKey == apiKey || apiKey == "" {
				matchedUser = uid
				matchedToken = u.AccessToken
				break
			}
		}
		if matchedUser == "" {
			matchedUser = "MOCK_USER"
			matchedToken = "mock_access_token"
		}

		writeKiteSuccess(w, map[string]any{
			"user_id":      matchedUser,
			"access_token": matchedToken,
		})
	})

	// Quote LTP
	mux.HandleFunc("GET /quote/ltp", func(w http.ResponseWriter, r *http.Request) {
		instruments := r.URL.Query()["i"]
		res := make(map[string]any)
		for _, fullSymbol := range instruments {
			parts := strings.SplitN(fullSymbol, ":", 2)
			if len(parts) == 2 {
				inst, ok := eng.GetInstrument(parts[0], parts[1])
				if ok {
					res[fullSymbol] = map[string]any{
						"instrument_token": inst.InstrumentToken,
						"last_price":       inst.LTP,
					}
				}
			}
		}
		writeKiteSuccess(w, res)
	})

	// Instruments (CSV export)
	mux.HandleFunc("GET /instruments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "instrument_token,exchange_token,tradingsymbol,name,last_price,expiry,strike,tick_size,lot_size,instrument_type,segment,exchange")
		for _, inst := range eng.GetAllInstruments() {
			fmt.Fprintf(w, "%d,0,%s,%s,%.2f,,0,%.2f,%d,FUT,%s,%s\n",
				inst.InstrumentToken, inst.Tradingsymbol, inst.Tradingsymbol,
				inst.LTP, inst.TickSize, inst.LotSize, inst.Exchange, inst.Exchange)
		}
	})

	// -------------------------------------------------------------
	// Admin API Endpoints (for embedded UI & test automation)
	// -------------------------------------------------------------

	// List users
	mux.HandleFunc("GET /api/users", func(w http.ResponseWriter, r *http.Request) {
		type userDTO struct {
			UserID      string `json:"user_id"`
			Role        string `json:"role"`
			APIKey      string `json:"api_key"`
			AccessToken string `json:"access_token"`
		}
		users := make([]userDTO, 0, len(cfg.Users))
		for uid, u := range cfg.Users {
			users = append(users, userDTO{
				UserID:      uid,
				Role:        u.Role,
				APIKey:      u.APIKey,
				AccessToken: u.AccessToken,
			})
		}
		writeKiteSuccess(w, users)
	})

	// List all orders (optionally filtered by ?user_id=...)
	mux.HandleFunc("GET /api/orders", func(w http.ResponseWriter, r *http.Request) {
		uid := r.URL.Query().Get("user_id")
		var orders []*Order
		if uid != "" {
			orders = eng.GetOrders(uid)
		} else {
			orders = eng.GetAllOrders()
		}
		if orders == nil {
			orders = []*Order{}
		}
		writeKiteSuccess(w, orders)
	})

	// Manual fill
	mux.HandleFunc("POST /api/orders/{order_id}/fill", func(w http.ResponseWriter, r *http.Request) {
		orderID := r.PathValue("order_id")
		var body struct {
			Price float64 `json:"price"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Price == 0 {
			if o, ok := eng.GetOrder(orderID); ok {
				if inst, found := eng.GetInstrument(o.Exchange, o.Tradingsymbol); found {
					body.Price = inst.LTP
				}
			}
		}
		if err := eng.FillOrder(orderID, body.Price); err != nil {
			writeKiteError(w, http.StatusBadRequest, "OrderException", err.Error())
			return
		}
		writeKiteSuccess(w, map[string]string{"message": "order filled"})
	})

	// Manual reject
	mux.HandleFunc("POST /api/orders/{order_id}/reject", func(w http.ResponseWriter, r *http.Request) {
		orderID := r.PathValue("order_id")
		var body struct {
			Reason string `json:"reason"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Reason == "" {
			body.Reason = "Manually rejected by tester"
		}
		if err := eng.RejectOrder(orderID, body.Reason); err != nil {
			writeKiteError(w, http.StatusBadRequest, "OrderException", err.Error())
			return
		}
		writeKiteSuccess(w, map[string]string{"message": "order rejected"})
	})

	// Manual cancel
	mux.HandleFunc("POST /api/orders/{order_id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		orderID := r.PathValue("order_id")
		if err := eng.CancelOrder(orderID); err != nil {
			writeKiteError(w, http.StatusBadRequest, "OrderException", err.Error())
			return
		}
		writeKiteSuccess(w, map[string]string{"message": "order cancelled"})
	})

	// Get all instruments (JSON)
	mux.HandleFunc("GET /api/instruments", func(w http.ResponseWriter, r *http.Request) {
		writeKiteSuccess(w, eng.GetAllInstruments())
	})

	// Update LTP
	updateLTPHandler := func(w http.ResponseWriter, r *http.Request) {
		exchange := r.PathValue("exchange")
		symbol := r.PathValue("symbol")
		var body struct {
			LTP float64 `json:"ltp"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeKiteError(w, http.StatusBadRequest, "InputException", "invalid json")
			return
		}
		if err := eng.UpdateLTP(exchange, symbol, body.LTP); err != nil {
			writeKiteError(w, http.StatusNotFound, "InputException", err.Error())
			return
		}
		writeKiteSuccess(w, map[string]any{"exchange": exchange, "tradingsymbol": symbol, "ltp": body.LTP})
	}
	mux.HandleFunc("POST /api/instruments/{exchange}/{symbol}/ltp", updateLTPHandler)
	mux.HandleFunc("PUT /api/instruments/{exchange}/{symbol}", updateLTPHandler)

	// Quick order placement from UI
	mux.HandleFunc("POST /api/quick-order", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			UserID          string  `json:"user_id"`
			Variety         string  `json:"variety"`
			Exchange        string  `json:"exchange"`
			Tradingsymbol   string  `json:"tradingsymbol"`
			TransactionType string  `json:"transaction_type"`
			Product         string  `json:"product"`
			OrderType       string  `json:"order_type"`
			Quantity        int     `json:"quantity"`
			Price           float64 `json:"price"`
			TriggerPrice    float64 `json:"trigger_price"`
			Tag             string  `json:"tag"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeKiteError(w, http.StatusBadRequest, "InputException", err.Error())
			return
		}
		if req.UserID == "" {
			writeKiteError(w, http.StatusBadRequest, "InputException", "user_id is required")
			return
		}
		if req.Variety == "" {
			req.Variety = "regular"
		}

		resp, err := eng.PlaceOrder(req.UserID, req.Variety, OrderParams{
			Exchange:        req.Exchange,
			Tradingsymbol:   req.Tradingsymbol,
			TransactionType: req.TransactionType,
			Product:         req.Product,
			OrderType:       req.OrderType,
			Quantity:        req.Quantity,
			Price:           req.Price,
			TriggerPrice:    req.TriggerPrice,
			Tag:             req.Tag,
		})
		if err != nil {
			writeKiteError(w, http.StatusBadRequest, "OrderException", err.Error())
			return
		}
		writeKiteSuccess(w, resp)
	})

	// Execution mode get/set
	mux.HandleFunc("GET /api/mode", func(w http.ResponseWriter, r *http.Request) {
		writeKiteSuccess(w, map[string]string{"mode": cfg.ExecutionMode})
	})
	mux.HandleFunc("POST /api/mode", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Mode string `json:"mode"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Mode != "" {
			cfg.ExecutionMode = body.Mode
		}
		writeKiteSuccess(w, map[string]string{"mode": cfg.ExecutionMode})
	})

	// -------------------------------------------------------------
	// Root / WebSocket / UI routing
	// -------------------------------------------------------------

	// Embedded UI endpoints
	serveUI := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(embeddedIndexHTML)
	}
	mux.HandleFunc("GET /ui", serveUI)
	mux.HandleFunc("GET /ui/", serveUI)

	// Root path handler
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// WebSocket upgrade check
		if strings.ToLower(r.Header.Get("Upgrade")) == "websocket" {
			hub.HandleWS(w, r)
			return
		}
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/ui", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	// Wrap with Kite auth middleware
	return AuthMiddleware(cfg)(mux)
}

func parseOrderParams(r *http.Request) (OrderParams, error) {
	var p OrderParams

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return p, err
		}
		if err := json.Unmarshal(body, &p); err != nil {
			return p, err
		}
		return p, nil
	}

	// Form urlencoded or multipart
	if err := r.ParseForm(); err != nil {
		return p, err
	}
	p.Exchange = r.FormValue("exchange")
	p.Tradingsymbol = r.FormValue("tradingsymbol")
	p.TransactionType = r.FormValue("transaction_type")
	p.Product = r.FormValue("product")
	p.OrderType = r.FormValue("order_type")
	p.Tag = r.FormValue("tag")

	if q := r.FormValue("quantity"); q != "" {
		qty, _ := strconv.Atoi(q)
		p.Quantity = qty
	}
	if pr := r.FormValue("price"); pr != "" {
		price, _ := strconv.ParseFloat(pr, 64)
		p.Price = price
	}
	if tp := r.FormValue("trigger_price"); tp != "" {
		triggerPrice, _ := strconv.ParseFloat(tp, 64)
		p.TriggerPrice = triggerPrice
	}

	return p, nil
}

func writeKiteSuccess(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "success",
		"data":   data,
	})
}

func writeKiteError(w http.ResponseWriter, statusCode int, errorType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "error",
		"error_type": errorType,
		"message":    message,
	})
}
