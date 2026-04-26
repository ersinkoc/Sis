package dns

import mdns "github.com/miekg/dns"

// StripECS returns a copy of msg with EDNS Client Subnet options removed.
func StripECS(msg *mdns.Msg) *mdns.Msg {
	if msg == nil {
		return nil
	}
	out := msg.Copy()
	for _, extra := range out.Extra {
		opt, ok := extra.(*mdns.OPT)
		if !ok {
			continue
		}
		filtered := opt.Option[:0]
		for _, option := range opt.Option {
			if option.Option() == mdns.EDNS0SUBNET {
				continue
			}
			filtered = append(filtered, option)
		}
		opt.Option = filtered
	}
	return out
}
