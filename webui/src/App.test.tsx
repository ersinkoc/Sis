import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { App } from "./App";
import { AuthProvider } from "./lib/auth";
import { ThemeProvider } from "./lib/theme";

function renderApp() {
  render(
    <ThemeProvider>
      <AuthProvider>
        <App />
      </AuthProvider>
    </ThemeProvider>,
  );
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

function textResponse(body: string, status: number) {
  return new Response(body, { status });
}

describe("App", () => {
  it("renders the first-run setup form when the API requires setup", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(textResponse("setup required", 428));

    renderApp();

    expect(await screen.findByRole("heading", { name: "Create Admin" })).toBeInTheDocument();
    expect(screen.getByLabelText("Username")).toHaveValue("admin");
    expect(screen.getByText("setup")).toBeInTheDocument();
  });

  it("renders authenticated dashboard data from API responses", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(mockDashboardFetch());

    renderApp();

    expect(await screen.findByRole("heading", { name: "Live Summary" })).toBeInTheDocument();
    expect(screen.getByText("alice")).toBeInTheDocument();
    expect(screen.getByText("Queries")).toBeInTheDocument();
    expect(screen.getByText("123")).toBeInTheDocument();
    expect(screen.getAllByText("blocked.example.com").length).toBeGreaterThan(0);
    expect(screen.getByText("cloudflare")).toBeInTheDocument();
  });

  it("submits query tests with the selected type and client IP", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(mockDashboardFetch());

    renderApp();

    await screen.findByRole("heading", { name: "Live Summary" });
    const queryForm = screen.getByRole("heading", { name: "Query Test" }).closest("form");
    expect(queryForm).not.toBeNull();

    const query = within(queryForm as HTMLFormElement);
    fireEvent.change(query.getByLabelText("Domain"), { target: { value: "blocked.example.com" } });
    fireEvent.change(query.getByLabelText("Type"), { target: { value: "AAAA" } });
    fireEvent.change(query.getByLabelText("Client IP"), { target: { value: "192.0.2.10" } });
    fireEvent.click(query.getByRole("button", { name: "Run" }));

    expect(await screen.findByText("cache")).toBeInTheDocument();
    expect(screen.getByText("blocked.example.com. 60 IN AAAA ::")).toBeInTheDocument();

    await waitFor(() => {
      const request = findFetchCall(fetchMock, "/api/v1/query/test", "POST");
      expect(request).toMatchObject({
        domain: "blocked.example.com",
        type: "AAAA",
        client_ip: "192.0.2.10",
      });
    });
  });

  it("patches settings with edited cache and privacy values", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(mockDashboardFetch());

    renderApp();

    await screen.findByRole("heading", { name: "Live Summary" });
    const settingsForm = screen.getByRole("heading", { name: "Settings" }).closest("form");
    expect(settingsForm).not.toBeNull();

    const settings = within(settingsForm as HTMLFormElement);
    fireEvent.change(settings.getByLabelText("Cache entries"), { target: { value: "2000" } });
    fireEvent.change(settings.getByLabelText("Log mode"), { target: { value: "hashed" } });
    fireEvent.click(settings.getByRole("button", { name: "Save" }));

    await waitFor(() => {
      const request = findFetchCall(fetchMock, "/api/v1/settings", "PATCH");
      expect(request).toMatchObject({
        cache: expect.objectContaining({ max_entries: 2000 }),
        privacy: expect.objectContaining({ log_mode: "hashed" }),
      });
    });
  });
});

function mockDashboardFetch() {
  return async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = input instanceof Request ? input.url : String(input);
    const method = init?.method ?? (input instanceof Request ? input.method : "GET");

    if (method === "POST" && path === "/api/v1/query/test") {
      return jsonResponse({
        domain: "blocked.example.com",
        type: "AAAA",
        rcode: "NOERROR",
        source: "cache",
        latency_us: 42,
        answers: ["blocked.example.com. 60 IN AAAA ::"],
      });
    }
    if (method === "PATCH" && path === "/api/v1/settings") {
      return jsonResponse(JSON.parse(String(init?.body ?? "{}")));
    }

    const responses = dashboardResponses();
    const body = responses[path];
    if (body == null) {
      return textResponse(`unhandled ${method} ${path}`, 500);
    }
    return jsonResponse(body);
  };
}

function findFetchCall(
  fetchMock: { mock: { calls: Array<[RequestInfo | URL, RequestInit?]> } },
  path: string,
  method: string,
) {
  const call = fetchMock.mock.calls.find(([input, init]) => {
    const requestPath = input instanceof Request ? input.url : String(input);
    const requestMethod = init?.method ?? (input instanceof Request ? input.method : "GET");
    return requestPath === path && requestMethod === method;
  });
  expect(call).toBeDefined();
  return JSON.parse(String(call?.[1]?.body ?? "{}"));
}

function dashboardResponses(): Record<string, unknown> {
  return {
    "/api/v1/auth/me": { username: "alice", expires_at: "2026-04-30T12:00:00Z" },
    "/api/v1/stats/summary": {
      query_total: 123,
      cache_hit: 12,
      cache_miss: 111,
      blocked_total: 7,
      rate_limited_total: 1,
      malformed_total: 0,
    },
    "/api/v1/clients": [
      {
        key: "192.0.2.10",
        type: "ip",
        name: "laptop",
        group: "default",
        first_seen: "2026-04-30T10:00:00Z",
        last_seen: "2026-04-30T10:01:00Z",
        last_ip: "192.0.2.10",
        hidden: false,
      },
    ],
    "/api/v1/groups": [{ name: "default", blocklists: ["ads"], allowlist: [], schedules: [] }],
    "/api/v1/blocklists": [
      {
        id: "ads",
        name: "Ads",
        url: "file:///tmp/ads.txt",
        enabled: true,
        refresh_interval: "24h",
      },
    ],
    "/api/v1/allowlist": { domains: ["allowed.example.com"] },
    "/api/v1/custom-blocklist": { domains: ["blocked.example.com"] },
    "/api/v1/upstreams": [
      {
        id: "cloudflare",
        name: "Cloudflare",
        url: "https://cloudflare-dns.com/dns-query",
        bootstrap: ["1.1.1.1"],
        healthy: true,
      },
    ],
    "/api/v1/settings": {
      cache: { max_entries: 1000, min_ttl: "30s", max_ttl: "1h", negative_ttl: "30s" },
      privacy: {
        strip_ecs: true,
        block_local_ptr: true,
        log_mode: "full",
        log_salt: "redacted",
      },
      logging: {
        query_log: true,
        audit_log: true,
        rotate_size_mb: 10,
        retention_days: 7,
        gzip: true,
      },
      block: {
        response_a: "0.0.0.0",
        response_aaaa: "::",
        response_ttl: "1m",
        use_nxdomain: false,
      },
    },
    "/api/v1/system/info": {
      service: "sis",
      dns_listen: ["127.0.0.1:5353"],
      http_listen: "127.0.0.1:8080",
      http_tls: false,
      data_dir: "/tmp/sis",
      store_backend: "json",
      first_run: false,
    },
    "/api/v1/logs/query?limit=8": {
      entries: [
        {
          ts: "2026-04-30T10:00:00Z",
          client_key: "192.0.2.10",
          client_name: "laptop",
          client_group: "default",
          client_ip: "192.0.2.10",
          qname: "blocked.example.com",
          qtype: "A",
          rcode: "NOERROR",
          blocked: true,
          block_reason: "list",
          upstream: "",
          cache_hit: false,
          latency_us: 100,
          proto: "udp",
        },
      ],
    },
    "/api/v1/system/config/history?limit=3": { snapshots: [] },
    "/api/v1/stats/top-domains?limit=5": { domains: [{ key: "example.com", count: 10 }] },
    "/api/v1/stats/top-domains?blocked=true&limit=5": {
      domains: [{ key: "blocked.example.com", count: 7 }],
    },
    "/api/v1/stats/top-clients?limit=5": { clients: [{ key: "laptop", count: 12 }] },
    "/api/v1/stats/timeseries?bucket=1m&limit=12": {
      bucket: "1m",
      rows: [{ bucket: "2026-04-30T10:00:00Z", counters: { query_total: 123, blocked_total: 7 } }],
    },
  };
}
