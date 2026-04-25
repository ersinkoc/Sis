package dns

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	sislog "github.com/ersinkoc/sis/internal/log"
	"github.com/ersinkoc/sis/internal/policy"
	"github.com/ersinkoc/sis/internal/stats"
	"github.com/ersinkoc/sis/internal/upstream"
	mdns "github.com/miekg/dns"
)

type Request struct {
	Msg       *mdns.Msg
	SrcIP     net.IP
	Proto     string
	StartedAt time.Time
}

type Response struct {
	Msg     *mdns.Msg
	Source  string
	Latency time.Duration
}

type Pipeline struct {
	mu       sync.RWMutex
	cfg      *config.Holder
	cache    *Cache
	policy   *policy.Engine
	upstream *upstream.Pool
	log      *sislog.Query
	stats    *stats.Counters
	clientID *ClientID
	limiter  *RateLimiter
}

type PipelineOptions struct {
	Config   *config.Holder
	Cache    *Cache
	Policy   *policy.Engine
	Upstream *upstream.Pool
	Log      *sislog.Query
	Stats    *stats.Counters
	ClientID *ClientID
	Limiter  *RateLimiter
}

func NewPipeline(cache *Cache) *Pipeline {
	return NewPipelineWithDeps(PipelineOptions{Cache: cache})
}

func NewPipelineWithDeps(opts PipelineOptions) *Pipeline {
	cache := opts.Cache
	if cache == nil {
		cache = NewCache(CacheOptions{})
	}
	if opts.Stats == nil {
		opts.Stats = stats.New()
	}
	return &Pipeline{
		cfg: opts.Config, cache: cache, policy: opts.Policy,
		upstream: opts.Upstream, log: opts.Log, stats: opts.Stats,
		clientID: opts.ClientID, limiter: opts.Limiter,
	}
}

func (p *Pipeline) Reconfigure(c *config.Config) {
	if p == nil || c == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.limiter = NewRateLimiter(c.Server.DNS.RateLimitQPS, c.Server.DNS.RateLimitBurst)
}

func (p *Pipeline) Handle(ctx context.Context, r *Request) *Response {
	if r == nil || r.Msg == nil {
		return &Response{Msg: synthServerFailure(nil), Source: "synthetic"}
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now()
	}
	if limiter := p.rateLimiter(); limiter != nil && !limiter.Allow(r.SrcIP) {
		if r.Proto == "tcp" {
			return &Response{Msg: synthRefused(r.Msg), Source: "rate-limit"}
		}
		return &Response{Source: "rate-limit"}
	}
	p.stats.IncQuery()
	identity := p.identityFor(r.SrcIP)
	if len(r.Msg.Question) == 0 {
		return p.finish(r, identity, synthRCode(r.Msg, mdns.RcodeFormatError), "synthetic", false, "", "")
	}
	q := r.Msg.Question[0]
	qname := canonicalName(q.Name)
	if p.clientID != nil {
		_ = p.clientID.Touch(identity)
	}
	if resp, ok := handleSpecial(r.Msg, qname, q.Qtype, p.blockLocalPTR()); ok {
		return p.finish(r, identity, resp, "local", false, "", "")
	}
	key := cacheKey{qname: qname, qtype: q.Qtype, qclass: q.Qclass}
	if p.policy != nil {
		decision := p.policy.For(policy.Identity{Key: identity.Key, Type: identity.Type, IP: identity.IP.String()}).Evaluate(qname, q.Qtype, time.Now())
		if decision.Blocked {
			p.stats.IncBlocked()
			resp := synthBlock(r.Msg, q.Qtype, p.blockOptions())
			return p.finish(r, identity, resp, "synthetic", true, decision.Reason, decision.List)
		}
	}
	if cached, ok := p.cache.Get(key, r.Msg); ok {
		p.stats.IncCacheHit()
		return p.finish(r, identity, cached, "cache", false, "", "")
	}
	p.stats.IncCacheMiss()
	if p.upstream != nil {
		out := r.Msg
		if p.stripECS() {
			out = StripECS(r.Msg)
		}
		resp, src, attempts, err := p.upstream.Forward(ctx, out)
		p.recordUpstreamAttempts(attempts, r.StartedAt)
		if err == nil {
			p.cache.Put(key, resp)
			return p.finish(r, identity, resp, "upstream:"+src, false, "", "")
		}
	}
	resp := synthRCode(r.Msg, mdns.RcodeServerFailure)
	p.cache.Put(key, resp)
	return p.finish(r, identity, resp, "synthetic", false, "upstream-error", "")
}

func (p *Pipeline) recordUpstreamAttempts(attempts []upstream.Attempt, started time.Time) {
	for _, attempt := range attempts {
		upstreamStats := p.stats.Upstream(attempt.ID)
		upstreamStats.IncRequest()
		if attempt.OK {
			upstreamStats.MarkSuccess()
			upstreamStats.ObserveLatency(time.Since(started))
			continue
		}
		upstreamStats.IncError()
		if !attempt.Healthy {
			upstreamStats.MarkUnhealthy()
		}
	}
}

func (p *Pipeline) rateLimiter() *RateLimiter {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.limiter
}

func (p *Pipeline) finish(r *Request, id Identity, msg *mdns.Msg, source string, blocked bool, reason, list string) *Response {
	resp := &Response{Msg: msg, Source: source, Latency: time.Since(r.StartedAt)}
	p.stats.ObserveLatency(resp.Latency)
	if len(r.Msg.Question) > 0 {
		p.stats.AddDomain(canonicalName(r.Msg.Question[0].Name), blocked)
		p.stats.AddClient(id.Key)
	}
	if p.log != nil && len(r.Msg.Question) > 0 {
		q := r.Msg.Question[0]
		_ = p.log.Write(&sislog.Entry{
			TS: time.Now().UTC(), ClientKey: id.Key, ClientIP: id.IP.String(),
			QName: q.Name, QType: qtypeString(q.Qtype), QClass: qclassString(q.Qclass),
			RCode: mdns.RcodeToString[msg.Rcode], Answers: answerStrings(msg),
			Blocked: blocked, BlockReason: reason, BlockList: list,
			Upstream: upstreamSource(source), CacheHit: source == "cache",
			LatencyUS: resp.Latency.Microseconds(), Proto: r.Proto,
		})
	}
	return resp
}

func canonicalName(name string) string {
	return mdns.Fqdn(mdns.CanonicalName(name))
}

func (p *Pipeline) blockLocalPTR() bool {
	if p.cfg == nil || p.cfg.Get() == nil {
		return true
	}
	return p.cfg.Get().Privacy.BlockLocalPTR
}

func (p *Pipeline) stripECS() bool {
	if p.cfg == nil || p.cfg.Get() == nil {
		return true
	}
	return p.cfg.Get().Privacy.StripECS
}

func (p *Pipeline) blockOptions() BlockOptions {
	opts := BlockOptions{ResponseA: net.IPv4zero, ResponseAAAA: net.IPv6zero, TTL: time.Minute}
	if p.cfg == nil || p.cfg.Get() == nil {
		return opts
	}
	cfg := p.cfg.Get()
	if ip := net.ParseIP(cfg.Block.ResponseA); ip != nil {
		opts.ResponseA = ip
	}
	if ip := net.ParseIP(cfg.Block.ResponseAAAA); ip != nil {
		opts.ResponseAAAA = ip
	}
	opts.TTL = cfg.Block.ResponseTTL.Duration
	opts.UseNXDOMAIN = cfg.Block.UseNXDOMAIN
	return opts
}

func clientKey(ip net.IP) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}

func (p *Pipeline) identityFor(ip net.IP) Identity {
	if p.clientID != nil {
		return p.clientID.Resolve(ip)
	}
	return Identity{Key: clientKey(ip), Type: "ip", IP: ip}
}

func qtypeString(qtype uint16) string {
	if s := mdns.TypeToString[qtype]; s != "" {
		return s
	}
	return "TYPE" + strconv.Itoa(int(qtype))
}

func qclassString(qclass uint16) string {
	if s := mdns.ClassToString[qclass]; s != "" {
		return s
	}
	return "CLASS" + strconv.Itoa(int(qclass))
}

func upstreamSource(source string) string {
	const prefix = "upstream:"
	if len(source) > len(prefix) && source[:len(prefix)] == prefix {
		return source[len(prefix):]
	}
	return ""
}

func answerStrings(msg *mdns.Msg) []string {
	if msg == nil || len(msg.Answer) == 0 {
		return nil
	}
	out := make([]string, 0, len(msg.Answer))
	for _, rr := range msg.Answer {
		switch v := rr.(type) {
		case *mdns.A:
			out = append(out, v.A.String())
		case *mdns.AAAA:
			out = append(out, v.AAAA.String())
		case *mdns.CNAME:
			out = append(out, v.Target)
		case *mdns.MX:
			out = append(out, v.Mx)
		case *mdns.NS:
			out = append(out, v.Ns)
		case *mdns.PTR:
			out = append(out, v.Ptr)
		case *mdns.TXT:
			out = append(out, v.Txt...)
		default:
			out = append(out, rr.String())
		}
	}
	return out
}
