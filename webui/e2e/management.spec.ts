import { expect, test, type Page, type Route } from "@playwright/test";

type Method = "GET" | "POST" | "PATCH" | "DELETE";

test("login opens dashboard", async ({ page }) => {
  let authenticated = false;
  await mockDashboardAPI(page, {
    authenticated: () => authenticated,
    onLogin: () => {
      authenticated = true;
    },
  });

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Sign In" })).toBeVisible();
  await page.getByLabel("Username").fill("admin");
  await page.getByLabel("Password").fill("change-me-now");
  await page.getByRole("button", { name: "Sign in" }).click();

  await expect(page.getByRole("heading", { name: "Live Summary" })).toBeVisible();
  await expect(page.getByText("admin").first()).toBeVisible();
});

test("client edit, upstream, blocklist, and domain list controls call APIs", async ({ page }) => {
  const calls: string[] = [];
  let clients = [
    {
      key: "192.0.2.10",
      type: "ip",
      name: "Phone",
      group: "default",
      first_seen: "2026-04-29T00:00:00Z",
      last_seen: "2026-04-29T00:00:00Z",
      last_ip: "192.0.2.10",
      hidden: false,
    },
  ];
  const groups = [
    { name: "default", blocklists: ["ads"], allowlist: [], schedules: [] },
    { name: "iot", blocklists: ["ads"], allowlist: [], schedules: [] },
  ];
  let upstreams = [
    {
      id: "cloudflare",
      name: "Cloudflare",
      url: "https://cloudflare-dns.com/dns-query",
      bootstrap: ["1.1.1.1"],
      healthy: true,
    },
  ];
  let blocklists = [
    { id: "ads", name: "Ads", url: "file:///ads.txt", enabled: true, refresh_interval: "24h" },
  ];
  let allowlist = ["safe.example"];
  let customBlocklist = ["blocked.example"];

  await mockDashboardAPI(page, {
    clients: () => clients,
    groups: () => groups,
    upstreams: () => upstreams,
    blocklists: () => blocklists,
    allowlist: () => allowlist,
    customBlocklist: () => customBlocklist,
    handler: async (route, method, path) => {
      calls.push(`${method} ${path}`);
      if (method === "PATCH" && path === "/api/v1/clients/192.0.2.10") {
        const patch = route.request().postDataJSON() as Partial<(typeof clients)[number]>;
        clients = [{ ...clients[0], ...patch }];
        await route.fulfill({ json: clients[0] });
        return true;
      }
      if (method === "POST" && path === "/api/v1/upstreams") {
        const created = route.request().postDataJSON() as (typeof upstreams)[number];
        upstreams = [...upstreams, { ...created, healthy: true }];
        await route.fulfill({ status: 201, json: upstreams.at(-1) });
        return true;
      }
      if (method === "POST" && path === "/api/v1/upstreams/quad9/test") {
        await route.fulfill({ json: { rcode: 0, latency_us: 1234, answers: 1 } });
        return true;
      }
      if (method === "PATCH" && path === "/api/v1/upstreams/quad9") {
        const patch = route.request().postDataJSON() as (typeof upstreams)[number];
        upstreams = upstreams.map((upstream) =>
          upstream.id === "quad9" ? { ...upstream, ...patch } : upstream,
        );
        await route.fulfill({ json: upstreams.find((upstream) => upstream.id === "quad9") });
        return true;
      }
      if (method === "DELETE" && path === "/api/v1/upstreams/quad9") {
        upstreams = upstreams.filter((upstream) => upstream.id !== "quad9");
        await route.fulfill({ status: 204 });
        return true;
      }
      if (method === "POST" && path === "/api/v1/blocklists") {
        const created = route.request().postDataJSON() as (typeof blocklists)[number];
        blocklists = [...blocklists, { ...created, enabled: true, refresh_interval: "24h" }];
        await route.fulfill({ status: 201, json: blocklists.at(-1) });
        return true;
      }
      if (method === "POST" && path === "/api/v1/blocklists/malware/sync") {
        await route.fulfill({ json: { id: "malware", accepted: 2, from_cache: false, not_modified: false } });
        return true;
      }
      if (method === "GET" && path === "/api/v1/blocklists/malware/entries") {
        await route.fulfill({ json: { entries: ["malware.example"], count: 1 } });
        return true;
      }
      if (method === "POST" && path === "/api/v1/allowlist") {
        const body = route.request().postDataJSON() as { domain: string };
        allowlist = [...allowlist, body.domain];
        await route.fulfill({ status: 204 });
        return true;
      }
      if (method === "DELETE" && path === "/api/v1/allowlist/safe.example") {
        allowlist = allowlist.filter((domain) => domain !== "safe.example");
        await route.fulfill({ status: 204 });
        return true;
      }
      if (method === "POST" && path === "/api/v1/custom-blocklist") {
        const body = route.request().postDataJSON() as { domain: string };
        customBlocklist = [...customBlocklist, body.domain];
        await route.fulfill({ status: 204 });
        return true;
      }
      if (method === "DELETE" && path === "/api/v1/custom-blocklist/blocked.example") {
        customBlocklist = customBlocklist.filter((domain) => domain !== "blocked.example");
        await route.fulfill({ status: 204 });
        return true;
      }
      return false;
    },
  });

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Clients", exact: true })).toBeVisible();

  const clientsPanel = panel(page, "Clients");
  const clientRow = clientsPanel.locator("tbody tr").filter({ hasText: "192.0.2.10" });
  await clientRow.locator("input").first().fill("Living Room Phone");
  await clientRow.locator("select").first().selectOption("iot");
  await clientRow.getByRole("button", { name: "Save" }).click();
  await expect.poll(() => calls).toContain("PATCH /api/v1/clients/192.0.2.10");

  const upstreamPanel = panel(page, "Upstreams");
  await upstreamPanel.getByPlaceholder("id").fill("quad9");
  await upstreamPanel.getByPlaceholder("name").fill("Quad9");
  await upstreamPanel.getByPlaceholder("https://dns.example/dns-query").fill("https://dns.quad9.net/dns-query");
  await upstreamPanel.getByPlaceholder("1.1.1.1, 1.0.0.1").fill("9.9.9.9");
  await upstreamPanel.getByRole("button", { name: "Add" }).click();
  await expect.poll(() => calls).toContain("POST /api/v1/upstreams");
  const quad9 = upstreamPanel.locator("article").filter({ hasText: "quad9" });
  await quad9.getByRole("button", { name: "Test" }).click();
  await expect.poll(() => calls).toContain("POST /api/v1/upstreams/quad9/test");
  await quad9.getByLabel("Name").fill("Quad9 DNS");
  await quad9.getByRole("button", { name: "Save" }).click();
  await expect.poll(() => calls).toContain("PATCH /api/v1/upstreams/quad9");
  await quad9.getByRole("button", { name: "Delete" }).click();
  await expect.poll(() => calls).toContain("DELETE /api/v1/upstreams/quad9");

  const blocklistsPanel = panel(page, "Blocklists");
  await blocklistsPanel.getByPlaceholder("id").fill("malware");
  await blocklistsPanel.getByPlaceholder("name").fill("Malware");
  await blocklistsPanel.getByPlaceholder("https://example.com/list.txt").fill("file:///malware.txt");
  await blocklistsPanel.getByRole("button", { name: "Add" }).click();
  await expect.poll(() => calls).toContain("POST /api/v1/blocklists");
  const malware = blocklistsPanel.locator("article").filter({ hasText: "malware" });
  await malware.getByRole("button", { name: "Sync" }).click();
  await expect.poll(() => calls).toContain("POST /api/v1/blocklists/malware/sync");
  await malware.getByRole("button", { name: "Inspect" }).click();
  await expect(blocklistsPanel.getByText("malware.example")).toBeVisible();

  const allowlistPanel = panel(page, "Allowlist");
  await allowlistPanel.getByPlaceholder("example.com").fill("allowed.example");
  await allowlistPanel.getByRole("button", { name: "Add" }).click();
  await expect.poll(() => calls).toContain("POST /api/v1/allowlist");
  await allowlistPanel.getByRole("button", { name: "Remove" }).first().click();
  await expect.poll(() => calls).toContain("DELETE /api/v1/allowlist/safe.example");

  const customBlocklistPanel = panel(page, "Custom Blocklist");
  await customBlocklistPanel.getByPlaceholder("example.com").fill("deny.example");
  await customBlocklistPanel.getByRole("button", { name: "Add" }).click();
  await expect.poll(() => calls).toContain("POST /api/v1/custom-blocklist");
  await customBlocklistPanel.getByRole("button", { name: "Remove" }).first().click();
  await expect.poll(() => calls).toContain("DELETE /api/v1/custom-blocklist/blocked.example");
});

function panel(page: Page, name: string) {
  return page
    .getByRole("heading", { name, exact: true })
    .locator("xpath=ancestor::section[1]");
}

async function mockDashboardAPI(
  page: Page,
  overrides: {
    authenticated?: () => boolean;
    onLogin?: () => void;
    clients?: () => unknown[];
    groups?: () => unknown[];
    upstreams?: () => unknown[];
    blocklists?: () => unknown[];
    allowlist?: () => string[];
    customBlocklist?: () => string[];
    handler?: (route: Route, method: Method, path: string) => Promise<boolean>;
  } = {},
) {
  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const method = request.method() as Method;

    if ((await overrides.handler?.(route, method, path)) === true) {
      return;
    }
    if (path === "/api/v1/auth/me") {
      if (overrides.authenticated?.() === false) {
        await route.fulfill({ status: 401, json: { error: "unauthorized" } });
        return;
      }
      await route.fulfill({ json: { username: "admin", expires_at: "2099-01-01T00:00:00Z" } });
      return;
    }
    if (path === "/api/v1/auth/login" && method === "POST") {
      overrides.onLogin?.();
      await route.fulfill({ json: { username: "admin" } });
      return;
    }
    if (path === "/api/v1/stats/summary") {
      await route.fulfill({
        json: {
          query_total: 0,
          cache_hit: 0,
          cache_miss: 0,
          blocked_total: 0,
          rate_limited_total: 0,
          malformed_total: 0,
        },
      });
      return;
    }
    if (path === "/api/v1/clients") {
      await route.fulfill({ json: overrides.clients?.() ?? [] });
      return;
    }
    if (path === "/api/v1/groups") {
      await route.fulfill({ json: overrides.groups?.() ?? [{ name: "default", blocklists: [], allowlist: [], schedules: [] }] });
      return;
    }
    if (path === "/api/v1/blocklists") {
      await route.fulfill({ json: overrides.blocklists?.() ?? [] });
      return;
    }
    if (path === "/api/v1/allowlist") {
      await route.fulfill({ json: { domains: overrides.allowlist?.() ?? [] } });
      return;
    }
    if (path === "/api/v1/custom-blocklist") {
      await route.fulfill({ json: { domains: overrides.customBlocklist?.() ?? [] } });
      return;
    }
    if (path === "/api/v1/upstreams") {
      await route.fulfill({ json: overrides.upstreams?.() ?? [] });
      return;
    }
    if (path === "/api/v1/settings") {
      await route.fulfill({
        json: {
          cache: { max_entries: 1000, min_ttl: "1m", max_ttl: "1h", negative_ttl: "30s" },
          privacy: { strip_ecs: true, block_local_ptr: true, log_mode: "full", log_salt: "" },
          logging: { query_log: true, audit_log: true, rotate_size_mb: 100, retention_days: 7, gzip: true },
          block: { response_a: "0.0.0.0", response_aaaa: "::", response_ttl: "1m", use_nxdomain: false },
        },
      });
      return;
    }
    if (path === "/api/v1/system/info") {
      await route.fulfill({ json: { service: "sis", store_backend: "json", first_run: false } });
      return;
    }
    if (path === "/api/v1/logs/query") {
      await route.fulfill({ json: { entries: [] } });
      return;
    }
    if (path === "/api/v1/system/config/history") {
      await route.fulfill({ json: { snapshots: [] } });
      return;
    }
    if (path === "/api/v1/stats/top-domains" || path === "/api/v1/stats/top-clients") {
      await route.fulfill({ json: path.endsWith("top-clients") ? { clients: [] } : { domains: [] } });
      return;
    }
    if (path === "/api/v1/stats/timeseries") {
      await route.fulfill({ json: { bucket: "1m", rows: [] } });
      return;
    }
    await route.fulfill({ json: {} });
  });
}
