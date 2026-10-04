package main

import (
	"crypto/sha256"
	"encoding/hex"
	"bytes"
	"encoding/json"
	"log"
	"net/http"
)

// computeChecksum returns hex(sha256(orderID + orderTimestamp + apiSecret))
func computeChecksum(orderID, orderTimestamp, apiSecret string) string {
	h := sha256.New()
	h.Write([]byte(orderID + orderTimestamp + apiSecret))
	return hex.EncodeToString(h.Sum(nil))
}

func IsTerminal(status string) bool {
	switch status {
	case "COMPLETE", "REJECTED", "CANCELLED":
		return true
	}
	return false
}

type PostbackDispatcher struct {
	url    string
	users  map[string]UserConfig
	client *http.Client
	logger *log.Logger
}

func NewPostbackDispatcher(postbackURL string, users map[string]UserConfig) *PostbackDispatcher {
	return &PostbackDispatcher{
		url:    postbackURL,
		users:  users,
		client: http.DefaultClient,
		logger: log.Default(),
	}
}

func (d *PostbackDispatcher) Dispatch(o *Order) {
	if !IsTerminal(o.Status) {
		return
	}

	user, ok := d.users[o.UserID]
	if !ok {
		if d.logger != nil {
			d.logger.Printf("PostbackDispatcher: unknown user %s for order %s", o.UserID, o.OrderID)
		}
		return
	}

	go func(ord *Order, apiSecret string) {
		// Create a copy of the order to mutate
		payloadOrder := *ord
		payloadOrder.Checksum = computeChecksum(payloadOrder.OrderID, payloadOrder.OrderTimestamp, apiSecret)

		payloadData, err := json.Marshal(payloadOrder)
		if err != nil {
			if d.logger != nil {
				d.logger.Printf("PostbackDispatcher: failed to marshal order %s: %v", ord.OrderID, err)
			}
			return
		}

		req, err := http.NewRequest(http.MethodPost, d.url, bytes.NewReader(payloadData))
		if err != nil {
			if d.logger != nil {
				d.logger.Printf("PostbackDispatcher: failed to create request for order %s: %v", ord.OrderID, err)
			}
			return
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := d.client.Do(req)
		if err != nil {
			if d.logger != nil {
				d.logger.Printf("PostbackDispatcher: failed to send postback for order %s: %v", ord.OrderID, err)
			}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			if d.logger != nil {
				d.logger.Printf("PostbackDispatcher: postback for order %s returned status %d", ord.OrderID, resp.StatusCode)
			}
		}
	}(o, user.APISecret)
}
