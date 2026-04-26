package dns

import (
	"net"
	"time"

	mdns "github.com/miekg/dns"
)

// BlockOptions controls the synthetic DNS response used for blocked queries.
type BlockOptions struct {
	ResponseA    net.IP
	ResponseAAAA net.IP
	TTL          time.Duration
	UseNXDOMAIN bool
}

func synthRCode(req *mdns.Msg, rcode int) *mdns.Msg {
	resp := new(mdns.Msg)
	if req != nil {
		resp.SetReply(req)
	} else {
		resp.MsgHdr.Response = true
	}
	resp.Rcode = rcode
	resp.RecursionAvailable = true
	return resp
}

func synthServerFailure(req *mdns.Msg) *mdns.Msg {
	return synthRCode(req, mdns.RcodeServerFailure)
}

func synthNXDOMAIN(req *mdns.Msg) *mdns.Msg {
	return synthRCode(req, mdns.RcodeNameError)
}

func synthRefused(req *mdns.Msg) *mdns.Msg {
	return synthRCode(req, mdns.RcodeRefused)
}

func synthNODATA(req *mdns.Msg, ttl uint32) *mdns.Msg {
	resp := synthRCode(req, mdns.RcodeSuccess)
	if req != nil && len(req.Question) > 0 {
		resp.Ns = []mdns.RR{soaFor(req.Question[0].Name, ttl)}
	}
	return resp
}

func synthLoopback(req *mdns.Msg, qtype uint16) *mdns.Msg {
	resp := synthRCode(req, mdns.RcodeSuccess)
	if len(req.Question) == 0 {
		return resp
	}
	name := req.Question[0].Name
	switch qtype {
	case mdns.TypeA:
		resp.Answer = []mdns.RR{&mdns.A{Hdr: rrHeader(name, mdns.TypeA, 60), A: net.IPv4(127, 0, 0, 1)}}
	case mdns.TypeAAAA:
		resp.Answer = []mdns.RR{&mdns.AAAA{Hdr: rrHeader(name, mdns.TypeAAAA, 60), AAAA: net.IPv6loopback}}
	default:
		resp.Ns = []mdns.RR{soaFor(name, 60)}
	}
	return resp
}

func synthBlock(req *mdns.Msg, qtype uint16, opts BlockOptions) *mdns.Msg {
	ttl := uint32(opts.TTL.Seconds())
	if ttl == 0 {
		ttl = 60
	}
	if opts.UseNXDOMAIN {
		return synthNXDOMAIN(req)
	}
	resp := synthRCode(req, mdns.RcodeSuccess)
	if req == nil || len(req.Question) == 0 {
		return resp
	}
	name := req.Question[0].Name
	switch qtype {
	case mdns.TypeA:
		ip := opts.ResponseA
		if ip == nil {
			ip = net.IPv4zero
		}
		resp.Answer = []mdns.RR{&mdns.A{Hdr: rrHeader(name, mdns.TypeA, ttl), A: ip.To4()}}
	case mdns.TypeAAAA:
		ip := opts.ResponseAAAA
		if ip == nil {
			ip = net.IPv6zero
		}
		resp.Answer = []mdns.RR{&mdns.AAAA{Hdr: rrHeader(name, mdns.TypeAAAA, ttl), AAAA: ip.To16()}}
	case mdns.TypeHTTPS, mdns.TypeSVCB:
		resp.Ns = []mdns.RR{soaFor(name, ttl)}
	default:
		return synthNXDOMAIN(req)
	}
	return resp
}

func rrHeader(name string, typ uint16, ttl uint32) mdns.RR_Header {
	return mdns.RR_Header{Name: mdns.Fqdn(name), Rrtype: typ, Class: mdns.ClassINET, Ttl: ttl}
}

func soaFor(name string, ttl uint32) mdns.RR {
	zone := mdns.Fqdn(name)
	return &mdns.SOA{
		Hdr:     rrHeader(zone, mdns.TypeSOA, ttl),
		Ns:      "sis.local.",
		Mbox:    "hostmaster.sis.local.",
		Serial:  1,
		Refresh: 60,
		Retry:   60,
		Expire:  60,
		Minttl:  ttl,
	}
}
