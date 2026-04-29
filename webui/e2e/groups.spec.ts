import { expect, test } from "@playwright/test";

test("group saves preserve and edit schedules", async ({ page }) => {
  let groups = [
    {
      name: "default",
      blocklists: ["ads"],
      allowlist: [],
      schedules: [
        { name: "school-night", days: ["mon", "tue"], from: "22:00", to: "07:00", block: ["adult"] },
      ],
    },
  ];
  let patchPayload: unknown;

  await page.route("**/api/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    if (path === "/api/v1/auth/me") {
      await route.fulfill({ json: { username: "admin", expires_at: "2099-01-01T00:00:00Z" } });
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
    if (path === "/api/v1/stats/top-domains") {
      await route.fulfill({ json: { domains: [] } });
      return;
    }
    if (path === "/api/v1/stats/top-clients") {
      await route.fulfill({ json: { clients: [] } });
      return;
    }
    if (path === "/api/v1/stats/timeseries") {
      await route.fulfill({ json: { bucket: "1m", rows: [] } });
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
    if (path === "/api/v1/clients") {
      await route.fulfill({ json: [] });
      return;
    }
    if (path === "/api/v1/groups" && request.method() === "GET") {
      await route.fulfill({ json: groups });
      return;
    }
    if (path === "/api/v1/groups/default" && request.method() === "PATCH") {
      patchPayload = request.postDataJSON();
      groups = [patchPayload as (typeof groups)[number]];
      await route.fulfill({ json: groups[0] });
      return;
    }
    if (path === "/api/v1/blocklists") {
      await route.fulfill({
        json: [
          { id: "ads", name: "Ads", url: "file:///ads.txt", enabled: true, refresh_interval: "24h" },
          { id: "adult", name: "Adult", url: "file:///adult.txt", enabled: true, refresh_interval: "24h" },
        ],
      });
      return;
    }
    if (path === "/api/v1/allowlist" || path === "/api/v1/custom-blocklist") {
      await route.fulfill({ json: { domains: [] } });
      return;
    }
    if (path === "/api/v1/upstreams") {
      await route.fulfill({ json: [] });
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
    await route.fulfill({ json: {} });
  });

  await page.goto("/");
  const groupsPanel = page.locator("section").filter({ has: page.getByRole("heading", { name: "Groups", exact: true }) });
  await expect(groupsPanel.getByText("school-night")).toBeVisible();

  await groupsPanel.getByLabel("Allowlist").fill("safe.example");
  await groupsPanel.getByRole("button", { name: "Save" }).click();
  expect(patchPayload).toMatchObject({
    name: "default",
    blocklists: ["ads"],
    allowlist: ["safe.example"],
    schedules: [{ name: "school-night", days: ["mon", "tue"], from: "22:00", to: "07:00", block: ["adult"] }],
  });

  await groupsPanel.getByRole("button", { name: "Add schedule" }).click();
  const newSchedule = groupsPanel.getByLabel("Schedule").last();
  await newSchedule.fill("weekend");
  await groupsPanel.getByLabel("Days").last().fill("sat, sun");
  await groupsPanel.getByRole("button", { name: "Save" }).click();

  expect(patchPayload).toMatchObject({
    schedules: [
      { name: "school-night", days: ["mon", "tue"], from: "22:00", to: "07:00", block: ["adult"] },
      { name: "weekend", days: ["sat", "sun"], from: "22:00", to: "07:00", block: [] },
    ],
  });
});
