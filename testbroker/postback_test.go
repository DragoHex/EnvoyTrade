package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestComputeChecksum(t *testing.T) {
	orderID := "260900000000001"
	timestamp := "2026-10-03 10:30:00"
	secret := "secret_master"
	expected := "4f374592a17569a94c173bed136027a05e89a8cd4cea77ae86932e0a52acf0d3"

	got := computeChecksum(orderID, timestamp, secret)
	if got != expected {
		t.Errorf("expected checksum %q, got %q", expected, got)
	}
}

func TestBuildPostbackPayload(t *testing.T) {
	o := Order{
		OrderID:        "123",
		UserID:         "U1",
		OrderTimestamp: "2026-10-03 10:30:00",
		Status:         "COMPLETE",
	}
	// compute manually expected
	expChecksum := computeChecksum("123", "2026-10-03 10:30:00", "secret123")

	payloadOrder := o
	payloadOrder.Checksum = expChecksum

	data, err := json.Marshal(payloadOrder)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if parsed["order_id"] != "123" {
		t.Errorf("expected order_id 123, got %v", parsed["order_id"])
	}
	if parsed["user_id"] != "U1" {
		t.Errorf("expected user_id U1, got %v", parsed["user_id"])
	}
	if parsed["checksum"] != expChecksum {
		t.Errorf("expected checksum %s, got %v", expChecksum, parsed["checksum"])
	}
}

func TestPostbackDispatcher_Sends(t *testing.T) {
	received := make(chan []byte, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	users := map[string]UserConfig{
		"U1": {APISecret: "sec"},
	}
	d := NewPostbackDispatcher(srv.URL, users)

	o := &Order{
		OrderID:        "O1",
		UserID:         "U1",
		Status:         "COMPLETE",
		OrderTimestamp: "T1",
	}

	d.Dispatch(o)

	select {
	case body := <-received:
		var parsed map[string]interface{}
		json.Unmarshal(body, &parsed)
		if parsed["order_id"] != "O1" {
			t.Errorf("unexpected order_id: %v", parsed["order_id"])
		}
		expChecksum := computeChecksum("O1", "T1", "sec")
		if parsed["checksum"] != expChecksum {
			t.Errorf("unexpected checksum: %v", parsed["checksum"])
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for postback")
	}
}

func TestPostbackDispatcher_NoSendForNonTerminal(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	users := map[string]UserConfig{
		"U1": {APISecret: "sec"},
	}
	d := NewPostbackDispatcher(srv.URL, users)

	o := &Order{
		OrderID: "O1",
		UserID:  "U1",
		Status:  "OPEN",
	}

	d.Dispatch(o)

	time.Sleep(100 * time.Millisecond) // wait to ensure no goroutine fired
	if called {
		t.Error("expected no postback for OPEN order")
	}
}
