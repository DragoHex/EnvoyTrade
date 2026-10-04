package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// UserInfo represents an account registered in the mock broker.
type UserInfo struct {
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	APIKey      string `json:"api_key"`
	AccessToken string `json:"access_token"`
}

// SetExecutionMode updates the server-wide execution mode ("instant", "manual", "delayed").
func (c *Client) SetExecutionMode(mode string) error {
	payload, _ := json.Marshal(map[string]string{"mode": mode})
	return c.doRequest(http.MethodPost, "/api/mode", bytes.NewReader(payload), "application/json", nil)
}

// GetExecutionMode retrieves the current execution mode from the server.
func (c *Client) GetExecutionMode() (string, error) {
	var resp struct {
		Mode string `json:"mode"`
	}
	err := c.doRequest(http.MethodGet, "/api/mode", nil, "", &resp)
	return resp.Mode, err
}

// SetLTP updates the Last Traded Price for an instrument at runtime.
func (c *Client) SetLTP(exchange, symbol string, ltp float64) error {
	payload, _ := json.Marshal(map[string]float64{"ltp": ltp})
	path := fmt.Sprintf("/api/instruments/%s/%s/ltp", exchange, symbol)
	return c.doRequest(http.MethodPost, path, bytes.NewReader(payload), "application/json", nil)
}

// ManualFill transitions an OPEN order to COMPLETE at the given price.
func (c *Client) ManualFill(orderID string, price float64) error {
	payload, _ := json.Marshal(map[string]float64{"price": price})
	path := fmt.Sprintf("/api/orders/%s/fill", orderID)
	return c.doRequest(http.MethodPost, path, bytes.NewReader(payload), "application/json", nil)
}

// ManualReject transitions an OPEN order to REJECTED with the given reason.
func (c *Client) ManualReject(orderID, reason string) error {
	payload, _ := json.Marshal(map[string]string{"reason": reason})
	path := fmt.Sprintf("/api/orders/%s/reject", orderID)
	return c.doRequest(http.MethodPost, path, bytes.NewReader(payload), "application/json", nil)
}

// ManualCancel transitions an OPEN order to CANCELLED.
func (c *Client) ManualCancel(orderID string) error {
	path := fmt.Sprintf("/api/orders/%s/cancel", orderID)
	return c.doRequest(http.MethodPost, path, nil, "", nil)
}

// GetAllOrders fetches all orders across all user accounts from the admin API.
func (c *Client) GetAllOrders() ([]Order, error) {
	var orders []Order
	err := c.doRequest(http.MethodGet, "/api/orders", nil, "", &orders)
	return orders, err
}

// GetUsers returns the list of all registered test users and their credentials.
func (c *Client) GetUsers() ([]UserInfo, error) {
	var users []UserInfo
	err := c.doRequest(http.MethodGet, "/api/users", nil, "", &users)
	return users, err
}

// PlaceQuickOrder bypasses normal token auth and places an order directly on behalf of any user.
func (c *Client) PlaceQuickOrder(userID string, params OrderParams) (OrderResponse, error) {
	reqMap := map[string]any{
		"user_id":          userID,
		"variety":          VarietyRegular,
		"exchange":         params.Exchange,
		"tradingsymbol":    params.Tradingsymbol,
		"transaction_type": params.TransactionType,
		"product":          params.Product,
		"order_type":       params.OrderType,
		"quantity":         params.Quantity,
		"price":            params.Price,
		"trigger_price":    params.TriggerPrice,
		"tag":              params.Tag,
	}
	payload, _ := json.Marshal(reqMap)

	var resp OrderResponse
	err := c.doRequest(http.MethodPost, "/api/quick-order", bytes.NewReader(payload), "application/json", &resp)
	return resp, err
}
