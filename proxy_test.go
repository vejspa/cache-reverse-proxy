package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestXForwardedForIsRemoved(t *testing.T) {
	var receivedHeaders http.Header
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))
	defer backend.Close()

	proxy := newReverseProxy(backend.URL)
	proxyServer := httptest.NewServer(proxy)
	defer proxyServer.Close()

	req, err := http.NewRequest("GET", proxyServer.URL+"/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("X-Forwarded-For", "1.2.3.4")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			t.Errorf("Could not close body")
		}
	}(resp.Body)
	_, _ = io.ReadAll(resp.Body)
	serverAddress := backend.Listener.Addr().String()
	host, _, err := net.SplitHostPort(serverAddress)

	if got := receivedHeaders.Get("X-Forwarded-For"); got != host {
		t.Errorf("expected X-Forwarded-For to be removed, but got: %q, expected %q", got, host)
	}
}
