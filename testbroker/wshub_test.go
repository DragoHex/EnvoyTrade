package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func setupTestWSHub() (*WSHub, *httptest.Server, *Config) {
	cfg := &Config{
		Users: map[string]UserConfig{
			"USER1": {APIKey: "key1", AccessToken: "token1"},
			"USER2": {APIKey: "key2", AccessToken: "token2"},
		},
		tokenIndex: map[string]string{
			"key1:token1": "USER1",
			"key2:token2": "USER2",
		},
	}
	hub := NewWSHub(cfg)
	server := httptest.NewServer(http.HandlerFunc(hub.HandleWS))
	return hub, server, cfg
}

func connectWS(t *testing.T, serverURL string, apiKey, accessToken string) (*websocket.Conn, *http.Response, error) {
	wsURL := strings.Replace(serverURL, "http://", "ws://", 1) + "?api_key=" + apiKey + "&access_token=" + accessToken
	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, resp, err := dialer.DialContext(context.Background(), wsURL, nil)
	return conn, resp, err
}

func TestWSHub_ConnectAndReceive(t *testing.T) {
	hub, server, _ := setupTestWSHub()
	defer server.Close()

	conn, _, err := connectWS(t, server.URL, "key1", "token1")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	order := &Order{
		OrderID: "123",
		UserID:  "USER1",
	}

	// Give a little time for the connection to be registered
	time.Sleep(50 * time.Millisecond)

	hub.Broadcast(order)

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var msg wsMessage
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("Failed to read JSON: %v", err)
	}

	if msg.Type != "order" {
		t.Errorf("Expected type 'order', got %q", msg.Type)
	}

	dataMap, ok := msg.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected Data to be a map, got %T", msg.Data)
	}
	if dataMap["order_id"] != "123" {
		t.Errorf("Expected order_id '123', got %v", dataMap["order_id"])
	}
}

func TestWSHub_AuthReject(t *testing.T) {
	_, server, _ := setupTestWSHub()
	defer server.Close()

	// Missing token
	_, resp, err := connectWS(t, server.URL, "key1", "wrong")
	if err == nil {
		t.Fatal("Expected error connecting with wrong token")
	}
	if resp != nil && resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 403 or 401, got %d", resp.StatusCode)
	}
}

func TestWSHub_UserIsolation(t *testing.T) {
	hub, server, _ := setupTestWSHub()
	defer server.Close()

	conn1, _, err := connectWS(t, server.URL, "key1", "token1")
	if err != nil {
		t.Fatalf("Failed to connect user1: %v", err)
	}
	defer conn1.Close()

	conn2, _, err := connectWS(t, server.URL, "key2", "token2")
	if err != nil {
		t.Fatalf("Failed to connect user2: %v", err)
	}
	defer conn2.Close()

	time.Sleep(50 * time.Millisecond)

	// Broadcast to user1
	hub.Broadcast(&Order{OrderID: "O1", UserID: "USER1"})

	// Read from conn1
	conn1.SetReadDeadline(time.Now().Add(1 * time.Second))
	var msg wsMessage
	if err := conn1.ReadJSON(&msg); err != nil {
		t.Fatalf("User1 failed to read message: %v", err)
	}

	// Read from conn2 should timeout
	conn2.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	err = conn2.ReadJSON(&msg)
	if err == nil {
		t.Fatal("User2 received a message that was for User1")
	}
}

func TestWSHub_Disconnect(t *testing.T) {
	hub, server, _ := setupTestWSHub()
	defer server.Close()

	conn, _, err := connectWS(t, server.URL, "key1", "token1")
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	
	// Close connection
	conn.Close()
	
	time.Sleep(50 * time.Millisecond)

	// Broadcast should not panic and shouldn't hang
	hub.Broadcast(&Order{OrderID: "O1", UserID: "USER1"})
}
