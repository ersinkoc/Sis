package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const maxCLIResponseBytes = 8 << 20

type cliClient struct {
	base   string
	client *http.Client
	cookie string
}

func newCLIClient(base, cookie string) *cliClient {
	if base == "" {
		base = "http://127.0.0.1:8080"
	}
	return &cliClient{base: base, cookie: cookie, client: &http.Client{Timeout: 30 * time.Second}}
}

func (c *cliClient) get(path string, out io.Writer) error {
	return c.do(http.MethodGet, path, nil, out)
}

func (c *cliClient) post(path string, body any, out io.Writer) error {
	return c.do(http.MethodPost, path, body, out)
}

func (c *cliClient) patch(path string, body any, out io.Writer) error {
	return c.do(http.MethodPatch, path, body, out)
}

func (c *cliClient) delete(path string, out io.Writer) error {
	return c.do(http.MethodDelete, path, nil, out)
}

func (c *cliClient) do(method, path string, body any, out io.Writer) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out != nil && isEventStream(resp.Header.Get("Content-Type")) {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			raw, err := io.ReadAll(io.LimitReader(resp.Body, maxCLIResponseBytes+1))
			if err != nil {
				return err
			}
			if len(raw) > maxCLIResponseBytes {
				return fmt.Errorf("HTTP response exceeds %d bytes", maxCLIResponseBytes)
			}
			return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
		}
		_, err := io.Copy(out, resp.Body)
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxCLIResponseBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxCLIResponseBytes {
		return fmt.Errorf("HTTP response exceeds %d bytes", maxCLIResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(raw))
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name != "" && cookie.Value != "" {
			fmt.Fprintf(os.Stderr, "cookie: %s=%s\n", cookie.Name, cookie.Value)
		}
	}
	if out != nil {
		if shouldPrettyPrint(out, resp.Header.Get("Content-Type"), raw) {
			return prettyPrintJSON(raw)
		}
		_, _ = out.Write(raw)
	}
	return nil
}

func shouldPrettyPrint(out io.Writer, contentType string, raw []byte) bool {
	file, ok := out.(*os.File)
	if !ok || file != os.Stdout {
		return false
	}
	if len(raw) == 0 {
		return false
	}
	if contentType != "" && !isJSONContent(contentType) {
		return false
	}
	return json.Valid(raw)
}

func isJSONContent(contentType string) bool {
	return mediaTypeMatches(contentType, "application/json")
}

func isEventStream(contentType string) bool {
	return mediaTypeMatches(contentType, "text/event-stream")
}

func mediaTypeMatches(contentType, want string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return contentType == want || strings.HasPrefix(contentType, want+";")
}

func prettyPrintJSON(raw []byte) error {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		fmt.Println(string(raw))
		return nil
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func responseBuffer() io.Writer {
	return os.Stdout
}

func printResponse(err error) error {
	if err != nil {
		return err
	}
	return nil
}

func encodedPathPart(v string) string {
	return url.PathEscape(v)
}

func encodedQuery(v string) string {
	return url.QueryEscape(v)
}
