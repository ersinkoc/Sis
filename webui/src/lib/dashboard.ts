import { useEffect, useState } from "react";

import { ApiError, apiRequest } from "./api";

export type StatsSummary = {
  query_total: number;
  cache_hit: number;
  cache_miss: number;
  blocked_total: number;
  rate_limited_total: number;
};

export type Client = {
  key: string;
  type: string;
  name: string;
  group: string;
  first_seen: string;
  last_seen: string;
  last_ip: string;
  hidden: boolean;
};

export type Group = {
  name: string;
  blocklists: string[];
  allowlist: string[];
  schedules?: Schedule[];
};

export type Schedule = {
  name: string;
  days: string[];
  from: string;
  to: string;
  block: string[];
};

export type Blocklist = {
  id: string;
  name: string;
  url: string;
  enabled: boolean;
  refresh_interval: string;
};

export type DomainList = {
  domains: string[];
};

export type BlocklistEntriesResponse = {
  entries: string[];
  count: number;
};

export type Upstream = {
  id: string;
  name: string;
  url: string;
  bootstrap: string[];
  healthy: boolean;
};

export type UpstreamTestResult = {
  rcode: number;
  latency_us: number;
  answers: number;
};

export type QueryLogEntry = {
  ts: string;
  client_key?: string;
  client_name?: string;
  client_group?: string;
  client_ip?: string;
  qname: string;
  qtype: string;
  rcode: string;
  blocked: boolean;
  block_reason?: string;
  upstream?: string;
  cache_hit: boolean;
  latency_us: number;
  proto: string;
};

export type QueryLogResponse = {
  entries: QueryLogEntry[];
};

export type QueryLogFilter = {
  client: string;
  qname: string;
  blocked: "all" | "true" | "false";
  limit: number;
};

export type Settings = {
  cache: {
    max_entries: number;
    min_ttl: string;
    max_ttl: string;
    negative_ttl: string;
  };
  privacy: {
    strip_ecs: boolean;
    block_local_ptr: boolean;
    log_mode: "full" | "hashed" | "anonymous";
    log_salt: string;
  };
  logging: {
    query_log: boolean;
    audit_log: boolean;
    rotate_size_mb: number;
    retention_days: number;
    gzip: boolean;
  };
  block: {
    response_a: string;
    response_aaaa: string;
    response_ttl: string;
    use_nxdomain: boolean;
  };
};

export type SystemInfo = {
  service: string;
  dns_listen?: string[];
  http_listen?: string;
  http_tls?: boolean;
  data_dir?: string;
  store_backend?: string;
  first_run?: boolean;
};

export type CacheFlushResult = {
  flushed: boolean;
  entries: number;
};

export type StoreVerifyResult = {
  ok: boolean;
  store: {
    backend: string;
    path: string;
    records: number;
    schema_version: number;
  };
};

export type ConfigSnapshot = {
  ts: string;
  yaml: string;
};

export type ConfigHistoryResponse = {
  snapshots: ConfigSnapshot[];
};

export type QueryTestResult = {
  domain: string;
  type: string;
  rcode: string;
  source: string;
  latency_us: number;
  answers: string[];
};

export type TopItem = {
  key: string;
  count: number;
};

export type TopDomainsResponse = {
  domains: TopItem[];
};

export type TopClientsResponse = {
  clients: TopItem[];
};

export type StatsRow = {
  bucket: string;
  counters: Record<string, number>;
};

export type TimeseriesResponse = {
  bucket: string;
  rows: StatsRow[];
};

export type DashboardState =
  | { status: "idle" | "loading" }
  | ReadyDashboardState
  | { status: "error"; message: string };

type ReadyDashboardState = {
  status: "ready";
  stats: StatsSummary;
  clients: Client[];
  groups: Group[];
  blocklists: Blocklist[];
  allowlist: string[];
  customBlocklist: string[];
  upstreams: Upstream[];
  logs: QueryLogEntry[];
  logsError: string;
  settings: Settings;
  system: SystemInfo;
  configHistory: ConfigSnapshot[];
  configHistoryError: string;
  topDomains: TopItem[];
  blockedDomains: TopItem[];
  topClients: TopItem[];
  topStatsError: string;
  timeseries: StatsRow[];
  timeseriesError: string;
};

type RequiredDashboardData = {
  stats: StatsSummary;
  clients: Client[];
  groups: Group[];
  blocklists: Blocklist[];
  allowlist: DomainList;
  customBlocklist: DomainList;
  upstreams: Upstream[];
  settings: Settings;
  system: SystemInfo;
};

type OptionalDashboardData = {
  logsResult: OptionalResult<QueryLogResponse>;
  historyResult: OptionalResult<ConfigHistoryResponse>;
  topDomainsResult: OptionalResult<TopDomainsResponse>;
  blockedDomainsResult: OptionalResult<TopDomainsResponse>;
  topClientsResult: OptionalResult<TopClientsResponse>;
  timeseriesResult: OptionalResult<TimeseriesResponse>;
};

type OptionalResult<T> = { ok: true; value: T } | { ok: false; message: string };

export function useDashboard(enabled: boolean) {
  const [state, setState] = useState<DashboardState>({ status: "idle" });
  const [version, setVersion] = useState(0);

  useEffect(() => {
    if (!enabled) {
      setState({ status: "idle" });
      return;
    }
    let cancelled = false;
    async function load() {
      setState({ status: "loading" });
      try {
        const [required, optional] = await Promise.all([
          loadRequiredDashboardData(),
          loadOptionalDashboardData(),
        ]);
        if (!cancelled) {
          setState(toReadyDashboardState(required, optional));
        }
      } catch (error) {
        if (!cancelled) {
          setState({ status: "error", message: errorMessage(error) });
        }
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [enabled, version]);

  return { state, reload: () => setVersion((current) => current + 1) };
}

async function loadRequiredDashboardData(): Promise<RequiredDashboardData> {
  const [
    stats,
    clients,
    groups,
    blocklists,
    allowlist,
    customBlocklist,
    upstreams,
    settings,
    system,
  ] = await Promise.all([
    apiRequest<StatsSummary>("/api/v1/stats/summary"),
    apiRequest<Client[]>("/api/v1/clients"),
    apiRequest<Group[]>("/api/v1/groups"),
    apiRequest<Blocklist[]>("/api/v1/blocklists"),
    apiRequest<DomainList>("/api/v1/allowlist"),
    apiRequest<DomainList>("/api/v1/custom-blocklist"),
    apiRequest<Upstream[]>("/api/v1/upstreams"),
    apiRequest<Settings>("/api/v1/settings"),
    apiRequest<SystemInfo>("/api/v1/system/info"),
  ]);
  return {
    stats,
    clients,
    groups,
    blocklists,
    allowlist,
    customBlocklist,
    upstreams,
    settings,
    system,
  };
}

async function loadOptionalDashboardData(): Promise<OptionalDashboardData> {
  const [
    logsResult,
    historyResult,
    topDomainsResult,
    blockedDomainsResult,
    topClientsResult,
    timeseriesResult,
  ] = await Promise.all([
    optionalRequest<QueryLogResponse>("/api/v1/logs/query?limit=8"),
    optionalRequest<ConfigHistoryResponse>("/api/v1/system/config/history?limit=3"),
    optionalRequest<TopDomainsResponse>("/api/v1/stats/top-domains?limit=5"),
    optionalRequest<TopDomainsResponse>("/api/v1/stats/top-domains?blocked=true&limit=5"),
    optionalRequest<TopClientsResponse>("/api/v1/stats/top-clients?limit=5"),
    optionalRequest<TimeseriesResponse>("/api/v1/stats/timeseries?bucket=1m&limit=12"),
  ]);
  return {
    logsResult,
    historyResult,
    topDomainsResult,
    blockedDomainsResult,
    topClientsResult,
    timeseriesResult,
  };
}

function toReadyDashboardState(
  required: RequiredDashboardData,
  optional: OptionalDashboardData,
): ReadyDashboardState {
  const topStatsError =
    firstError(
      optional.topDomainsResult,
      optional.blockedDomainsResult,
      optional.topClientsResult,
    ) ?? "";
  return {
    status: "ready",
    stats: required.stats,
    clients: required.clients,
    groups: required.groups,
    blocklists: required.blocklists,
    allowlist: required.allowlist.domains,
    customBlocklist: required.customBlocklist.domains,
    upstreams: required.upstreams,
    logs: optional.logsResult.ok ? optional.logsResult.value.entries : [],
    logsError: optional.logsResult.ok ? "" : optional.logsResult.message,
    settings: required.settings,
    system: required.system,
    configHistory: optional.historyResult.ok ? optional.historyResult.value.snapshots : [],
    configHistoryError: optional.historyResult.ok ? "" : optional.historyResult.message,
    topDomains: optional.topDomainsResult.ok ? optional.topDomainsResult.value.domains : [],
    blockedDomains: optional.blockedDomainsResult.ok
      ? optional.blockedDomainsResult.value.domains
      : [],
    topClients: optional.topClientsResult.ok ? optional.topClientsResult.value.clients : [],
    topStatsError,
    timeseries: optional.timeseriesResult.ok ? optional.timeseriesResult.value.rows : [],
    timeseriesError: optional.timeseriesResult.ok ? "" : optional.timeseriesResult.message,
  };
}

export async function updateClient(
  key: string,
  patch: Partial<Pick<Client, "name" | "group" | "hidden">>,
): Promise<Client> {
  return apiRequest<Client>(`/api/v1/clients/${encodeURIComponent(key)}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

export async function deleteClient(key: string): Promise<void> {
  await apiRequest<void>(`/api/v1/clients/${encodeURIComponent(key)}`, { method: "DELETE" });
}

export async function listQueryLogs(filter: QueryLogFilter): Promise<QueryLogResponse> {
  const params = new URLSearchParams({ limit: String(filter.limit) });
  if (filter.client.trim() !== "") {
    params.set("client", filter.client.trim());
  }
  if (filter.qname.trim() !== "") {
    params.set("qname", filter.qname.trim());
  }
  if (filter.blocked !== "all") {
    params.set("blocked", filter.blocked);
  }
  return apiRequest<QueryLogResponse>(`/api/v1/logs/query?${params.toString()}`);
}

export async function addDomainListEntry(kind: "allow" | "block", domain: string): Promise<void> {
  await apiRequest<{ domain: string }>(domainListPath(kind), {
    method: "POST",
    body: JSON.stringify({ domain }),
  });
}

export async function removeDomainListEntry(kind: "allow" | "block", domain: string): Promise<void> {
  await apiRequest<void>(`${domainListPath(kind)}/${encodeURIComponent(domain)}`, {
    method: "DELETE",
  });
}

export async function testUpstream(id: string): Promise<UpstreamTestResult> {
  return apiRequest<UpstreamTestResult>(`/api/v1/upstreams/${encodeURIComponent(id)}/test`, {
    method: "POST",
  });
}

export async function createUpstream(input: {
  id: string;
  name: string;
  url: string;
  bootstrap: string[];
}): Promise<Upstream> {
  return apiRequest<Upstream>("/api/v1/upstreams", {
    method: "POST",
    body: JSON.stringify({ ...input, timeout: "3s" }),
  });
}

export async function updateUpstream(
  id: string,
  input: {
    name: string;
    url: string;
    bootstrap: string[];
  },
): Promise<Upstream> {
  return apiRequest<Upstream>(`/api/v1/upstreams/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify({ id, ...input, timeout: "3s" }),
  });
}

export async function deleteUpstream(id: string): Promise<void> {
  await apiRequest<void>(`/api/v1/upstreams/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function updateSettings(settings: Settings): Promise<Settings> {
  return apiRequest<Settings>("/api/v1/settings", {
    method: "PATCH",
    body: JSON.stringify(settings),
  });
}

export async function flushCache(): Promise<CacheFlushResult> {
  return apiRequest<CacheFlushResult>("/api/v1/system/cache/flush", { method: "POST" });
}

export async function verifyStore(): Promise<StoreVerifyResult> {
  return apiRequest<StoreVerifyResult>("/api/v1/system/store/verify");
}

export async function reloadConfig(): Promise<void> {
  await apiRequest<{ reloaded: boolean }>("/api/v1/system/config/reload", { method: "POST" });
}

export async function runQueryTest(input: {
  domain: string;
  type: string;
  client_ip: string;
}): Promise<QueryTestResult> {
  return apiRequest<QueryTestResult>("/api/v1/query/test", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function createGroup(name: string): Promise<Group> {
  return apiRequest<Group>("/api/v1/groups", {
    method: "POST",
    body: JSON.stringify({ name, blocklists: [], allowlist: [], schedules: [] }),
  });
}

export async function updateGroup(
  name: string,
  input: Pick<Group, "name" | "blocklists" | "allowlist" | "schedules">,
): Promise<Group> {
  return apiRequest<Group>(`/api/v1/groups/${encodeURIComponent(name)}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export async function deleteGroup(name: string): Promise<void> {
  await apiRequest<void>(`/api/v1/groups/${encodeURIComponent(name)}`, { method: "DELETE" });
}

export async function createBlocklist(input: {
  id: string;
  name: string;
  url: string;
}): Promise<Blocklist> {
  return apiRequest<Blocklist>("/api/v1/blocklists", {
    method: "POST",
    body: JSON.stringify({
      ...input,
      enabled: true,
      refresh_interval: "24h",
    }),
  });
}

export async function updateBlocklist(
  id: string,
  input: {
    name: string;
    url: string;
    enabled: boolean;
    refresh_interval: string;
  },
): Promise<Blocklist> {
  return apiRequest<Blocklist>(`/api/v1/blocklists/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify({ id, ...input }),
  });
}

export async function deleteBlocklist(id: string): Promise<void> {
  await apiRequest<void>(`/api/v1/blocklists/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export async function syncBlocklist(id: string): Promise<void> {
  await apiRequest<unknown>(`/api/v1/blocklists/${encodeURIComponent(id)}/sync`, {
    method: "POST",
  });
}

export async function listBlocklistEntries(
  id: string,
  query: string,
  limit = 50,
): Promise<BlocklistEntriesResponse> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (query.trim() !== "") {
    params.set("q", query.trim());
  }
  return apiRequest<BlocklistEntriesResponse>(
    `/api/v1/blocklists/${encodeURIComponent(id)}/entries?${params.toString()}`,
  );
}

async function optionalRequest<T>(
  path: string,
): Promise<{ ok: true; value: T } | { ok: false; message: string }> {
  try {
    return { ok: true, value: await apiRequest<T>(path) };
  } catch (error) {
    return { ok: false, message: errorMessage(error) };
  }
}

function firstError(
  ...results: Array<{ ok: true; value: unknown } | { ok: false; message: string }>
) {
  return results.find((result) => !result.ok)?.message;
}

function domainListPath(kind: "allow" | "block") {
  return kind === "allow" ? "/api/v1/allowlist" : "/api/v1/custom-blocklist";
}

function errorMessage(error: unknown) {
  if (error instanceof ApiError) {
    return error.body.trim() || `HTTP ${error.status}`;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return "request failed";
}
