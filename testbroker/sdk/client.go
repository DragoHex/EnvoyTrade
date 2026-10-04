package sdk

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURI = "http://localhost:8089"

// Client is a Kite-compatible REST API client for the mock broker.
type Client struct {
	apiKey      string
	accessToken string
	baseURI     string
	httpClient  *http.Client
}

// New creates a new Client instance with the given API key.
func New(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		baseURI:    defaultBaseURI,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// SetAccessToken sets the user access token for authenticated requests.
func (c *Client) SetAccessToken(token string) {
	c.accessToken = token
}

// SetBaseURI overrides the default REST server base URI.
func (c *Client) SetBaseURI(baseURI string) {
	c.baseURI = strings.TrimRight(baseURI, "/")
}

// SetHTTPClient sets a custom HTTP client (e.g. for proxy egress).
func (c *Client) SetHTTPClient(h *http.Client) {
	if h != nil {
		c.httpClient = h
	}
}

// PlaceOrder places a new order with the mock broker.
func (c *Client) PlaceOrder(variety string, params OrderParams) (OrderResponse, error) {
	if variety == "" {
		variety = VarietyRegular
	}

	form := url.Values{}
	form.Set("exchange", params.Exchange)
	form.Set("tradingsymbol", params.Tradingsymbol)
	form.Set("transaction_type", params.TransactionType)
	form.Set("product", params.Product)
	form.Set("order_type", params.OrderType)
	form.Set("quantity", strconv.Itoa(params.Quantity))
	if params.Price > 0 {
		form.Set("price", strconv.FormatFloat(params.Price, 'f', -1, 64))
	}
	if params.TriggerPrice > 0 {
		form.Set("trigger_price", strconv.FormatFloat(params.TriggerPrice, 'f', -1, 64))
	}
	if params.Tag != "" {
		form.Set("tag", params.Tag)
	}
	if params.Validity != "" {
		form.Set("validity", params.Validity)
	}

	var resp OrderResponse
	path := fmt.Sprintf("/orders/%s", variety)
	err := c.doRequest(http.MethodPost, path, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", &resp)
	return resp, err
}

// CancelOrder cancels an open order.
func (c *Client) CancelOrder(variety, orderID string, parentOrderID *string) (OrderResponse, error) {
	if variety == "" {
		variety = VarietyRegular
	}
	var resp OrderResponse
	path := fmt.Sprintf("/orders/%s/%s", variety, orderID)
	err := c.doRequest(http.MethodDelete, path, nil, "", &resp)
	return resp, err
}

// GetOrders retrieves all orders for the authenticated user.
func (c *Client) GetOrders() ([]Order, error) {
	var orders []Order
	err := c.doRequest(http.MethodGet, "/orders", nil, "", &orders)
	return orders, err
}

// GetOrderHistory retrieves state transition history for an order.
func (c *Client) GetOrderHistory(orderID string) ([]Order, error) {
	var history []Order
	path := fmt.Sprintf("/orders/%s", orderID)
	err := c.doRequest(http.MethodGet, path, nil, "", &history)
	return history, err
}

// GetPositions retrieves net and day positions.
func (c *Client) GetPositions() (Positions, error) {
	var pos Positions
	err := c.doRequest(http.MethodGet, "/portfolio/positions", nil, "", &pos)
	return pos, err
}

// GetHoldings retrieves user equity holdings.
func (c *Client) GetHoldings() ([]Holding, error) {
	var holdings []Holding
	err := c.doRequest(http.MethodGet, "/portfolio/holdings", nil, "", &holdings)
	return holdings, err
}

// GetUserMargins retrieves equity and commodity user margins.
func (c *Client) GetUserMargins() (AllMargins, error) {
	var margins AllMargins
	err := c.doRequest(http.MethodGet, "/user/margins", nil, "", &margins)
	return margins, err
}

// GetLTP fetches Last Traded Price for requested symbols (e.g. "NFO:NIFTY26OCTFUT").
func (c *Client) GetLTP(instruments ...string) (map[string]QuoteLTP, error) {
	query := url.Values{}
	for _, inst := range instruments {
		query.Add("i", inst)
	}
	path := "/quote/ltp?" + query.Encode()

	quotes := make(map[string]QuoteLTP)
	err := c.doRequest(http.MethodGet, path, nil, "", &quotes)
	return quotes, err
}

// GenerateSession performs the request_token to access_token exchange.
// It automatically updates the client's internal access token upon success.
func (c *Client) GenerateSession(requestToken, apiSecret string) (UserSession, error) {
	h := sha256.Sum256([]byte(c.apiKey + requestToken + apiSecret))
	checksum := hex.EncodeToString(h[:])

	form := url.Values{}
	form.Set("api_key", c.apiKey)
	form.Set("request_token", requestToken)
	form.Set("checksum", checksum)

	var session UserSession
	err := c.doRequest(http.MethodPost, "/session/token", strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", &session)
	if err == nil && session.AccessToken != "" {
		c.accessToken = session.AccessToken
	}
	return session, err
}

func (c *Client) doRequest(method, path string, body io.Reader, contentType string, target any) error {
	fullURL := c.baseURI + path
	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.apiKey != "" && c.accessToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("token %s:%s", c.apiKey, c.accessToken))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	// Envelope parsing
	if resp.StatusCode >= http.StatusBadRequest {
		var errEnv struct {
			Status    string `json:"status"`
			ErrorType string `json:"error_type"`
			Message   string `json:"message"`
		}
		if err := json.Unmarshal(respBytes, &errEnv); err == nil && errEnv.Status == "error" {
			return &KiteError{
				StatusCode: resp.StatusCode,
				Status:     errEnv.Status,
				ErrorType:  errEnv.ErrorType,
				Message:    errEnv.Message,
			}
		}
		return &KiteError{
			StatusCode: resp.StatusCode,
			Status:     "error",
			ErrorType:  "GeneralException",
			Message:    string(respBytes),
		}
	}

	if target != nil {
		var env struct {
			Status string          `json:"status"`
			Data   json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(respBytes, &env); err != nil {
			return fmt.Errorf("unmarshal envelope: %w", err)
		}
		if err := json.Unmarshal(env.Data, target); err != nil {
			return fmt.Errorf("unmarshal data: %w", err)
		}
	}

	return nil
}
