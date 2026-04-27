package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestCLIClientWritesJSONMethodsAndCookie(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path+" "+r.Header.Get("Cookie"))
		if r.Method != http.MethodDelete {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if body["domain"] != "example.com" {
				t.Fatalf("body = %#v", body)
			}
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("content type = %q", got)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	client := newCLIClient(server.URL, "sis_session=abc")
	for _, call := range []struct {
		name string
		run  func(io.Writer) error
	}{
		{name: "post", run: func(out io.Writer) error {
			return client.post("/items", map[string]string{"domain": "example.com"}, out)
		}},
		{name: "patch", run: func(out io.Writer) error {
			return client.patch("/items/1", map[string]string{"domain": "example.com"}, out)
		}},
		{name: "delete", run: func(out io.Writer) error { return client.delete("/items/1", out) }},
	} {
		var out bytes.Buffer
		if err := call.run(&out); err != nil {
			t.Fatalf("%s err = %v", call.name, err)
		}
		if out.String() != `{"ok":true}` {
			t.Fatalf("%s out = %q", call.name, out.String())
		}
	}

	want := []string{
		"POST /items sis_session=abc",
		"PATCH /items/1 sis_session=abc",
		"DELETE /items/1 sis_session=abc",
	}
	if strings.Join(seen, "\n") != strings.Join(want, "\n") {
		t.Fatalf("seen = %#v", seen)
	}
}

func TestCLIClientReportsHTTPErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	defer server.Close()

	client := newCLIClient(server.URL, "")
	err := client.get("/", io.Discard)
	if err == nil || !strings.Contains(err.Error(), "HTTP 418: nope") {
		t.Fatalf("err = %v", err)
	}
}

func TestCLIClientDefaultBaseAndBadRequestURL(t *testing.T) {
	client := newCLIClient("", "")
	if client.base != "http://127.0.0.1:8080" {
		t.Fatalf("base = %q", client.base)
	}
	client.base = "://bad-url"
	if err := client.get("/", io.Discard); err == nil {
		t.Fatal("expected bad URL error")
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

func TestCLIHelpers(t *testing.T) {
	if shouldPrettyPrint(io.Discard, "application/json", []byte(`{"ok":true}`)) {
		t.Fatal("non-stdout writer should not pretty print")
	}
	if responseBuffer() == nil {
		t.Fatal("responseBuffer returned nil")
	}
	if err := printResponse(nil); err != nil {
		t.Fatalf("printResponse nil err = %v", err)
	}
	if err := printResponse(io.ErrUnexpectedEOF); err == nil {
		t.Fatal("printResponse should return input error")
	}
	if got := encodedPathPart("a/b c"); got != "a%2Fb%20c" {
		t.Fatalf("encodedPathPart = %q", got)
	}
	if got := encodedQuery("a/b c"); got != "a%2Fb+c" {
		t.Fatalf("encodedQuery = %q", got)
	}
}

func TestPrettyPrintJSON(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
		_ = r.Close()
	}()

	if !shouldPrettyPrint(os.Stdout, "application/json", []byte(`{"ok":true}`)) {
		t.Fatal("expected stdout JSON to pretty print")
	}
	if err := prettyPrintJSON([]byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := prettyPrintJSON([]byte(`not-json`)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	if !strings.Contains(out, "\"ok\": true") || !strings.Contains(out, "not-json") {
		t.Fatalf("stdout = %q", out)
	}
}
