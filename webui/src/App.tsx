import { FormEvent, useEffect, useMemo, useState } from "react";

import { ApiError } from "./lib/api";
import { useAuth } from "./lib/auth";
import {
  addDomainListEntry,
  Blocklist,
  Client,
  ConfigSnapshot,
  createBlocklist,
  createGroup,
  createUpstream,
  DashboardState,
  deleteBlocklist,
  deleteClient,
  deleteGroup,
  deleteUpstream,
  flushCache,
  Group,
  listBlocklistEntries,
  listQueryLogs,
  QueryLogEntry,
  QueryLogFilter,
  QueryTestResult,
  reloadConfig,
  removeDomainListEntry,
  runQueryTest,
  Settings,
  Schedule,
  StatsSummary,
  StatsRow,
  StoreVerifyResult,
  syncBlocklist,
  SystemInfo,
  testUpstream,
  TopItem,
  Upstream,
  UpstreamTestResult,
  updateBlocklist,
  updateClient,
  updateGroup,
  updateSettings,
  updateUpstream,
  useDashboard,
  verifyStore,
} from "./lib/dashboard";
import { useTheme } from "./lib/theme";

const statusItems = [
  { label: "DNS ingress", value: "UDP/TCP" },
  { label: "Upstream", value: "DoH" },
  { label: "API", value: "/api/v1" },
  { label: "Health", value: "/healthz" },
];

export function App() {
  const { state, logout } = useAuth();
  const { nextTheme, theme } = useTheme();
  const dashboard = useDashboard(state.status === "authenticated");
  const authLabel = authStatusLabel(state);

  return (
    <main className="min-h-screen bg-[#f7f9fb] text-[#17202a] dark:bg-[#101418] dark:text-[#eef2f6]">
      <section className="mx-auto grid min-h-screen w-full max-w-6xl grid-rows-[auto_1fr] px-6 py-6">
        <header className="flex items-center justify-between border-b border-[#d8dee6] pb-4 dark:border-[#2c3540]">
          <div>
            <h1 className="text-2xl font-semibold tracking-normal">Sis</h1>
            <p className="mt-1 text-sm text-[#637083] dark:text-[#a6b1bd]">
              Sorgular siste, cevaplar berrak.
            </p>
          </div>
          <div className="flex items-center gap-3">
            <button
              className="rounded border border-[#c8d1dc] px-3 py-1 text-sm text-[#354252] hover:bg-[#ecf1f5] dark:border-[#3a4654] dark:text-[#c3ccd6] dark:hover:bg-[#1d252d]"
              type="button"
              onClick={nextTheme}
            >
              {theme}
            </button>
            <span className="rounded border border-[#c8d1dc] px-3 py-1 text-sm text-[#354252] dark:border-[#3a4654] dark:text-[#c3ccd6]">
              {authLabel}
            </span>
            {state.status === "authenticated" ? (
              <button
                className="rounded border border-[#c8d1dc] px-3 py-1 text-sm text-[#354252] hover:bg-[#ecf1f5] dark:border-[#3a4654] dark:text-[#c3ccd6] dark:hover:bg-[#1d252d]"
                type="button"
                onClick={() => void logout()}
              >
                Sign out
              </button>
            ) : null}
          </div>
        </header>

        <div className="grid content-center gap-8 py-10">
          {state.status === "setup-required" || state.status === "unauthenticated" ? (
            <AuthForm mode={state.status === "setup-required" ? "setup" : "login"} />
          ) : null}

          <section>
            <h2 className="text-lg font-medium tracking-normal">Runtime Surface</h2>
            <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              {statusItems.map((item) => (
                <article
                  key={item.label}
                  className="rounded border border-[#d8dee6] bg-white p-4 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]"
                >
                  <p className="text-sm text-[#637083] dark:text-[#a6b1bd]">{item.label}</p>
                  <p className="mt-2 font-mono text-sm">{item.value}</p>
                </article>
              ))}
            </div>
          </section>

          <section className="grid gap-4 border-l-4 border-[#287d7d] bg-white px-5 py-4 shadow-sm dark:bg-[#151b21]">
            <h2 className="text-lg font-medium tracking-normal">Control Plane</h2>
            <dl className="grid gap-3 sm:grid-cols-3">
              <div>
                <dt className="text-sm text-[#637083] dark:text-[#a6b1bd]">Session</dt>
                <dd className="mt-1 font-mono text-sm">{authDetail(state)}</dd>
              </div>
              <div>
                <dt className="text-sm text-[#637083] dark:text-[#a6b1bd]">Policy</dt>
                <dd className="mt-1 font-mono text-sm">groups</dd>
              </div>
              <div>
                <dt className="text-sm text-[#637083] dark:text-[#a6b1bd]">Telemetry</dt>
                <dd className="mt-1 font-mono text-sm">stats</dd>
              </div>
            </dl>
          </section>

          {state.status === "authenticated" ? (
            <DashboardPanel state={dashboard.state} onChanged={dashboard.reload} />
          ) : null}
        </div>
      </section>
    </main>
  );
}

type AuthState = ReturnType<typeof useAuth>["state"];
function DashboardPanel({ state, onChanged }: { state: DashboardState; onChanged: () => void }) {
  if (state.status === "error") {
    return (
      <section className="border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]">
        <h2 className="text-lg font-medium tracking-normal">Dashboard</h2>
        <p className="mt-2 text-sm text-[#a33a3a]">{state.message}</p>
      </section>
    );
  }
  if (state.status !== "ready") {
    return (
      <section className="border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]">
        <h2 className="text-lg font-medium tracking-normal">Dashboard</h2>
        <p className="mt-2 text-sm text-[#637083] dark:text-[#a6b1bd]">Loading</p>
      </section>
    );
  }
  return (
    <section className="grid gap-5">
      <StatsCards stats={state.stats} />
      <TimeseriesPanel error={state.timeseriesError} rows={state.timeseries} />
      <TopStatsPanel
        blockedDomains={state.blockedDomains}
        error={state.topStatsError}
        topClients={state.topClients}
        topDomains={state.topDomains}
      />
      <SystemPanel system={state.system} onChanged={onChanged} />
      <QueryTestPanel onChanged={onChanged} />
      <ConfigHistoryPanel snapshots={state.configHistory} error={state.configHistoryError} />
      <UpstreamPanel upstreams={state.upstreams} onChanged={onChanged} />
      <QueryLogPanel entries={state.logs} error={state.logsError} />
      <SettingsPanel settings={state.settings} onChanged={onChanged} />
      <GroupsPanel
        groups={state.groups}
        clients={state.clients}
        blocklists={state.blocklists}
        onChanged={onChanged}
      />
      <BlocklistsPanel blocklists={state.blocklists} groups={state.groups} onChanged={onChanged} />
      <PolicyLists
        allowlist={state.allowlist}
        customBlocklist={state.customBlocklist}
        onChanged={onChanged}
      />
      <ClientTable clients={state.clients} groups={state.groups} onChanged={onChanged} />
    </section>
  );
}

function SystemPanel({ system, onChanged }: { system: SystemInfo; onChanged: () => void }) {
  const [busy, setBusy] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  async function run(action: "flush" | "reload" | "verify") {
    setBusy(action);
    setMessage("");
    setError("");
    try {
      if (action === "flush") {
        const result = await flushCache();
        setMessage(`flushed ${result.entries} entries`);
      } else if (action === "verify") {
        setMessage(storeVerifyMessage(await verifyStore()));
      } else {
        await reloadConfig();
        setMessage("config reloaded");
      }
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "operation failed");
    } finally {
      setBusy("");
    }
  }

  return (
    <section className="border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 className="text-lg font-medium tracking-normal">System</h2>
        <div className="flex gap-2">
          <button
            className="rounded border border-[#c8d1dc] px-3 py-1 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
            disabled={busy !== ""}
            type="button"
            onClick={() => void run("flush")}
          >
            {busy === "flush" ? "Flushing" : "Flush cache"}
          </button>
          <button
            className="rounded border border-[#c8d1dc] px-3 py-1 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
            disabled={busy !== ""}
            type="button"
            onClick={() => void run("verify")}
          >
            {busy === "verify" ? "Verifying" : "Verify store"}
          </button>
          <button
            className="rounded border border-[#c8d1dc] px-3 py-1 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
            disabled={busy !== ""}
            type="button"
            onClick={() => void run("reload")}
          >
            {busy === "reload" ? "Reloading" : "Reload config"}
          </button>
        </div>
      </div>
      {message !== "" ? <p className="mt-3 text-sm text-[#1d6a4f]">{message}</p> : null}
      {error !== "" ? <p className="mt-3 text-sm text-[#a33a3a]">{error}</p> : null}
      <dl className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
        <div>
          <dt className="text-sm text-[#637083] dark:text-[#a6b1bd]">DNS</dt>
          <dd className="mt-1 truncate font-mono text-sm">{system.dns_listen?.join(", ") || "-"}</dd>
        </div>
        <div>
          <dt className="text-sm text-[#637083] dark:text-[#a6b1bd]">HTTP</dt>
          <dd className="mt-1 truncate font-mono text-sm">
            {system.http_listen || "-"} {system.http_tls ? "tls" : ""}
          </dd>
        </div>
        <div>
          <dt className="text-sm text-[#637083] dark:text-[#a6b1bd]">Data</dt>
          <dd className="mt-1 truncate font-mono text-sm">{system.data_dir || "-"}</dd>
        </div>
        <div>
          <dt className="text-sm text-[#637083] dark:text-[#a6b1bd]">Store</dt>
          <dd className="mt-1 font-mono text-sm">{system.store_backend || "json"}</dd>
        </div>
        <div>
          <dt className="text-sm text-[#637083] dark:text-[#a6b1bd]">First run</dt>
          <dd className="mt-1 font-mono text-sm">{system.first_run ? "yes" : "no"}</dd>
        </div>
      </dl>
    </section>
  );
}

function storeVerifyMessage(result: StoreVerifyResult): string {
  return `store ${result.store.backend} verified: ${result.store.records} records, schema ${result.store.schema_version}`;
}

function TimeseriesPanel({ rows, error }: { rows: StatsRow[]; error: string }) {
  const max = Math.max(1, ...rows.map((row) => row.counters.query_total ?? 0));
  return (
    <section className="border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]">
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-lg font-medium tracking-normal">Query Trend</h2>
        <span className="font-mono text-sm text-[#637083] dark:text-[#a6b1bd]">{rows.length}</span>
      </div>
      {error !== "" ? (
        <p className="mt-3 text-sm text-[#a33a3a]">{error}</p>
      ) : (
        <div className="mt-4 grid gap-2">
          {rows.map((row) => {
            const queries = row.counters.query_total ?? 0;
            const blocked = row.counters.blocked_total ?? 0;
            return (
              <div key={row.bucket} className="grid grid-cols-[90px_1fr_120px] items-center gap-3">
                <span className="truncate font-mono text-xs text-[#637083] dark:text-[#a6b1bd]">
                  {formatBucket(row.bucket)}
                </span>
                <div className="h-3 overflow-hidden rounded bg-[#edf1f5] dark:bg-[#202932]">
                  <div
                    className="h-full bg-[#287d7d]"
                    style={{ width: `${Math.max(3, (queries / max) * 100)}%` }}
                  />
                </div>
                <span className="text-right font-mono text-xs">
                  {queries.toLocaleString()} q · {blocked.toLocaleString()} b
                </span>
              </div>
            );
          })}
          {rows.length === 0 ? (
            <p className="text-sm text-[#637083] dark:text-[#a6b1bd]">No timeseries rows</p>
          ) : null}
        </div>
      )}
    </section>
  );
}

function TopStatsPanel({
  topDomains,
  blockedDomains,
  topClients,
  error,
}: {
  topDomains: TopItem[];
  blockedDomains: TopItem[];
  topClients: TopItem[];
  error: string;
}) {
  return (
    <section className="grid gap-4 lg:grid-cols-3">
      <TopList title="Top Domains" items={topDomains} />
      <TopList title="Blocked Domains" items={blockedDomains} />
      <TopList title="Top Clients" items={topClients} />
      {error !== "" ? <p className="text-sm text-[#a33a3a] lg:col-span-3">{error}</p> : null}
    </section>
  );
}

function TopList({ title, items }: { title: string; items: TopItem[] }) {
  return (
    <article className="border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]">
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-lg font-medium tracking-normal">{title}</h2>
        <span className="font-mono text-sm text-[#637083] dark:text-[#a6b1bd]">{items.length}</span>
      </div>
      <ol className="mt-4 grid gap-2">
        {items.map((item) => (
          <li
            key={item.key}
            className="grid grid-cols-[1fr_auto] gap-3 border-b border-[#edf1f5] py-2 last:border-b-0 dark:border-[#202932]"
          >
            <span className="min-w-0 truncate font-mono text-sm">{item.key}</span>
            <span className="font-mono text-sm text-[#637083] dark:text-[#a6b1bd]">
              {item.count.toLocaleString()}
            </span>
          </li>
        ))}
        {items.length === 0 ? (
          <li className="py-2 text-sm text-[#637083] dark:text-[#a6b1bd]">No activity</li>
        ) : null}
      </ol>
    </article>
  );
}

function QueryTestPanel({ onChanged }: { onChanged: () => void }) {
  const [domain, setDomain] = useState("example.com");
  const [type, setType] = useState("A");
  const [clientIP, setClientIP] = useState("");
  const [result, setResult] = useState<QueryTestResult | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const next = await runQueryTest({ domain, type, client_ip: clientIP });
      setResult(next);
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "query test failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form
      className="grid gap-4 border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]"
      onSubmit={(event) => void submit(event)}
    >
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 className="text-lg font-medium tracking-normal">Query Test</h2>
        <button
          className="rounded bg-[#287d7d] px-4 py-2 text-sm font-medium text-white hover:bg-[#216b6b] disabled:cursor-not-allowed disabled:opacity-60"
          disabled={busy || domain.trim() === ""}
          type="submit"
        >
          {busy ? "Running" : "Run"}
        </button>
      </div>
      <div className="grid gap-3 md:grid-cols-[1fr_120px_180px]">
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Domain</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            value={domain}
            onChange={(event) => setDomain(event.currentTarget.value)}
          />
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Type</span>
          <select
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            value={type}
            onChange={(event) => setType(event.currentTarget.value)}
          >
            {["A", "AAAA", "CNAME", "MX", "TXT", "NS", "SOA", "PTR"].map((qtype) => (
              <option key={qtype} value={qtype}>
                {qtype}
              </option>
            ))}
          </select>
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Client IP</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            placeholder="optional"
            value={clientIP}
            onChange={(event) => setClientIP(event.currentTarget.value)}
          />
        </label>
      </div>
      {error !== "" ? <p className="text-sm text-[#a33a3a]">{error}</p> : null}
      {result ? (
        <div className="grid gap-3 rounded border border-[#d8dee6] bg-[#f7f9fb] p-3 dark:border-[#2c3540] dark:bg-[#101418]">
          <dl className="grid gap-3 sm:grid-cols-4">
            <div>
              <dt className="text-sm text-[#637083] dark:text-[#a6b1bd]">RCode</dt>
              <dd className="mt-1 font-mono text-sm">{result.rcode}</dd>
            </div>
            <div>
              <dt className="text-sm text-[#637083] dark:text-[#a6b1bd]">Source</dt>
              <dd className="mt-1 font-mono text-sm">{result.source}</dd>
            </div>
            <div>
              <dt className="text-sm text-[#637083] dark:text-[#a6b1bd]">Latency</dt>
              <dd className="mt-1 font-mono text-sm">{result.latency_us} us</dd>
            </div>
            <div>
              <dt className="text-sm text-[#637083] dark:text-[#a6b1bd]">Answers</dt>
              <dd className="mt-1 font-mono text-sm">{result.answers.length}</dd>
            </div>
          </dl>
          {result.answers.length > 0 ? (
            <pre className="max-h-40 overflow-auto font-mono text-xs leading-5">
              {result.answers.join("\n")}
            </pre>
          ) : null}
        </div>
      ) : null}
    </form>
  );
}

function ConfigHistoryPanel({
  snapshots,
  error,
}: {
  snapshots: ConfigSnapshot[];
  error: string;
}) {
  const [selected, setSelected] = useState(0);
  const active = snapshots[selected];

  useEffect(() => {
    setSelected(0);
  }, [snapshots]);

  return (
    <section className="border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]">
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-lg font-medium tracking-normal">Config History</h2>
        <span className="font-mono text-sm text-[#637083] dark:text-[#a6b1bd]">
          {snapshots.length}
        </span>
      </div>
      {error !== "" ? (
        <p className="mt-3 text-sm text-[#a33a3a]">{error}</p>
      ) : (
        <div className="mt-4 grid gap-4 lg:grid-cols-[220px_1fr]">
          <div className="grid content-start gap-2">
            {snapshots.map((snapshot, index) => (
              <button
                key={`${snapshot.ts}-${index}`}
                className={
                  selected === index
                    ? "rounded border border-[#287d7d] bg-[#e5f3f1] px-3 py-2 text-left font-mono text-sm dark:bg-[#18362e]"
                    : "rounded border border-[#c8d1dc] px-3 py-2 text-left font-mono text-sm hover:bg-[#ecf1f5] dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                }
                type="button"
                onClick={() => setSelected(index)}
              >
                {snapshot.ts}
              </button>
            ))}
            {snapshots.length === 0 ? (
              <p className="text-sm text-[#637083] dark:text-[#a6b1bd]">No snapshots</p>
            ) : null}
          </div>
          <pre className="max-h-72 overflow-auto rounded border border-[#d8dee6] bg-[#f7f9fb] p-3 font-mono text-xs leading-5 dark:border-[#2c3540] dark:bg-[#101418]">
            {active?.yaml || ""}
          </pre>
        </div>
      )}
    </section>
  );
}

function QueryLogPanel({ entries, error }: { entries: QueryLogEntry[]; error: string }) {
  const [shown, setShown] = useState(entries);
  const [filter, setFilter] = useState<QueryLogFilter>({
    client: "",
    qname: "",
    blocked: "all",
    limit: 25,
  });
  const [busy, setBusy] = useState(false);
  const [filterError, setFilterError] = useState("");

  useEffect(() => {
    setShown(entries);
  }, [entries]);

  async function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setFilterError("");
    try {
      const result = await listQueryLogs(filter);
      setShown(result.entries);
    } catch (err) {
      setFilterError(err instanceof ApiError ? err.body.trim() : "query log search failed");
    } finally {
      setBusy(false);
    }
  }

  function reset() {
    setFilter({ client: "", qname: "", blocked: "all", limit: 25 });
    setShown(entries);
    setFilterError("");
  }

  return (
    <section className="border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 className="text-lg font-medium tracking-normal">Recent Queries</h2>
        <span className="font-mono text-sm text-[#637083] dark:text-[#a6b1bd]">
          {shown.length}
        </span>
      </div>
      {error !== "" ? (
        <p className="mt-3 text-sm text-[#a33a3a]">{error}</p>
      ) : (
        <div className="mt-4 grid gap-4">
          <form
            className="grid gap-3 lg:grid-cols-[1fr_1fr_150px_120px_auto_auto]"
            onSubmit={(event) => void search(event)}
          >
            <input
              className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
              placeholder="client key, IP, name"
              value={filter.client}
              onChange={(event) => setFilter({ ...filter, client: event.currentTarget.value })}
            />
            <input
              className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
              placeholder="domain contains"
              value={filter.qname}
              onChange={(event) => setFilter({ ...filter, qname: event.currentTarget.value })}
            />
            <select
              className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
              value={filter.blocked}
              onChange={(event) =>
                setFilter({
                  ...filter,
                  blocked: event.currentTarget.value as QueryLogFilter["blocked"],
                })
              }
            >
              <option value="all">all</option>
              <option value="true">blocked</option>
              <option value="false">allowed</option>
            </select>
            <input
              className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
              min={1}
              max={1000}
              type="number"
              value={filter.limit}
              onChange={(event) =>
                setFilter({ ...filter, limit: Number(event.currentTarget.value) })
              }
            />
            <button
              className="rounded bg-[#287d7d] px-4 py-2 text-sm font-medium text-white hover:bg-[#216b6b] disabled:cursor-not-allowed disabled:opacity-60"
              disabled={busy}
              type="submit"
            >
              {busy ? "Searching" : "Search"}
            </button>
            <button
              className="rounded border border-[#c8d1dc] px-4 py-2 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
              disabled={busy}
              type="button"
              onClick={reset}
            >
              Reset
            </button>
          </form>
          {filterError !== "" ? <p className="text-sm text-[#a33a3a]">{filterError}</p> : null}
          <div className="overflow-x-auto">
            <table className="w-full min-w-[760px] border-collapse text-left text-sm">
              <thead className="border-b border-[#d8dee6] text-[#637083] dark:border-[#2c3540] dark:text-[#a6b1bd]">
                <tr>
                  <th className="py-2 pr-4 font-medium">Time</th>
                  <th className="py-2 pr-4 font-medium">Client</th>
                  <th className="py-2 pr-4 font-medium">Query</th>
                  <th className="py-2 pr-4 font-medium">Result</th>
                  <th className="py-2 font-medium">Path</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((entry, index) => (
                  <tr
                    key={`${entry.ts}-${entry.client_key ?? ""}-${entry.qname}-${index}`}
                    className="border-b border-[#edf1f5] dark:border-[#202932]"
                  >
                    <td className="py-3 pr-4 font-mono">{formatTime(entry.ts)}</td>
                    <td className="py-3 pr-4">
                      <span className="block font-mono">
                        {entry.client_name || entry.client_key || "-"}
                      </span>
                      {entry.client_ip ? (
                        <span className="block font-mono text-xs text-[#637083] dark:text-[#a6b1bd]">
                          {entry.client_ip}
                        </span>
                      ) : null}
                    </td>
                    <td className="py-3 pr-4">
                      <span className="block truncate font-mono">{entry.qname}</span>
                      <span className="block font-mono text-xs text-[#637083] dark:text-[#a6b1bd]">
                        {entry.qtype} · {entry.proto}
                      </span>
                    </td>
                    <td className="py-3 pr-4">
                      <span
                        className={
                          entry.blocked
                            ? "rounded bg-[#f5e1e1] px-2 py-0.5 text-xs text-[#9a3131] dark:bg-[#3a2020] dark:text-[#f0b0b0]"
                            : "rounded bg-[#dff3ec] px-2 py-0.5 text-xs text-[#1d6a4f] dark:bg-[#18362e] dark:text-[#9ce0c8]"
                        }
                      >
                        {entry.blocked ? "blocked" : entry.rcode}
                      </span>
                    </td>
                    <td className="py-3 font-mono">
                      {entry.cache_hit ? "cache" : entry.upstream || "-"}
                      <span className="ml-2 text-xs text-[#637083] dark:text-[#a6b1bd]">
                        {entry.latency_us} us
                      </span>
                    </td>
                  </tr>
                ))}
                {shown.length === 0 ? (
                  <tr>
                    <td className="py-4 text-[#637083] dark:text-[#a6b1bd]" colSpan={5}>
                      No recent queries
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </section>
  );
}

function SettingsPanel({ settings, onChanged }: { settings: Settings; onChanged: () => void }) {
  const [draft, setDraft] = useState(settings);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const changed = JSON.stringify(draft) !== JSON.stringify(settings);

  useEffect(() => {
    setDraft(settings);
  }, [settings]);

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setError("");
    try {
      await updateSettings(draft);
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "settings update failed");
    } finally {
      setSaving(false);
    }
  }

  return (
    <form
      className="grid gap-4 border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]"
      onSubmit={(event) => void save(event)}
    >
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-lg font-medium tracking-normal">Settings</h2>
        <div className="flex gap-2">
          <button
            className="rounded border border-[#c8d1dc] px-4 py-2 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
            disabled={!changed || saving}
            type="button"
            onClick={() => {
              setDraft(settings);
              setError("");
            }}
          >
            Reset
          </button>
          <button
            className="rounded bg-[#287d7d] px-4 py-2 text-sm font-medium text-white hover:bg-[#216b6b] disabled:cursor-not-allowed disabled:opacity-60"
            disabled={!changed || saving}
            type="submit"
          >
            {saving ? "Saving" : "Save"}
          </button>
        </div>
      </div>
      {error !== "" ? <p className="text-sm text-[#a33a3a]">{error}</p> : null}
      <div className="grid gap-4 lg:grid-cols-4">
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Cache entries</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            min={1}
            type="number"
            value={draft.cache.max_entries}
            onChange={(event) =>
              setDraft({
                ...draft,
                cache: { ...draft.cache, max_entries: Number(event.currentTarget.value) },
              })
            }
          />
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Min TTL</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            value={draft.cache.min_ttl}
            onChange={(event) =>
              setDraft({
                ...draft,
                cache: { ...draft.cache, min_ttl: event.currentTarget.value },
              })
            }
          />
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Max TTL</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            value={draft.cache.max_ttl}
            onChange={(event) =>
              setDraft({
                ...draft,
                cache: { ...draft.cache, max_ttl: event.currentTarget.value },
              })
            }
          />
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Negative TTL</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            value={draft.cache.negative_ttl}
            onChange={(event) =>
              setDraft({
                ...draft,
                cache: { ...draft.cache, negative_ttl: event.currentTarget.value },
              })
            }
          />
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Log mode</span>
          <select
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            value={draft.privacy.log_mode}
            onChange={(event) =>
              setDraft({
                ...draft,
                privacy: {
                  ...draft.privacy,
                  log_mode: event.currentTarget.value as Settings["privacy"]["log_mode"],
                },
              })
            }
          >
            <option value="full">full</option>
            <option value="hashed">hashed</option>
            <option value="anonymous">anonymous</option>
          </select>
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Block A</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            value={draft.block.response_a}
            onChange={(event) =>
              setDraft({
                ...draft,
                block: { ...draft.block, response_a: event.currentTarget.value },
              })
            }
          />
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Block AAAA</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            value={draft.block.response_aaaa}
            onChange={(event) =>
              setDraft({
                ...draft,
                block: { ...draft.block, response_aaaa: event.currentTarget.value },
              })
            }
          />
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Block TTL</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            value={draft.block.response_ttl}
            onChange={(event) =>
              setDraft({
                ...draft,
                block: { ...draft.block, response_ttl: event.currentTarget.value },
              })
            }
          />
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Rotate MB</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            min={1}
            type="number"
            value={draft.logging.rotate_size_mb}
            onChange={(event) =>
              setDraft({
                ...draft,
                logging: { ...draft.logging, rotate_size_mb: Number(event.currentTarget.value) },
              })
            }
          />
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Retention days</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            min={1}
            type="number"
            value={draft.logging.retention_days}
            onChange={(event) =>
              setDraft({
                ...draft,
                logging: { ...draft.logging, retention_days: Number(event.currentTarget.value) },
              })
            }
          />
        </label>
        <ToggleField
          checked={draft.privacy.strip_ecs}
          label="Strip ECS"
          onChange={(checked) =>
            setDraft({ ...draft, privacy: { ...draft.privacy, strip_ecs: checked } })
          }
        />
        <ToggleField
          checked={draft.block.use_nxdomain}
          label="NXDOMAIN blocks"
          onChange={(checked) =>
            setDraft({ ...draft, block: { ...draft.block, use_nxdomain: checked } })
          }
        />
        <ToggleField
          checked={draft.privacy.block_local_ptr}
          label="Block local PTR"
          onChange={(checked) =>
            setDraft({ ...draft, privacy: { ...draft.privacy, block_local_ptr: checked } })
          }
        />
        <ToggleField
          checked={draft.logging.query_log}
          label="Query log"
          onChange={(checked) =>
            setDraft({ ...draft, logging: { ...draft.logging, query_log: checked } })
          }
        />
        <ToggleField
          checked={draft.logging.audit_log}
          label="Audit log"
          onChange={(checked) =>
            setDraft({ ...draft, logging: { ...draft.logging, audit_log: checked } })
          }
        />
        <ToggleField
          checked={draft.logging.gzip}
          label="Gzip rotation"
          onChange={(checked) =>
            setDraft({ ...draft, logging: { ...draft.logging, gzip: checked } })
          }
        />
      </div>
    </form>
  );
}

function GroupsPanel({
  groups,
  clients,
  blocklists,
  onChanged,
}: {
  groups: Group[];
  clients: Client[];
  blocklists: Blocklist[];
  onChanged: () => void;
}) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [editing, setEditing] = useState<
    Record<string, { name: string; blocklists: string[]; allowlist: string; schedules: Schedule[] }>
  >({});

  function draftFor(group: Group) {
    return (
      editing[group.name] ?? {
        name: group.name,
        blocklists: group.blocklists,
        allowlist: group.allowlist.join(", "),
        schedules: cloneSchedules(group.schedules),
      }
    );
  }

  function updateDraft(
    nameValue: string,
    patch: Partial<{ name: string; blocklists: string[]; allowlist: string; schedules: Schedule[] }>,
  ) {
    const group = groups.find((candidate) => candidate.name === nameValue);
    setEditing((current) => ({
      ...current,
      [nameValue]: {
        name: current[nameValue]?.name ?? group?.name ?? nameValue,
        blocklists: current[nameValue]?.blocklists ?? group?.blocklists ?? [],
        allowlist: current[nameValue]?.allowlist ?? group?.allowlist.join(", ") ?? "",
        schedules: current[nameValue]?.schedules ?? cloneSchedules(group?.schedules),
        ...patch,
      },
    }));
  }

  function updateSchedule(group: Group, index: number, patch: Partial<Schedule>) {
    const draft = draftFor(group);
    const schedules = draft.schedules.map((schedule, currentIndex) =>
      currentIndex === index ? { ...schedule, ...patch } : schedule,
    );
    updateDraft(group.name, { schedules });
  }

  function addSchedule(group: Group) {
    const draft = draftFor(group);
    updateDraft(group.name, {
      schedules: [
        ...draft.schedules,
        { name: "new-schedule", days: ["all"], from: "22:00", to: "07:00", block: [] },
      ],
    });
  }

  function removeSchedule(group: Group, index: number) {
    const draft = draftFor(group);
    updateDraft(group.name, {
      schedules: draft.schedules.filter((_, currentIndex) => currentIndex !== index),
    });
  }

  async function add(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const next = name.trim();
    if (next === "") {
      return;
    }
    setBusy(next);
    setError("");
    try {
      await createGroup(next);
      setName("");
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "group create failed");
    } finally {
      setBusy("");
    }
  }

  async function remove(group: Group) {
    setBusy(group.name);
    setError("");
    try {
      await deleteGroup(group.name);
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "group delete failed");
    } finally {
      setBusy("");
    }
  }

  async function save(group: Group) {
    const draft = draftFor(group);
    const nextName = draft.name.trim();
    if (nextName === "") {
      setError("group name is required");
      return;
    }
    setBusy(`save:${group.name}`);
    setError("");
    try {
      await updateGroup(group.name, {
        name: nextName,
        blocklists: draft.blocklists,
        allowlist: splitCSV(draft.allowlist),
        schedules: draft.schedules.map((schedule) => ({
          ...schedule,
          name: schedule.name.trim(),
          days: normalizeList(schedule.days),
          block: normalizeList(schedule.block),
        })),
      });
      setEditing((current) => {
        const next = { ...current };
        delete next[group.name];
        return next;
      });
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "group update failed");
    } finally {
      setBusy("");
    }
  }

  return (
    <section className="border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]">
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-lg font-medium tracking-normal">Groups</h2>
        <span className="font-mono text-sm text-[#637083] dark:text-[#a6b1bd]">{groups.length}</span>
      </div>
      <form className="mt-4 flex gap-2" onSubmit={(event) => void add(event)}>
        <input
          className="h-10 min-w-0 flex-1 rounded border border-[#c8d1dc] bg-transparent px-3 outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
          placeholder="new group"
          value={name}
          onChange={(event) => setName(event.currentTarget.value)}
        />
        <button
          className="rounded bg-[#287d7d] px-4 py-2 text-sm font-medium text-white hover:bg-[#216b6b] disabled:cursor-not-allowed disabled:opacity-60"
          disabled={name.trim() === "" || busy !== ""}
          type="submit"
        >
          Add
        </button>
      </form>
      {error !== "" ? <p className="mt-3 text-sm text-[#a33a3a]">{error}</p> : null}
      <ul className="mt-4 grid gap-3">
        {groups.map((group) => {
          const clientCount = clients.filter((client) => client.group === group.name).length;
          const protectedGroup = group.name === "default" || clientCount > 0;
          const draft = draftFor(group);
          const allowlist = splitCSV(draft.allowlist);
          const schedules = draft.schedules;
          const changed =
            draft.name !== group.name ||
            draft.blocklists.join("\n") !== group.blocklists.join("\n") ||
            allowlist.join("\n") !== group.allowlist.join("\n") ||
            JSON.stringify(schedules) !== JSON.stringify(group.schedules ?? []);
          return (
            <li
              key={group.name}
              className="grid gap-3 border-b border-[#edf1f5] py-3 last:border-b-0 dark:border-[#202932]"
            >
              <div className="flex flex-wrap items-center justify-between gap-3">
                <span className="min-w-0">
                  <span className="block font-mono text-sm">{group.name}</span>
                  <span className="block text-xs text-[#637083] dark:text-[#a6b1bd]">
                    {clientCount} clients · {draft.blocklists.length} blocklists ·{" "}
                    {allowlist.length} allow rules · {schedules.length} schedules
                  </span>
                </span>
                <div className="flex gap-2">
                  <button
                    className="rounded border border-[#c8d1dc] px-2 py-1 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                    disabled={!changed || busy !== ""}
                    type="button"
                    onClick={() =>
                      setEditing((current) => {
                        const next = { ...current };
                        delete next[group.name];
                        return next;
                      })
                    }
                  >
                    Reset
                  </button>
                  <button
                    className="rounded border border-[#c8d1dc] px-2 py-1 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                    disabled={!changed || busy !== ""}
                    type="button"
                    onClick={() => void save(group)}
                  >
                    {busy === `save:${group.name}` ? "Saving" : "Save"}
                  </button>
                  <button
                    className="rounded border border-[#c8d1dc] px-2 py-1 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                    disabled={protectedGroup || busy !== ""}
                    type="button"
                    onClick={() => void remove(group)}
                  >
                    Delete
                  </button>
                </div>
              </div>
              <div className="grid gap-3 lg:grid-cols-[180px_1fr_1fr]">
                <label className="grid gap-1 text-sm">
                  <span className="text-[#637083] dark:text-[#a6b1bd]">Name</span>
                  <input
                    className="h-9 rounded border border-[#c8d1dc] bg-transparent px-2 font-mono outline-none focus:border-[#287d7d] disabled:opacity-60 dark:border-[#3a4654]"
                    disabled={group.name === "default"}
                    value={draft.name}
                    onChange={(event) =>
                      updateDraft(group.name, { name: event.currentTarget.value })
                    }
                  />
                </label>
                <label className="grid gap-1 text-sm">
                  <span className="text-[#637083] dark:text-[#a6b1bd]">Allowlist</span>
                  <input
                    className="h-9 rounded border border-[#c8d1dc] bg-transparent px-2 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                    placeholder="example.com, *.lan"
                    value={draft.allowlist}
                    onChange={(event) =>
                      updateDraft(group.name, { allowlist: event.currentTarget.value })
                    }
                  />
                </label>
                <div className="grid gap-1 text-sm">
                  <span className="text-[#637083] dark:text-[#a6b1bd]">Blocklists</span>
                  <div className="flex min-h-9 flex-wrap gap-2">
                    {blocklists.map((blocklist) => {
                      const checked = draft.blocklists.includes(blocklist.id);
                      const nextBlocklists = checked
                        ? draft.blocklists.filter((idValue) => idValue !== blocklist.id)
                        : [...draft.blocklists, blocklist.id];
                      return (
                        <label
                          key={blocklist.id}
                          className="flex h-9 items-center gap-2 rounded border border-[#c8d1dc] px-2 text-xs dark:border-[#3a4654]"
                        >
                          <input
                            className="h-3.5 w-3.5 accent-[#287d7d]"
                            checked={checked}
                            type="checkbox"
                            onChange={() =>
                              updateDraft(group.name, { blocklists: nextBlocklists })
                            }
                          />
                          <span className="font-mono">{blocklist.id}</span>
                        </label>
                      );
                    })}
                    {blocklists.length === 0 ? (
                      <span className="self-center text-sm text-[#637083] dark:text-[#a6b1bd]">
                        No blocklists
                      </span>
                    ) : null}
                  </div>
                </div>
                </div>
                <div className="grid gap-2">
                  <div className="flex items-center justify-between gap-3">
                    <span className="text-sm text-[#637083] dark:text-[#a6b1bd]">Schedules</span>
                    <button
                      className="rounded border border-[#c8d1dc] px-2 py-1 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                      disabled={busy !== ""}
                      type="button"
                      onClick={() => addSchedule(group)}
                    >
                      Add schedule
                    </button>
                  </div>
                  {schedules.map((schedule, index) => (
                    <div
                      key={`${group.name}-${index}`}
                      className="grid gap-3 rounded border border-[#edf1f5] p-3 dark:border-[#202932]"
                    >
                      <div className="grid gap-3 lg:grid-cols-[1fr_120px_120px_1fr_auto]">
                        <label className="grid gap-1 text-sm">
                          <span className="text-[#637083] dark:text-[#a6b1bd]">Schedule</span>
                          <input
                            className="h-9 rounded border border-[#c8d1dc] bg-transparent px-2 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                            value={schedule.name}
                            onChange={(event) =>
                              updateSchedule(group, index, { name: event.currentTarget.value })
                            }
                          />
                        </label>
                        <label className="grid gap-1 text-sm">
                          <span className="text-[#637083] dark:text-[#a6b1bd]">From</span>
                          <input
                            className="h-9 rounded border border-[#c8d1dc] bg-transparent px-2 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                            type="time"
                            value={schedule.from}
                            onChange={(event) =>
                              updateSchedule(group, index, { from: event.currentTarget.value })
                            }
                          />
                        </label>
                        <label className="grid gap-1 text-sm">
                          <span className="text-[#637083] dark:text-[#a6b1bd]">To</span>
                          <input
                            className="h-9 rounded border border-[#c8d1dc] bg-transparent px-2 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                            type="time"
                            value={schedule.to}
                            onChange={(event) =>
                              updateSchedule(group, index, { to: event.currentTarget.value })
                            }
                          />
                        </label>
                        <label className="grid gap-1 text-sm">
                          <span className="text-[#637083] dark:text-[#a6b1bd]">Days</span>
                          <input
                            className="h-9 rounded border border-[#c8d1dc] bg-transparent px-2 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                            value={schedule.days.join(", ")}
                            onChange={(event) =>
                              updateSchedule(group, index, { days: splitCSV(event.currentTarget.value) })
                            }
                          />
                        </label>
                        <button
                          className="self-end rounded border border-[#c8d1dc] px-2 py-1 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                          disabled={busy !== ""}
                          type="button"
                          onClick={() => removeSchedule(group, index)}
                        >
                          Delete
                        </button>
                      </div>
                      <div className="flex min-h-9 flex-wrap gap-2">
                        {blocklists.map((blocklist) => {
                          const checked = schedule.block.includes(blocklist.id);
                          const nextBlock = checked
                            ? schedule.block.filter((idValue) => idValue !== blocklist.id)
                            : [...schedule.block, blocklist.id];
                          return (
                            <label
                              key={blocklist.id}
                              className="flex h-9 items-center gap-2 rounded border border-[#c8d1dc] px-2 text-xs dark:border-[#3a4654]"
                            >
                              <input
                                className="h-3.5 w-3.5 accent-[#287d7d]"
                                checked={checked}
                                type="checkbox"
                                onChange={() => updateSchedule(group, index, { block: nextBlock })}
                              />
                              <span className="font-mono">{blocklist.id}</span>
                            </label>
                          );
                        })}
                      </div>
                    </div>
                  ))}
                  {schedules.length === 0 ? (
                    <p className="text-sm text-[#637083] dark:text-[#a6b1bd]">No schedules</p>
                  ) : null}
                </div>
              </li>
            );
          })}
      </ul>
    </section>
  );
}

function cloneSchedules(schedules: Schedule[] | undefined): Schedule[] {
  return (schedules ?? []).map((schedule) => ({
    name: schedule.name,
    days: [...schedule.days],
    from: schedule.from,
    to: schedule.to,
    block: [...schedule.block],
  }));
}

function normalizeList(values: string[]) {
  return values.map((value) => value.trim()).filter(Boolean);
}

function BlocklistsPanel({
  blocklists,
  groups,
  onChanged,
}: {
  blocklists: Blocklist[];
  groups: Group[];
  onChanged: () => void;
}) {
  const [id, setID] = useState("");
  const [name, setName] = useState("");
  const [url, setURL] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [selectedID, setSelectedID] = useState("");
  const [entryQuery, setEntryQuery] = useState("");
  const [entries, setEntries] = useState<string[]>([]);
  const [entriesError, setEntriesError] = useState("");
  const [editing, setEditing] = useState<
    Record<string, Pick<Blocklist, "name" | "url" | "enabled" | "refresh_interval">>
  >({});

  function draftFor(blocklist: Blocklist) {
    return (
      editing[blocklist.id] ?? {
        name: blocklist.name,
        url: blocklist.url,
        enabled: blocklist.enabled,
        refresh_interval: blocklist.refresh_interval || "24h",
      }
    );
  }

  function updateDraft(
    idValue: string,
    patch: Partial<Pick<Blocklist, "name" | "url" | "enabled" | "refresh_interval">>,
  ) {
    const blocklist = blocklists.find((candidate) => candidate.id === idValue);
    setEditing((current) => ({
      ...current,
      [idValue]: {
        name: current[idValue]?.name ?? blocklist?.name ?? idValue,
        url: current[idValue]?.url ?? blocklist?.url ?? "",
        enabled: current[idValue]?.enabled ?? blocklist?.enabled ?? true,
        refresh_interval:
          current[idValue]?.refresh_interval ?? blocklist?.refresh_interval ?? "24h",
        ...patch,
      },
    }));
  }

  async function add(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const nextID = id.trim();
    const nextURL = url.trim();
    if (nextID === "" || nextURL === "") {
      return;
    }
    setBusy(nextID);
    setError("");
    try {
      await createBlocklist({ id: nextID, name: name.trim() || nextID, url: nextURL });
      setID("");
      setName("");
      setURL("");
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "blocklist create failed");
    } finally {
      setBusy("");
    }
  }

  async function run(idValue: string, action: "sync" | "delete") {
    setBusy(`${action}:${idValue}`);
    setError("");
    try {
      if (action === "sync") {
        await syncBlocklist(idValue);
      } else {
        await deleteBlocklist(idValue);
      }
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : `${action} failed`);
    } finally {
      setBusy("");
    }
  }

  async function save(blocklist: Blocklist) {
    const draft = draftFor(blocklist);
    if (draft.url.trim() === "" || draft.refresh_interval.trim() === "") {
      setError("url and refresh interval are required");
      return;
    }
    setBusy(`save:${blocklist.id}`);
    setError("");
    try {
      await updateBlocklist(blocklist.id, {
        name: draft.name.trim() || blocklist.id,
        url: draft.url.trim(),
        enabled: draft.enabled,
        refresh_interval: draft.refresh_interval.trim(),
      });
      setEditing((current) => {
        const next = { ...current };
        delete next[blocklist.id];
        return next;
      });
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "blocklist update failed");
    } finally {
      setBusy("");
    }
  }

  async function inspect(idValue: string) {
    setBusy(`entries:${idValue}`);
    setEntriesError("");
    setSelectedID(idValue);
    try {
      const result = await listBlocklistEntries(idValue, entryQuery);
      setEntries(result.entries);
    } catch (err) {
      setEntries([]);
      setEntriesError(err instanceof ApiError ? err.body.trim() : "entry lookup failed");
    } finally {
      setBusy("");
    }
  }

  return (
    <section className="border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]">
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-lg font-medium tracking-normal">Blocklists</h2>
        <span className="font-mono text-sm text-[#637083] dark:text-[#a6b1bd]">
          {blocklists.length}
        </span>
      </div>
      <form className="mt-4 grid gap-3 lg:grid-cols-[160px_1fr_2fr_auto]" onSubmit={(event) => void add(event)}>
        <input
          className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
          placeholder="id"
          value={id}
          onChange={(event) => setID(event.currentTarget.value)}
        />
        <input
          className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
          placeholder="name"
          value={name}
          onChange={(event) => setName(event.currentTarget.value)}
        />
        <input
          className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
          placeholder="https://example.com/list.txt"
          value={url}
          onChange={(event) => setURL(event.currentTarget.value)}
        />
        <button
          className="rounded bg-[#287d7d] px-4 py-2 text-sm font-medium text-white hover:bg-[#216b6b] disabled:cursor-not-allowed disabled:opacity-60"
          disabled={id.trim() === "" || url.trim() === "" || busy !== ""}
          type="submit"
        >
          Add
        </button>
      </form>
      {error !== "" ? <p className="mt-3 text-sm text-[#a33a3a]">{error}</p> : null}
      <div className="mt-4 grid gap-3">
        {blocklists.map((blocklist) => {
          const referenced = groups.some((group) => group.blocklists.includes(blocklist.id));
          const draft = draftFor(blocklist);
          const changed =
            draft.name !== blocklist.name ||
            draft.url !== blocklist.url ||
            draft.enabled !== blocklist.enabled ||
            draft.refresh_interval !== (blocklist.refresh_interval || "24h");
          return (
            <article
              key={blocklist.id}
              className="grid gap-3 border-b border-[#edf1f5] pb-3 last:border-b-0 last:pb-0 dark:border-[#202932]"
            >
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="font-medium">{blocklist.id}</h3>
                  <span className="rounded border border-[#c8d1dc] px-2 py-0.5 font-mono text-xs dark:border-[#3a4654]">
                    {draft.enabled ? "enabled" : "disabled"}
                  </span>
                </div>
                <div className="mt-3 grid gap-3 lg:grid-cols-[1fr_2fr_150px_120px]">
                  <label className="grid gap-1 text-sm">
                    <span className="text-[#637083] dark:text-[#a6b1bd]">Name</span>
                    <input
                      className="h-9 rounded border border-[#c8d1dc] bg-transparent px-2 outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                      value={draft.name}
                      onChange={(event) =>
                        updateDraft(blocklist.id, { name: event.currentTarget.value })
                      }
                    />
                  </label>
                  <label className="grid gap-1 text-sm">
                    <span className="text-[#637083] dark:text-[#a6b1bd]">URL</span>
                    <input
                      className="h-9 rounded border border-[#c8d1dc] bg-transparent px-2 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                      value={draft.url}
                      onChange={(event) =>
                        updateDraft(blocklist.id, { url: event.currentTarget.value })
                      }
                    />
                  </label>
                  <label className="grid gap-1 text-sm">
                    <span className="text-[#637083] dark:text-[#a6b1bd]">Refresh</span>
                    <input
                      className="h-9 rounded border border-[#c8d1dc] bg-transparent px-2 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                      value={draft.refresh_interval}
                      onChange={(event) =>
                        updateDraft(blocklist.id, {
                          refresh_interval: event.currentTarget.value,
                        })
                      }
                    />
                  </label>
                  <ToggleField
                    checked={draft.enabled}
                    label="Enabled"
                    onChange={(checked) => updateDraft(blocklist.id, { enabled: checked })}
                  />
                </div>
              </div>
              <div className="flex gap-2">
                <button
                  className="h-9 rounded border border-[#c8d1dc] px-3 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                  disabled={!changed || busy !== ""}
                  type="button"
                  onClick={() =>
                    setEditing((current) => {
                      const next = { ...current };
                      delete next[blocklist.id];
                      return next;
                    })
                  }
                >
                  Reset
                </button>
                <button
                  className="h-9 rounded border border-[#c8d1dc] px-3 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                  disabled={!changed || busy !== ""}
                  type="button"
                  onClick={() => void save(blocklist)}
                >
                  {busy === `save:${blocklist.id}` ? "Saving" : "Save"}
                </button>
                <button
                  className="h-9 rounded border border-[#c8d1dc] px-3 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                  disabled={busy !== ""}
                  type="button"
                  onClick={() => void inspect(blocklist.id)}
                >
                  {busy === `entries:${blocklist.id}` ? "Loading" : "Inspect"}
                </button>
                <button
                  className="h-9 rounded border border-[#c8d1dc] px-3 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                  disabled={busy !== ""}
                  type="button"
                  onClick={() => void run(blocklist.id, "sync")}
                >
                  {busy === `sync:${blocklist.id}` ? "Syncing" : "Sync"}
                </button>
                <button
                  className="h-9 rounded border border-[#c8d1dc] px-3 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                  disabled={referenced || busy !== ""}
                  type="button"
                  onClick={() => void run(blocklist.id, "delete")}
                >
                  Delete
                </button>
              </div>
            </article>
          );
        })}
        {blocklists.length === 0 ? (
          <p className="text-sm text-[#637083] dark:text-[#a6b1bd]">No blocklists</p>
        ) : null}
      </div>
      {selectedID !== "" ? (
        <div className="mt-5 rounded border border-[#d8dee6] bg-[#f7f9fb] p-4 dark:border-[#2c3540] dark:bg-[#101418]">
          <form
            className="grid gap-3 md:grid-cols-[1fr_auto]"
            onSubmit={(event) => {
              event.preventDefault();
              void inspect(selectedID);
            }}
          >
            <label className="grid gap-1 text-sm">
              <span className="text-[#637083] dark:text-[#a6b1bd]">
                Entries in <span className="font-mono">{selectedID}</span>
              </span>
              <input
                className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                placeholder="filter domains"
                value={entryQuery}
                onChange={(event) => setEntryQuery(event.currentTarget.value)}
              />
            </label>
            <button
              className="self-end rounded bg-[#287d7d] px-4 py-2 text-sm font-medium text-white hover:bg-[#216b6b] disabled:cursor-not-allowed disabled:opacity-60"
              disabled={busy !== ""}
              type="submit"
            >
              Search
            </button>
          </form>
          {entriesError !== "" ? (
            <p className="mt-3 text-sm text-[#a33a3a]">{entriesError}</p>
          ) : (
            <ul className="mt-3 grid max-h-56 gap-1 overflow-auto font-mono text-sm">
              {entries.map((entry) => (
                <li
                  key={entry}
                  className="border-b border-[#d8dee6] py-1 last:border-b-0 dark:border-[#2c3540]"
                >
                  {entry}
                </li>
              ))}
              {entries.length === 0 ? (
                <li className="py-1 text-[#637083] dark:text-[#a6b1bd]">No entries</li>
              ) : null}
            </ul>
          )}
        </div>
      ) : null}
    </section>
  );
}

function ToggleField({
  checked,
  label,
  onChange,
}: {
  checked: boolean;
  label: string;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="flex h-10 items-center gap-3 text-sm">
      <input
        className="h-4 w-4 accent-[#287d7d]"
        checked={checked}
        type="checkbox"
        onChange={(event) => onChange(event.currentTarget.checked)}
      />
      <span>{label}</span>
    </label>
  );
}

function UpstreamPanel({
  upstreams,
  onChanged,
}: {
  upstreams: Upstream[];
  onChanged: () => void;
}) {
  const [busy, setBusy] = useState("");
  const [results, setResults] = useState<Record<string, UpstreamTestResult>>({});
  const [error, setError] = useState("");
  const [id, setID] = useState("");
  const [name, setName] = useState("");
  const [url, setURL] = useState("");
  const [bootstrap, setBootstrap] = useState("");
  const [editing, setEditing] = useState<
    Record<string, { name: string; url: string; bootstrap: string }>
  >({});

  function draftFor(upstream: Upstream) {
    return (
      editing[upstream.id] ?? {
        name: upstream.name,
        url: upstream.url,
        bootstrap: upstream.bootstrap.join(", "),
      }
    );
  }

  function updateDraft(
    idValue: string,
    patch: Partial<{ name: string; url: string; bootstrap: string }>,
  ) {
    const upstream = upstreams.find((candidate) => candidate.id === idValue);
    setEditing((current) => ({
      ...current,
      [idValue]: {
        name: current[idValue]?.name ?? upstream?.name ?? idValue,
        url: current[idValue]?.url ?? upstream?.url ?? "",
        bootstrap: current[idValue]?.bootstrap ?? upstream?.bootstrap.join(", ") ?? "",
        ...patch,
      },
    }));
  }

  async function add(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const nextID = id.trim();
    const nextURL = url.trim();
    const nextBootstrap = bootstrap
      .split(",")
      .map((part) => part.trim())
      .filter(Boolean);
    if (nextID === "" || nextURL === "" || nextBootstrap.length === 0) {
      return;
    }
    setBusy(`create:${nextID}`);
    setError("");
    try {
      await createUpstream({
        id: nextID,
        name: name.trim() || nextID,
        url: nextURL,
        bootstrap: nextBootstrap,
      });
      setID("");
      setName("");
      setURL("");
      setBootstrap("");
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "upstream create failed");
    } finally {
      setBusy("");
    }
  }

  async function runTest(id: string) {
    setBusy(`test:${id}`);
    setError("");
    try {
      const result = await testUpstream(id);
      setResults((current) => ({ ...current, [id]: result }));
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "upstream test failed");
    } finally {
      setBusy("");
    }
  }

  async function save(upstream: Upstream) {
    const draft = draftFor(upstream);
    const nextBootstrap = draft.bootstrap
      .split(",")
      .map((part) => part.trim())
      .filter(Boolean);
    if (draft.url.trim() === "" || nextBootstrap.length === 0) {
      setError("url and bootstrap are required");
      return;
    }
    setBusy(`save:${upstream.id}`);
    setError("");
    try {
      await updateUpstream(upstream.id, {
        name: draft.name.trim() || upstream.id,
        url: draft.url.trim(),
        bootstrap: nextBootstrap,
      });
      setEditing((current) => {
        const next = { ...current };
        delete next[upstream.id];
        return next;
      });
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "upstream update failed");
    } finally {
      setBusy("");
    }
  }

  async function remove(idValue: string) {
    setBusy(`delete:${idValue}`);
    setError("");
    try {
      await deleteUpstream(idValue);
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "upstream delete failed");
    } finally {
      setBusy("");
    }
  }

  return (
    <section className="border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]">
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-lg font-medium tracking-normal">Upstreams</h2>
        <span className="font-mono text-sm text-[#637083] dark:text-[#a6b1bd]">
          {upstreams.length}
        </span>
      </div>
      <form className="mt-4 grid gap-3 lg:grid-cols-[140px_1fr_2fr_1.5fr_auto]" onSubmit={(event) => void add(event)}>
        <input
          className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
          placeholder="id"
          value={id}
          onChange={(event) => setID(event.currentTarget.value)}
        />
        <input
          className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
          placeholder="name"
          value={name}
          onChange={(event) => setName(event.currentTarget.value)}
        />
        <input
          className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
          placeholder="https://dns.example/dns-query"
          value={url}
          onChange={(event) => setURL(event.currentTarget.value)}
        />
        <input
          className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
          placeholder="1.1.1.1, 1.0.0.1"
          value={bootstrap}
          onChange={(event) => setBootstrap(event.currentTarget.value)}
        />
        <button
          className="rounded bg-[#287d7d] px-4 py-2 text-sm font-medium text-white hover:bg-[#216b6b] disabled:cursor-not-allowed disabled:opacity-60"
          disabled={id.trim() === "" || url.trim() === "" || bootstrap.trim() === "" || busy !== ""}
          type="submit"
        >
          Add
        </button>
      </form>
      {error !== "" ? <p className="mt-3 text-sm text-[#a33a3a]">{error}</p> : null}
      <div className="mt-4 grid gap-3">
        {upstreams.map((upstream) => {
          const result = results[upstream.id];
          const draft = draftFor(upstream);
          const changed =
            draft.name !== upstream.name ||
            draft.url !== upstream.url ||
            draft.bootstrap !== upstream.bootstrap.join(", ");
          return (
            <article
              key={upstream.id}
              className="grid gap-3 border-b border-[#edf1f5] pb-3 last:border-b-0 last:pb-0 dark:border-[#202932]"
            >
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="font-medium">{upstream.id}</h3>
                  <span
                    className={
                      upstream.healthy
                        ? "rounded bg-[#dff3ec] px-2 py-0.5 text-xs text-[#1d6a4f] dark:bg-[#18362e] dark:text-[#9ce0c8]"
                        : "rounded bg-[#f5e1e1] px-2 py-0.5 text-xs text-[#9a3131] dark:bg-[#3a2020] dark:text-[#f0b0b0]"
                    }
                  >
                    {upstream.healthy ? "healthy" : "unhealthy"}
                  </span>
                </div>
                <div className="mt-3 grid gap-3 lg:grid-cols-[1fr_2fr_1.5fr]">
                  <label className="grid gap-1 text-sm">
                    <span className="text-[#637083] dark:text-[#a6b1bd]">Name</span>
                    <input
                      className="h-9 rounded border border-[#c8d1dc] bg-transparent px-2 outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                      value={draft.name}
                      onChange={(event) =>
                        updateDraft(upstream.id, { name: event.currentTarget.value })
                      }
                    />
                  </label>
                  <label className="grid gap-1 text-sm">
                    <span className="text-[#637083] dark:text-[#a6b1bd]">URL</span>
                    <input
                      className="h-9 rounded border border-[#c8d1dc] bg-transparent px-2 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                      value={draft.url}
                      onChange={(event) =>
                        updateDraft(upstream.id, { url: event.currentTarget.value })
                      }
                    />
                  </label>
                  <label className="grid gap-1 text-sm">
                    <span className="text-[#637083] dark:text-[#a6b1bd]">Bootstrap</span>
                    <input
                      className="h-9 rounded border border-[#c8d1dc] bg-transparent px-2 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                      value={draft.bootstrap}
                      onChange={(event) =>
                        updateDraft(upstream.id, { bootstrap: event.currentTarget.value })
                      }
                    />
                  </label>
                </div>
                {result ? (
                  <p className="mt-2 font-mono text-xs text-[#354252] dark:text-[#c3ccd6]">
                    rcode {result.rcode} · {result.answers} answers · {result.latency_us} us
                  </p>
                ) : null}
              </div>
              <div className="flex gap-2">
                <button
                  className="h-9 rounded border border-[#c8d1dc] px-3 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                  disabled={!changed || busy !== ""}
                  type="button"
                  onClick={() =>
                    setEditing((current) => {
                      const next = { ...current };
                      delete next[upstream.id];
                      return next;
                    })
                  }
                >
                  Reset
                </button>
                <button
                  className="h-9 rounded border border-[#c8d1dc] px-3 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                  disabled={!changed || busy !== ""}
                  type="button"
                  onClick={() => void save(upstream)}
                >
                  {busy === `save:${upstream.id}` ? "Saving" : "Save"}
                </button>
                <button
                  className="h-9 rounded border border-[#c8d1dc] px-3 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                  disabled={busy !== ""}
                  type="button"
                  onClick={() => void runTest(upstream.id)}
                >
                  {busy === `test:${upstream.id}` ? "Testing" : "Test"}
                </button>
                <button
                  className="h-9 rounded border border-[#c8d1dc] px-3 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                  disabled={busy !== "" || upstreams.length <= 1}
                  type="button"
                  onClick={() => void remove(upstream.id)}
                >
                  Delete
                </button>
              </div>
            </article>
          );
        })}
        {upstreams.length === 0 ? (
          <p className="text-sm text-[#637083] dark:text-[#a6b1bd]">No upstreams</p>
        ) : null}
      </div>
    </section>
  );
}

function StatsCards({ stats }: { stats: StatsSummary }) {
  const items = [
    { label: "Queries", value: stats.query_total },
    { label: "Blocked", value: stats.blocked_total },
    { label: "Cache hits", value: stats.cache_hit },
    { label: "Cache misses", value: stats.cache_miss },
    { label: "Rate limited", value: stats.rate_limited_total },
    { label: "Malformed", value: stats.malformed_total },
  ];
  return (
    <section>
      <h2 className="text-lg font-medium tracking-normal">Live Summary</h2>
      <div className="mt-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-6">
        {items.map((item) => (
          <article
            key={item.label}
            className="rounded border border-[#d8dee6] bg-white p-4 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]"
          >
            <p className="text-sm text-[#637083] dark:text-[#a6b1bd]">{item.label}</p>
            <p className="mt-2 font-mono text-xl">{item.value.toLocaleString()}</p>
          </article>
        ))}
      </div>
    </section>
  );
}

function PolicyLists({
  allowlist,
  customBlocklist,
  onChanged,
}: {
  allowlist: string[];
  customBlocklist: string[];
  onChanged: () => void;
}) {
  return (
    <section className="grid gap-4 lg:grid-cols-2">
      <DomainListPanel kind="allow" title="Allowlist" domains={allowlist} onChanged={onChanged} />
      <DomainListPanel
        kind="block"
        title="Custom Blocklist"
        domains={customBlocklist}
        onChanged={onChanged}
      />
    </section>
  );
}

function DomainListPanel({
  kind,
  title,
  domains,
  onChanged,
}: {
  kind: "allow" | "block";
  title: string;
  domains: string[];
  onChanged: () => void;
}) {
  const [domain, setDomain] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const shown = domains.slice(0, 8);

  async function add(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const next = domain.trim();
    if (next === "") {
      return;
    }
    setBusy(next);
    setError("");
    try {
      await addDomainListEntry(kind, next);
      setDomain("");
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "add failed");
    } finally {
      setBusy("");
    }
  }

  async function remove(value: string) {
    setBusy(value);
    setError("");
    try {
      await removeDomainListEntry(kind, value);
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "remove failed");
    } finally {
      setBusy("");
    }
  }

  return (
    <section className="border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]">
      <div className="flex items-center justify-between gap-4">
        <h2 className="text-lg font-medium tracking-normal">{title}</h2>
        <span className="font-mono text-sm text-[#637083] dark:text-[#a6b1bd]">{domains.length}</span>
      </div>
      <form className="mt-4 flex gap-2" onSubmit={(event) => void add(event)}>
        <input
          className="h-10 min-w-0 flex-1 rounded border border-[#c8d1dc] bg-transparent px-3 outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
          placeholder="example.com"
          value={domain}
          onChange={(event) => setDomain(event.currentTarget.value)}
        />
        <button
          className="rounded bg-[#287d7d] px-4 py-2 text-sm font-medium text-white hover:bg-[#216b6b] disabled:cursor-not-allowed disabled:opacity-60"
          disabled={domain.trim() === "" || busy !== ""}
          type="submit"
        >
          Add
        </button>
      </form>
      {error !== "" ? <p className="mt-3 text-sm text-[#a33a3a]">{error}</p> : null}
      <ul className="mt-4 grid gap-2">
        {shown.map((value) => (
          <li
            key={value}
            className="flex items-center justify-between gap-3 border-b border-[#edf1f5] py-2 dark:border-[#202932]"
          >
            <span className="min-w-0 truncate font-mono text-sm">{value}</span>
            <button
              className="rounded border border-[#c8d1dc] px-2 py-1 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
              disabled={busy === value}
              type="button"
              onClick={() => void remove(value)}
            >
              Remove
            </button>
          </li>
        ))}
        {shown.length === 0 ? (
          <li className="py-2 text-sm text-[#637083] dark:text-[#a6b1bd]">No entries</li>
        ) : null}
      </ul>
    </section>
  );
}

function ClientTable({
  clients,
  groups,
  onChanged,
}: {
  clients: Client[];
  groups: Group[];
  onChanged: () => void;
}) {
  const [query, setQuery] = useState("");
  const [groupFilter, setGroupFilter] = useState("all");
  const [visibilityFilter, setVisibilityFilter] = useState<"all" | "visible" | "hidden">("all");
  const [editing, setEditing] = useState<Record<string, Pick<Client, "name" | "group">>>({});
  const [savingKey, setSavingKey] = useState("");
  const [error, setError] = useState("");
  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return clients.filter((client) => {
      const matchesQuery =
        needle === "" ||
        client.key.toLowerCase().includes(needle) ||
        client.name.toLowerCase().includes(needle) ||
        client.last_ip.toLowerCase().includes(needle) ||
        client.type.toLowerCase().includes(needle);
      const matchesGroup = groupFilter === "all" || (client.group || "default") === groupFilter;
      const matchesVisibility =
        visibilityFilter === "all" ||
        (visibilityFilter === "hidden" ? client.hidden : !client.hidden);
      return matchesQuery && matchesGroup && matchesVisibility;
    });
  }, [clients, groupFilter, query, visibilityFilter]);
  const shown = filtered.slice(0, 24);

  function draftFor(client: Client) {
    return editing[client.key] ?? { name: client.name, group: client.group || "default" };
  }

  function updateDraft(key: string, patch: Partial<Pick<Client, "name" | "group">>) {
    setEditing((current) => ({
      ...current,
      [key]: { name: current[key]?.name ?? "", group: current[key]?.group ?? "default", ...patch },
    }));
  }

  async function save(client: Client) {
    const draft = draftFor(client);
    setSavingKey(client.key);
    setError("");
    try {
      await updateClient(client.key, draft);
      setEditing((current) => {
        const next = { ...current };
        delete next[client.key];
        return next;
      });
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "save failed");
    } finally {
      setSavingKey("");
    }
  }

  async function setHidden(client: Client, hidden: boolean) {
    setSavingKey(client.key);
    setError("");
    try {
      await updateClient(client.key, { hidden });
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "visibility update failed");
    } finally {
      setSavingKey("");
    }
  }

  async function forget(client: Client) {
    if (!window.confirm(`Forget ${client.name || client.key}?`)) {
      return;
    }
    setSavingKey(client.key);
    setError("");
    try {
      await deleteClient(client.key);
      onChanged();
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "forget failed");
    } finally {
      setSavingKey("");
    }
  }

  return (
    <section className="border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <h2 className="text-lg font-medium tracking-normal">Clients</h2>
        <span className="font-mono text-sm text-[#637083] dark:text-[#a6b1bd]">
          {filtered.length} / {clients.length}
        </span>
      </div>
      <div className="mt-4 grid gap-3 lg:grid-cols-[1fr_180px_150px]">
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Search</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            placeholder="name, key, IP, type"
            value={query}
            onChange={(event) => setQuery(event.currentTarget.value)}
          />
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Group</span>
          <select
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            value={groupFilter}
            onChange={(event) => setGroupFilter(event.currentTarget.value)}
          >
            <option value="all">all</option>
            {groups.map((group) => (
              <option key={group.name} value={group.name}>
                {group.name}
              </option>
            ))}
          </select>
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Status</span>
          <select
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            value={visibilityFilter}
            onChange={(event) =>
              setVisibilityFilter(event.currentTarget.value as "all" | "visible" | "hidden")
            }
          >
            <option value="all">all</option>
            <option value="visible">visible</option>
            <option value="hidden">hidden</option>
          </select>
        </label>
      </div>
      {error !== "" ? <p className="mt-3 text-sm text-[#a33a3a]">{error}</p> : null}
      <div className="mt-4 overflow-x-auto">
        <table className="w-full min-w-[880px] border-collapse text-left text-sm">
          <thead className="border-b border-[#d8dee6] text-[#637083] dark:border-[#2c3540] dark:text-[#a6b1bd]">
            <tr>
              <th className="py-2 pr-4 font-medium">Client</th>
              <th className="py-2 pr-4 font-medium">Type</th>
              <th className="py-2 pr-4 font-medium">Group</th>
              <th className="py-2 pr-4 font-medium">Status</th>
              <th className="py-2 pr-4 font-medium">Last IP</th>
              <th className="py-2 font-medium">Last seen</th>
              <th className="py-2 pl-4 font-medium">Action</th>
            </tr>
          </thead>
          <tbody>
            {shown.map((client) => {
              const draft = draftFor(client);
              const changed = draft.name !== client.name || draft.group !== client.group;
              return (
                <tr key={client.key} className="border-b border-[#edf1f5] dark:border-[#202932]">
                  <td className="py-3 pr-4">
                    <input
                      className="h-9 w-full min-w-40 rounded border border-[#c8d1dc] bg-transparent px-2 outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                      placeholder={client.key}
                      value={draft.name}
                      onChange={(event) => updateDraft(client.key, { name: event.currentTarget.value })}
                    />
                    <span className="mt-1 block font-mono text-xs text-[#637083] dark:text-[#a6b1bd]">
                      {client.key}
                    </span>
                  </td>
                  <td className="py-3 pr-4 font-mono">{client.type}</td>
                  <td className="py-3 pr-4">
                    <select
                      className="h-9 rounded border border-[#c8d1dc] bg-transparent px-2 font-mono outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
                      value={draft.group}
                      onChange={(event) =>
                        updateDraft(client.key, { group: event.currentTarget.value })
                      }
                    >
                      {groups.map((group) => (
                        <option key={group.name} value={group.name}>
                          {group.name}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td className="py-3 pr-4">
                    <span
                      className={
                        client.hidden
                          ? "rounded bg-[#f2e7d8] px-2 py-0.5 text-xs text-[#7b5624] dark:bg-[#34291d] dark:text-[#e5c99f]"
                          : "rounded bg-[#dff3ec] px-2 py-0.5 text-xs text-[#1d6a4f] dark:bg-[#18362e] dark:text-[#9ce0c8]"
                      }
                    >
                      {client.hidden ? "hidden" : "visible"}
                    </span>
                  </td>
                  <td className="py-3 pr-4 font-mono">{client.last_ip || "-"}</td>
                  <td className="py-3 font-mono">{formatDate(client.last_seen)}</td>
                  <td className="py-3 pl-4">
                    <div className="flex gap-2">
                      <button
                        className="rounded border border-[#c8d1dc] px-3 py-1 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                        disabled={!changed || savingKey === client.key}
                        type="button"
                        onClick={() =>
                          setEditing((current) => {
                            const next = { ...current };
                            delete next[client.key];
                            return next;
                          })
                        }
                      >
                        Reset
                      </button>
                      <button
                        className="rounded border border-[#c8d1dc] px-3 py-1 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                        disabled={!changed || savingKey === client.key}
                        type="button"
                        onClick={() => void save(client)}
                      >
                        {savingKey === client.key ? "Saving" : "Save"}
                      </button>
                      <button
                        className="rounded border border-[#c8d1dc] px-3 py-1 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                        disabled={savingKey === client.key}
                        type="button"
                        onClick={() => void setHidden(client, !client.hidden)}
                      >
                        {client.hidden ? "Show" : "Hide"}
                      </button>
                      <button
                        className="rounded border border-[#c8d1dc] px-3 py-1 text-sm hover:bg-[#ecf1f5] disabled:cursor-not-allowed disabled:opacity-50 dark:border-[#3a4654] dark:hover:bg-[#1d252d]"
                        disabled={savingKey === client.key}
                        type="button"
                        onClick={() => void forget(client)}
                      >
                        Forget
                      </button>
                    </div>
                  </td>
                </tr>
              );
            })}
            {shown.length === 0 ? (
              <tr>
                <td className="py-4 text-[#637083] dark:text-[#a6b1bd]" colSpan={7}>
                  No clients
                </td>
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>
      {filtered.length > shown.length ? (
        <p className="mt-3 text-sm text-[#637083] dark:text-[#a6b1bd]">
          Showing first {shown.length} matches. Narrow the filters to inspect the rest.
        </p>
      ) : null}
    </section>
  );
}

function AuthForm({ mode }: { mode: "login" | "setup" }) {
  const { login, setup } = useAuth();
  const [username, setUsername] = useState(mode === "setup" ? "admin" : "");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      if (mode === "setup") {
        await setup(username, password);
      } else {
        await login(username, password);
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.body.trim() : "request failed");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form
      className="grid gap-4 border border-[#d8dee6] bg-white p-5 shadow-sm dark:border-[#2c3540] dark:bg-[#151b21]"
      onSubmit={(event) => void submit(event)}
    >
      <div>
        <h2 className="text-lg font-medium tracking-normal">
          {mode === "setup" ? "Create Admin" : "Sign In"}
        </h2>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Username</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            autoComplete="username"
            value={username}
            onChange={(event) => setUsername(event.currentTarget.value)}
          />
        </label>
        <label className="grid gap-1 text-sm">
          <span className="text-[#637083] dark:text-[#a6b1bd]">Password</span>
          <input
            className="h-10 rounded border border-[#c8d1dc] bg-transparent px-3 outline-none focus:border-[#287d7d] dark:border-[#3a4654]"
            autoComplete={mode === "setup" ? "new-password" : "current-password"}
            type="password"
            value={password}
            onChange={(event) => setPassword(event.currentTarget.value)}
          />
        </label>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <button
          className="rounded bg-[#287d7d] px-4 py-2 text-sm font-medium text-white hover:bg-[#216b6b] disabled:cursor-not-allowed disabled:opacity-60"
          disabled={submitting || username === "" || password.length < 8}
          type="submit"
        >
          {submitting ? "Working" : mode === "setup" ? "Create" : "Sign in"}
        </button>
        {error !== "" ? <p className="text-sm text-[#a33a3a]">{error}</p> : null}
      </div>
    </form>
  );
}

function authStatusLabel(state: AuthState) {
  switch (state.status) {
    case "authenticated":
      return state.session.username;
    case "setup-required":
      return "setup";
    case "unauthenticated":
      return "signed out";
    case "error":
      return "api error";
    default:
      return "checking";
  }
}

function authDetail(state: AuthState) {
  switch (state.status) {
    case "authenticated":
      return state.session.expires_at;
    case "setup-required":
      return "first-run";
    case "unauthenticated":
      return "none";
    case "error":
      return state.message;
    default:
      return "loading";
  }
}

function formatDate(value: string) {
  if (value === "" || value.startsWith("0001-")) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

function splitCSV(value: string) {
  return value
    .split(",")
    .map((part) => part.trim())
    .filter(Boolean);
}

function formatTime(value: string) {
  if (value === "" || value.startsWith("0001-")) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleTimeString();
}

function formatBucket(value: string) {
  const date = new Date(Number(value));
  if (!Number.isNaN(date.getTime())) {
    return date.toLocaleTimeString();
  }
  return value;
}
