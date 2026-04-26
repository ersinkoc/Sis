package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCLIClientRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, maxCLIResponseBytes+1))
	}))
	defer server.Close()

	client := newCLIClient(server.URL, "")
	if err := client.get("/", io.Discard); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v", err)
	}
}

func TestCLIClientStreamsEventStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		for i := 0; i < 3; i++ {
			_, _ = w.Write([]byte("data: ok\n\n"))
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	client := newCLIClient(server.URL, "")
	if err := client.get("/", &out); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(out.String(), "data: ok"); got != 3 {
		t.Fatalf("events = %d body=%q", got, out.String())
	}
}

func TestContentTypeHelpersAcceptParameters(t *testing.T) {
	if !isJSONContent("application/json; charset=utf-8") {
		t.Fatal("expected JSON content type with parameters")
	}
	if !isEventStream("text/event-stream; charset=utf-8") {
		t.Fatal("expected event-stream content type with parameters")
	}
	if isJSONContent("application/json-seq") {
		t.Fatal("unexpected JSON match for different media type")
	}
	if isEventStream("text/event-streaming") {
		t.Fatal("unexpected event-stream match for different media type")
	}
}
