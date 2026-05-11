package log

import "time"

// Entry is one structured DNS query log record.
type Entry struct {
	TS          time.Time `json:"ts"`
	RequestID   string    `json:"request_id,omitempty"`
	ClientKey   string    `json:"client_key,omitempty"`
	ClientName  string    `json:"client_name,omitempty"`
	ClientGroup string    `json:"client_group,omitempty"`
	ClientIP    string    `json:"client_ip,omitempty"`
	QName       string    `json:"qname"`
	QType       string    `json:"qtype"`
	QClass      string    `json:"qclass"`
	RCode       string    `json:"rcode"`
	Answers     []string  `json:"answers,omitempty"`
	Blocked     bool      `json:"blocked"`
	BlockReason string    `json:"block_reason,omitempty"`
	BlockList   string    `json:"block_list,omitempty"`
	Upstream    string    `json:"upstream,omitempty"`
	CacheHit    bool      `json:"cache_hit"`
	LatencyUS   int64     `json:"latency_us"`
	Proto       string    `json:"proto"`
}

func (e Entry) clone() Entry {
	if e.Answers != nil {
		e.Answers = append([]string(nil), e.Answers...)
	}
	return e
}

// AuditEntry is one structured administrative or system audit record.
type AuditEntry struct {
	TS      time.Time `json:"ts"`
	Actor   string    `json:"actor"`
	ActorIP string    `json:"actor_ip,omitempty"`
	Action  string    `json:"action"`
	Target  string    `json:"target"`
	Before  any       `json:"before,omitempty"`
	After   any       `json:"after,omitempty"`
}
