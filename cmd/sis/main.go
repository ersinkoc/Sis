package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"

	"github.com/ersinkoc/sis/internal/api"
	"github.com/ersinkoc/sis/internal/config"
	sisdns "github.com/ersinkoc/sis/internal/dns"
	sislog "github.com/ersinkoc/sis/internal/log"
	"github.com/ersinkoc/sis/internal/policy"
	"github.com/ersinkoc/sis/internal/stats"
	"github.com/ersinkoc/sis/internal/store"
	"github.com/ersinkoc/sis/internal/upstream"
	"github.com/ersinkoc/sis/pkg/version"
	mdns "github.com/miekg/dns"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Println(version.String())
			return
		case "serve":
			if err := runServe(os.Args[2:]); err != nil {
				slog.Error("serve failed", "error", err)
				os.Exit(1)
			}
			return
		case "config":
			if err := runConfig(os.Args[2:]); err != nil {
				slog.Error("config failed", "error", err)
				os.Exit(1)
			}
			return
		case "auth":
			must(runAuth(os.Args[2:]))
			return
		case "user":
			must(runUser(os.Args[2:]))
			return
		case "client":
			must(runClient(os.Args[2:]))
			return
		case "cache":
			must(runCache(os.Args[2:]))
			return
		case "upstream":
			must(runUpstream(os.Args[2:]))
			return
		case "logs":
			must(runLogs(os.Args[2:]))
			return
		case "stats":
			must(runStats(os.Args[2:]))
			return
		case "system":
			must(runSystem(os.Args[2:]))
			return
		case "allowlist":
			must(runAllowlist(os.Args[2:]))
			return
		case "blocklist":
			must(runBlocklist(os.Args[2:]))
			return
		case "group":
			must(runGroup(os.Args[2:]))
			return
		case "query":
			must(runQuery(os.Args[2:]))
			return
		}
	}
	fmt.Fprintf(os.Stderr, "usage: sis <serve|config|version|auth|user|client|cache|upstream|logs|stats|system|allowlist|blocklist|group|query>\n")
	os.Exit(2)
}

func must(err error) {
	if err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func apiFlags(name string, args []string) (*cliClient, []string, error) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	base := fs.String("api", "http://127.0.0.1:8080", "Sis API base URL")
	cookie := fs.String("cookie", "", "Cookie header, for example sis_session=...")
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}
	return newCLIClient(*base, *cookie), fs.Args(), nil
}

func runAuth(args []string) error {
	client, rest, err := apiFlags("auth", args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("usage: sis auth <setup|login|me|logout>")
	}
	switch rest[0] {
	case "setup":
		if len(rest) != 3 {
			return fmt.Errorf("usage: sis auth setup <username> <password>")
		}
		return printResponse(client.post("/api/v1/auth/setup", map[string]string{"username": rest[1], "password": rest[2]}, responseBuffer()))
	case "login":
		if len(rest) != 3 {
			return fmt.Errorf("usage: sis auth login <username> <password>")
		}
		return printResponse(client.post("/api/v1/auth/login", map[string]string{"username": rest[1], "password": rest[2]}, responseBuffer()))
	case "me":
		return printResponse(client.get("/api/v1/auth/me", responseBuffer()))
	case "logout":
		return client.post("/api/v1/auth/logout", nil, io.Discard)
	default:
		return fmt.Errorf("unknown auth command %q", rest[0])
	}
}

func runUser(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: sis user <add|passwd>")
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("user add", flag.ExitOnError)
		path := fs.String("config", defaultConfigPath(), "config file path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		rest := fs.Args()
		if len(rest) != 2 {
			return fmt.Errorf("usage: sis user add [-config path] <username> <password>")
		}
		return upsertConfigUser(*path, rest[0], rest[1], false)
	case "passwd":
		fs := flag.NewFlagSet("user passwd", flag.ExitOnError)
		path := fs.String("config", defaultConfigPath(), "config file path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		rest := fs.Args()
		if len(rest) != 2 {
			return fmt.Errorf("usage: sis user passwd [-config path] <username> <password>")
		}
		return upsertConfigUser(*path, rest[0], rest[1], true)
	default:
		return fmt.Errorf("unknown user command %q", args[0])
	}
}

func upsertConfigUser(path, username, password string, mustExist bool) error {
	username = strings.TrimSpace(username)
	if username == "" || len(password) < 8 {
		return fmt.Errorf("username and password with at least 8 chars are required")
	}
	loader := &config.Loader{Path: path}
	cfg, err := loader.Load()
	if err != nil {
		return err
	}
	hash, err := api.HashPassword(password)
	if err != nil {
		return err
	}
	idx := userIndex(cfg.Auth.Users, username)
	if idx < 0 {
		if mustExist {
			return fmt.Errorf("user %q not found", username)
		}
		cfg.Auth.Users = append(cfg.Auth.Users, config.User{Username: username, PasswordHash: hash})
	} else {
		if !mustExist {
			return fmt.Errorf("user %q already exists", username)
		}
		cfg.Auth.Users[idx].PasswordHash = hash
	}
	cfg.Auth.FirstRun = false
	if err := config.Validate(cfg); err != nil {
		return err
	}
	if err := loader.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("updated user %q in %s\n", username, path)
	return nil
}

func userIndex(users []config.User, username string) int {
	for i, user := range users {
		if user.Username == username {
			return i
		}
	}
	return -1
}

func runClient(args []string) error {
	client, rest, err := apiFlags("client", args)
	if err != nil {
		return err
	}
	if len(rest) == 0 || rest[0] != "list" {
		if len(rest) > 0 {
			switch rest[0] {
			case "get":
				if len(rest) != 2 {
					return fmt.Errorf("usage: sis client get <key>")
				}
				return printResponse(client.get("/api/v1/clients/"+encodedPathPart(rest[1]), responseBuffer()))
			case "rename":
				if len(rest) != 3 {
					return fmt.Errorf("usage: sis client rename <key> <name>")
				}
				return printResponse(client.patch("/api/v1/clients/"+encodedPathPart(rest[1]), map[string]string{"name": rest[2]}, responseBuffer()))
			case "move":
				if len(rest) != 3 {
					return fmt.Errorf("usage: sis client move <key> <group>")
				}
				return printResponse(client.patch("/api/v1/clients/"+encodedPathPart(rest[1]), map[string]string{"group": rest[2]}, responseBuffer()))
			case "forget":
				if len(rest) != 2 {
					return fmt.Errorf("usage: sis client forget <key>")
				}
				return client.delete("/api/v1/clients/"+encodedPathPart(rest[1]), io.Discard)
			}
		}
		return fmt.Errorf("usage: sis client <list|get|rename|move|forget>")
	}
	return printResponse(client.get("/api/v1/clients", responseBuffer()))
}

func runCache(args []string) error {
	client, rest, err := apiFlags("cache", args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("usage: sis cache <flush|stats>")
	}
	switch rest[0] {
	case "flush":
		return printResponse(client.post("/api/v1/system/cache/flush", nil, responseBuffer()))
	case "stats":
		return printResponse(client.get("/api/v1/stats/summary", responseBuffer()))
	default:
		return fmt.Errorf("unknown cache command %q", rest[0])
	}
}

func runUpstream(args []string) error {
	client, rest, err := apiFlags("upstream", args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("usage: sis upstream <health|test>")
	}
	switch rest[0] {
	case "health":
		return printResponse(client.get("/api/v1/upstreams", responseBuffer()))
	case "test":
		if len(rest) != 2 {
			return fmt.Errorf("usage: sis upstream test <id>")
		}
		return printResponse(client.post("/api/v1/upstreams/"+encodedPathPart(rest[1])+"/test", nil, responseBuffer()))
	default:
		return fmt.Errorf("unknown upstream command %q", rest[0])
	}
}

func runLogs(args []string) error {
	client, rest, err := apiFlags("logs", args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("usage: sis logs <list|tail>")
	}
	switch rest[0] {
	case "list":
		if len(rest) > 3 {
			return fmt.Errorf("usage: sis logs list [limit] [qname]")
		}
		path := "/api/v1/logs/query"
		if len(rest) > 1 {
			path += "?limit=" + encodedQuery(rest[1])
			if len(rest) == 3 {
				path += "&qname=" + encodedQuery(rest[2])
			}
		}
		return printResponse(client.get(path, responseBuffer()))
	case "tail":
		return client.get("/api/v1/logs/query/stream", os.Stdout)
	default:
		return fmt.Errorf("unknown logs command %q", rest[0])
	}
}

func runStats(args []string) error {
	client, rest, err := apiFlags("stats", args)
	if err != nil {
		return err
	}
	path := "/api/v1/stats/summary"
	if len(rest) > 0 {
		switch rest[0] {
		case "summary":
			path = "/api/v1/stats/summary"
		case "timeseries":
			path = "/api/v1/stats/timeseries"
		case "top-domains":
			path = "/api/v1/stats/top-domains"
		case "top-clients":
			path = "/api/v1/stats/top-clients"
		default:
			return fmt.Errorf("unknown stats command %q", rest[0])
		}
	}
	return printResponse(client.get(path, responseBuffer()))
}

func runSystem(args []string) error {
	client, rest, err := apiFlags("system", args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("usage: sis system <info|history|reload>")
	}
	switch rest[0] {
	case "info":
		return printResponse(client.get("/api/v1/system/info", responseBuffer()))
	case "history":
		if len(rest) > 2 {
			return fmt.Errorf("usage: sis system history [limit]")
		}
		path := "/api/v1/system/config/history"
		if len(rest) == 2 {
			path += "?limit=" + encodedQuery(rest[1])
		}
		return printResponse(client.get(path, responseBuffer()))
	case "reload":
		return printResponse(client.post("/api/v1/system/config/reload", nil, responseBuffer()))
	default:
		return fmt.Errorf("unknown system command %q", rest[0])
	}
}

func runAllowlist(args []string) error {
	client, rest, err := apiFlags("allowlist", args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("usage: sis allowlist <list|add|remove>")
	}
	switch rest[0] {
	case "list":
		return printResponse(client.get("/api/v1/allowlist", responseBuffer()))
	case "add":
		if len(rest) != 2 {
			return fmt.Errorf("usage: sis allowlist add <domain>")
		}
		return printResponse(client.post("/api/v1/allowlist", map[string]string{"domain": rest[1]}, responseBuffer()))
	case "remove":
		if len(rest) != 2 {
			return fmt.Errorf("usage: sis allowlist remove <domain>")
		}
		return client.delete("/api/v1/allowlist/"+encodedPathPart(rest[1]), io.Discard)
	default:
		return fmt.Errorf("unknown allowlist command %q", rest[0])
	}
}

func runBlocklist(args []string) error {
	client, rest, err := apiFlags("blocklist", args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("usage: sis blocklist <list|sync|entries|custom|add|remove>")
	}
	switch rest[0] {
	case "list":
		return printResponse(client.get("/api/v1/blocklists", responseBuffer()))
	case "sync":
		if len(rest) != 2 {
			return fmt.Errorf("usage: sis blocklist sync <id>")
		}
		return printResponse(client.post("/api/v1/blocklists/"+encodedPathPart(rest[1])+"/sync", nil, responseBuffer()))
	case "entries":
		if len(rest) < 2 || len(rest) > 3 {
			return fmt.Errorf("usage: sis blocklist entries <id> [query]")
		}
		path := "/api/v1/blocklists/" + encodedPathPart(rest[1]) + "/entries"
		if len(rest) == 3 {
			path += "?q=" + encodedQuery(rest[2])
		}
		return printResponse(client.get(path, responseBuffer()))
	case "custom":
		return printResponse(client.get("/api/v1/custom-blocklist", responseBuffer()))
	case "add":
		if len(rest) != 2 {
			return fmt.Errorf("usage: sis blocklist add <domain>")
		}
		return printResponse(client.post("/api/v1/custom-blocklist", map[string]string{"domain": rest[1]}, responseBuffer()))
	case "remove":
		if len(rest) != 2 {
			return fmt.Errorf("usage: sis blocklist remove <domain>")
		}
		return client.delete("/api/v1/custom-blocklist/"+encodedPathPart(rest[1]), io.Discard)
	default:
		return fmt.Errorf("unknown blocklist command %q", rest[0])
	}
}

func runGroup(args []string) error {
	client, rest, err := apiFlags("group", args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("usage: sis group <list|add|delete>")
	}
	switch rest[0] {
	case "list":
		return printResponse(client.get("/api/v1/groups", responseBuffer()))
	case "add":
		if len(rest) != 2 {
			return fmt.Errorf("usage: sis group add <name>")
		}
		return printResponse(client.post("/api/v1/groups", map[string]any{
			"name": rest[1], "blocklists": []string{}, "allowlist": []string{}, "schedules": []string{},
		}, responseBuffer()))
	case "delete":
		if len(rest) != 2 {
			return fmt.Errorf("usage: sis group delete <name>")
		}
		return client.delete("/api/v1/groups/"+encodedPathPart(rest[1]), io.Discard)
	default:
		return fmt.Errorf("unknown group command %q", rest[0])
	}
}

func runQuery(args []string) error {
	fs := flag.NewFlagSet("query", flag.ExitOnError)
	apiBase := fs.String("api", "", "Sis API base URL; when set, test through /api/v1/query/test")
	cookie := fs.String("cookie", "", "Cookie header for API mode, for example sis_session=...")
	server := fs.String("server", "127.0.0.1:5353", "DNS server address")
	proto := fs.String("proto", "udp", "udp or tcp")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 || rest[0] != "test" || len(rest) < 2 {
		return fmt.Errorf("usage: sis query [-server addr] [-proto udp|tcp] [-api url -cookie value] test <domain> [type]")
	}
	*proto = strings.ToLower(strings.TrimSpace(*proto))
	if *proto != "udp" && *proto != "tcp" {
		return fmt.Errorf("proto must be udp or tcp")
	}
	qtype := uint16(mdns.TypeA)
	qtypeName := "A"
	if len(rest) > 2 {
		if parsed, ok := mdns.StringToType[rest[2]]; ok {
			qtype = parsed
			qtypeName = rest[2]
		} else {
			return fmt.Errorf("unknown qtype %q", rest[2])
		}
	}
	if *apiBase != "" {
		client := newCLIClient(*apiBase, *cookie)
		return printResponse(client.post("/api/v1/query/test", map[string]string{
			"domain": rest[1], "type": qtypeName, "proto": *proto,
		}, responseBuffer()))
	}
	msg := new(mdns.Msg)
	msg.SetQuestion(mdns.Fqdn(rest[1]), qtype)
	client := &mdns.Client{Net: *proto, Timeout: 5 * time.Second}
	resp, rtt, err := client.Exchange(msg, *server)
	if err != nil {
		return err
	}
	fmt.Printf("rcode=%s rtt=%s answers=%d\n", mdns.RcodeToString[resp.Rcode], rtt, len(resp.Answer))
	for _, rr := range resp.Answer {
		fmt.Println(rr.String())
	}
	return nil
}

func runConfig(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: sis config <check|validate|show>")
	}
	switch args[0] {
	case "check", "validate":
		fs := flag.NewFlagSet("config "+args[0], flag.ExitOnError)
		path := fs.String("config", defaultConfigPath(), "config file path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		_, err := (&config.Loader{Path: *path}).Load()
		if err != nil {
			return err
		}
		fmt.Println("config ok")
		return nil
	case "show":
		fs := flag.NewFlagSet("config show", flag.ExitOnError)
		path := fs.String("config", defaultConfigPath(), "config file path")
		secrets := fs.Bool("secrets", false, "include password hashes and other secrets")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		cfg, err := (&config.Loader{Path: *path}).Load()
		if err != nil {
			return err
		}
		out := *cfg
		if !*secrets {
			out.Auth.Users = redactUsers(out.Auth.Users)
			out.Privacy.LogSalt = redactString(out.Privacy.LogSalt)
		}
		raw, err := yaml.Marshal(&out)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(raw)
		return err
	default:
		return fmt.Errorf("unknown config command %q", args[0])
	}
}

func redactUsers(users []config.User) []config.User {
	out := make([]config.User, len(users))
	copy(out, users)
	for i := range out {
		out[i].PasswordHash = redactString(out[i].PasswordHash)
	}
	return out
}

func redactString(value string) string {
	if value == "" {
		return ""
	}
	return "redacted"
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	path := fs.String("config", defaultConfigPath(), "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := (&config.Loader{Path: *path}).Load()
	if err != nil {
		return err
	}
	if changed, err := config.EnsureLogSalt(cfg); err != nil {
		return err
	} else if changed {
		if err := (&config.Loader{Path: *path}).Save(cfg); err != nil {
			return err
		}
	}
	holder := config.NewHolder(cfg)
	reloader := config.NewReloader(&config.Loader{Path: *path}, holder)
	reloader.Register(func(_, next *config.Config) error {
		changed, err := config.EnsureLogSalt(next)
		if err != nil {
			return err
		}
		if changed {
			return (&config.Loader{Path: *path}).Save(next)
		}
		return nil
	})

	st, err := store.Open(cfg.Server.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := seedConfigClients(st, cfg); err != nil {
		return err
	}

	queryLog, err := sislog.OpenQuery(cfg)
	if err != nil {
		return err
	}
	defer queryLog.Close()
	auditLog, err := sislog.OpenAudit(cfg)
	if err != nil {
		return err
	}
	defer auditLog.Close()

	counters := stats.New()
	engine, err := policy.NewEngine(cfg, policy.StoreClientResolver{Clients: st.Clients()})
	if err != nil {
		return err
	}
	if err := loadCustomPolicy(engine, st); err != nil {
		return err
	}
	engine.RegisterReload(reloader)
	pool := upstream.NewPool(cfg.Upstreams)

	arp := sisdns.NewARPTable(30 * time.Second)
	clientID := sisdns.NewClientID(arp, st.Clients())
	cache := sisdns.NewCache(sisdns.CacheOptions{
		MaxEntries: cfg.Cache.MaxEntries,
		MinTTL:     cfg.Cache.MinTTL.Duration, MaxTTL: cfg.Cache.MaxTTL.Duration,
		NegativeTTL: cfg.Cache.NegativeTTL.Duration,
	})
	limiter := sisdns.NewRateLimiter(cfg.Server.DNS.RateLimitQPS, cfg.Server.DNS.RateLimitBurst)
	pipeline := sisdns.NewPipelineWithDeps(sisdns.PipelineOptions{
		Config: holder, Cache: cache, Policy: engine, Upstream: pool,
		Log: queryLog, Stats: counters, ClientID: clientID, Limiter: limiter,
	})
	dnsServer := sisdns.NewServer(holder, pipeline)
	fetcher := policy.NewFetcher(filepath.Join(cfg.Server.DataDir, "blocklists"))
	syncer := policy.NewSyncer(holder, fetcher, engine, auditLog)
	apiServer := api.NewWithDeps(api.Options{
		Config: holder, Logger: slog.Default(), QueryLog: queryLog,
		Audit: auditLog, Policy: engine, Stats: counters, Store: st,
		Syncer: syncer, Upstream: pool, Cache: cache, Pipeline: pipeline, ConfigPath: *path,
	})
	reloader.Register(func(_, next *config.Config) error {
		if err := seedConfigClients(st, next); err != nil {
			return err
		}
		if queryLog != nil {
			if err := queryLog.Reconfigure(next); err != nil {
				return err
			}
		}
		if auditLog != nil {
			if err := auditLog.Reconfigure(next); err != nil {
				return err
			}
		}
		pool.Replace(next.Upstreams)
		cache.Reconfigure(sisdns.CacheOptions{
			MaxEntries: next.Cache.MaxEntries,
			MinTTL:     next.Cache.MinTTL.Duration, MaxTTL: next.Cache.MaxTTL.Duration,
			NegativeTTL: next.Cache.NegativeTTL.Duration,
		})
		pipeline.Reconfigure(next)
		return appendRuntimeConfigHistory(st, next)
	})
	aggregator := stats.NewAggregator(counters, st.Stats())

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go reloader.WatchSIGHUP(ctx, slog.Default())
	go arp.Run(ctx)
	go syncer.Run(ctx)
	go pool.RunHealthProber(ctx, time.Minute)
	go aggregator.Run(ctx)
	go cleanupSessions(ctx, st)
	go watchOperationalSignals(ctx, cfg.Server.DataDir, queryLog, auditLog)
	if err := dnsServer.Start(ctx); err != nil {
		return err
	}
	apiErr := make(chan error, 1)
	go func() {
		apiErr <- apiServer.Start(ctx)
	}()

	select {
	case <-ctx.Done():
	case err := <-apiErr:
		if err != nil {
			return err
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := dnsServer.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return apiServer.Shutdown(shutdownCtx)
}

func watchOperationalSignals(ctx context.Context, dataDir string, queryLog *sislog.Query, auditLog *sislog.Audit) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGUSR1, syscall.SIGUSR2)
	defer signal.Stop(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case sig := <-ch:
			switch sig {
			case syscall.SIGUSR1:
				_ = queryLog.Rotate()
				_ = auditLog.Rotate()
			case syscall.SIGUSR2:
				_ = dumpDebug(dataDir)
			}
		}
	}
}

func dumpDebug(dataDir string) error {
	dir := filepath.Join(dataDir, "dbg")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	goroutines, err := os.OpenFile(filepath.Join(dir, "goroutine-"+stamp+".txt"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	if err := pprof.Lookup("goroutine").WriteTo(goroutines, 2); err != nil {
		_ = goroutines.Close()
		return err
	}
	if err := goroutines.Close(); err != nil {
		return err
	}
	heap, err := os.OpenFile(filepath.Join(dir, "heap-"+stamp+".pprof"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return err
	}
	_ = pprof.WriteHeapProfile(heap)
	return heap.Close()
}

func cleanupSessions(ctx context.Context, st store.Store) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = st.Sessions().DeleteExpired()
		}
	}
}

func defaultConfigPath() string {
	if _, err := os.Stat("/etc/sis/sis.yaml"); err == nil {
		return "/etc/sis/sis.yaml"
	}
	return "./sis.yaml"
}

func loadCustomPolicy(engine *policy.Engine, st store.Store) error {
	customBlock, err := st.CustomLists().List("custom")
	if err != nil {
		return err
	}
	for _, domain := range customBlock {
		engine.AddCustomBlock(domain)
	}
	customAllow, err := st.CustomLists().List("custom-allow")
	if err != nil {
		return err
	}
	for _, domain := range customAllow {
		engine.AddCustomAllow(domain)
	}
	return nil
}

func seedConfigClients(st store.Store, cfg *config.Config) error {
	if st == nil || cfg == nil {
		return nil
	}
	now := time.Now().UTC()
	for _, configured := range cfg.Clients {
		if configured.Key == "" {
			continue
		}
		client, err := st.Clients().Get(configured.Key)
		if err != nil {
			if !errors.Is(err, store.ErrNotFound) {
				return err
			}
			client = &store.Client{
				Key: configured.Key, Type: configured.Type,
				FirstSeen: now, LastSeen: now,
			}
		}
		if configured.Type != "" {
			client.Type = configured.Type
		}
		if client.Type == "" {
			client.Type = "ip"
		}
		client.Name = configured.Name
		if configured.Group != "" {
			client.Group = configured.Group
		}
		if client.Group == "" {
			client.Group = "default"
		}
		client.Hidden = configured.Hidden
		if err := st.Clients().Upsert(client); err != nil {
			return err
		}
	}
	return nil
}

func appendRuntimeConfigHistory(st store.Store, cfg *config.Config) error {
	if st == nil || cfg == nil {
		return nil
	}
	raw, err := yaml.Marshal(config.RedactedCopy(cfg))
	if err != nil {
		return err
	}
	return st.ConfigHistory().Append(&store.ConfigSnapshot{
		TS:   time.Now().UTC(),
		YAML: string(raw),
	})
}
