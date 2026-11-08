//go:build windows

package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type geoRoundTripFunc func(*http.Request) (*http.Response, error)

func (f geoRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestLookupIPAPIBatchRetriesTimeoutOnce(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: geoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return nil, context.DeadlineExceeded
		}
		return geoResponse(http.StatusOK, "[{\"status\":\"success\",\"query\":\"198.51.100.1\",\"country\":\"China\"}]"), nil
	})}
	got, err := lookupIPAPIBatchWithClient(context.Background(), []string{"198.51.100.1"}, client, "http://geo.test/batch")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || got["198.51.100.1"].Country != "China" {
		t.Fatalf("calls = %d, results = %#v", calls, got)
	}
}

func TestLookupIPAPIBatchDoesNotRetryRateLimit(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: geoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return geoResponse(http.StatusTooManyRequests, ""), nil
	})}
	_, err := lookupIPAPIBatchWithClient(context.Background(), []string{"198.51.100.1"}, client, "http://geo.test/batch")
	if calls != 1 || err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("calls = %d, error = %v", calls, err)
	}
}

func TestLookupIPAPIBatchKeepsEarlierBatchOnFailure(t *testing.T) {
	ips := make([]string, 101)
	for i := range ips {
		ips[i] = fmt.Sprintf("198.51.100.%d", i)
	}
	calls := 0
	client := &http.Client{Transport: geoRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 2 {
			return geoResponse(http.StatusTooManyRequests, ""), nil
		}
		var batch []string
		if err := json.NewDecoder(req.Body).Decode(&batch); err != nil {
			t.Fatal(err)
		}
		if len(batch) != 100 {
			t.Fatalf("first batch has %d IPs", len(batch))
		}
		entries := make([]ipAPIEntry, len(batch))
		for i, ip := range batch {
			entries[i] = ipAPIEntry{Status: "success", Query: ip}
		}
		body, err := json.Marshal(entries)
		if err != nil {
			t.Fatal(err)
		}
		return geoResponse(http.StatusOK, string(body)), nil
	})}
	got, err := lookupIPAPIBatchWithClient(context.Background(), ips, client, "http://geo.test/batch")
	if err == nil || calls != 2 || len(got) != 100 {
		t.Fatalf("calls = %d, results = %d, error = %v", calls, len(got), err)
	}
}

func geoResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
