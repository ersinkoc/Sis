package dns

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/ersinkoc/sis/internal/config"
	mdns "github.com/miekg/dns"
)

// Server listens for classic DNS over UDP and TCP.
type Server struct {
	cfg      *config.Holder
	pipeline *Pipeline
	workers  *workerPool
	tcpSlots chan struct{}
	udpConns []*net.UDPConn
	tcpLns   []*net.TCPListener
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewServer creates a DNS server using cfg and pipeline.
func NewServer(cfg *config.Holder, pipeline *Pipeline) *Server {
	return &Server{cfg: cfg, pipeline: pipeline}
}

// Start binds configured UDP/TCP listeners and begins serving DNS queries.
func (s *Server) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	started := false
	defer func() {
		if !started {
			s.cleanupStarted()
		}
	}()
	cfg := s.cfg.Get()
	workers := cfg.Server.DNS.UDPWorkers
	if workers <= 0 {
		workers = runtime.NumCPU() * 4
	}
	s.workers = newWorkerPool(runCtx, workers, workers*8)
	tcpWorkers := cfg.Server.DNS.TCPWorkers
	if tcpWorkers <= 0 {
		tcpWorkers = runtime.NumCPU() * 4
	}
	s.tcpSlots = make(chan struct{}, tcpWorkers)
	for _, addr := range cfg.Server.DNS.Listen {
		udpAddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			return err
		}
		udpConn, err := net.ListenUDP("udp", udpAddr)
		if err != nil {
			return err
		}
		s.udpConns = append(s.udpConns, udpConn)
		s.wg.Add(1)
		go s.serveUDP(runCtx, udpConn)

		tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
		if err != nil {
			return err
		}
		tcpLn, err := net.ListenTCP("tcp", tcpAddr)
		if err != nil {
			return err
		}
		s.tcpLns = append(s.tcpLns, tcpLn)
		s.wg.Add(1)
		go s.serveTCP(runCtx, tcpLn)
	}
	started = true
	return nil
}

func (s *Server) cleanupStarted() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	for _, conn := range s.udpConns {
		_ = conn.Close()
	}
	for _, ln := range s.tcpLns {
		_ = ln.Close()
	}
	s.wg.Wait()
	if s.workers != nil {
		s.workers.Close()
		s.workers = nil
	}
	s.udpConns = nil
	s.tcpLns = nil
	s.tcpSlots = nil
}

// Shutdown closes listeners and waits for active workers to finish.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	for _, conn := range s.udpConns {
		_ = conn.Close()
	}
	for _, ln := range s.tcpLns {
		_ = ln.Close()
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		if s.workers != nil {
			s.workers.Close()
		}
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (s *Server) serveUDP(ctx context.Context, conn *net.UDPConn) {
	defer s.wg.Done()
	size := s.cfg.Get().Server.DNS.UDPSize
	if size <= 0 {
		size = 1232
	}
	for {
		buf := make([]byte, size)
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		packet := append([]byte(nil), buf[:n]...)
		_ = s.workers.Submit(func() {
			s.handleUDP(ctx, conn, addr, packet)
		}, false)
	}
}

func (s *Server) handleUDP(ctx context.Context, conn *net.UDPConn, addr *net.UDPAddr, packet []byte) {
	var msg mdns.Msg
	if err := msg.Unpack(packet); err != nil {
		return
	}
	resp := s.pipeline.Handle(ctx, &Request{Msg: &msg, SrcIP: addr.IP, Proto: "udp", StartedAt: time.Now()})
	if resp == nil || resp.Msg == nil {
		return
	}
	maxSize := s.cfg.Get().Server.DNS.UDPSize
	if maxSize <= 0 {
		maxSize = 1232
	}
	wire, err := packUDPResponse(resp.Msg, maxSize)
	if err != nil {
		return
	}
	_, _ = conn.WriteToUDP(wire, addr)
}

func packUDPResponse(msg *mdns.Msg, maxSize int) ([]byte, error) {
	wire, err := msg.Pack()
	if err != nil || maxSize <= 0 || len(wire) <= maxSize {
		return wire, err
	}
	truncated := msg.Copy()
	truncated.Truncated = true
	truncated.Answer = nil
	truncated.Ns = nil
	truncated.Extra = nil
	wire, err = truncated.Pack()
	if err != nil || len(wire) <= maxSize {
		return wire, err
	}
	minimal := new(mdns.Msg)
	minimal.MsgHdr = msg.MsgHdr
	minimal.Response = true
	minimal.Truncated = true
	return minimal.Pack()
}

func (s *Server) serveTCP(ctx context.Context, ln *net.TCPListener) {
	defer s.wg.Done()
	for {
		conn, err := ln.AcceptTCP()
		if err != nil {
			return
		}
		if !s.acquireTCPSlot() {
			_ = conn.Close()
			continue
		}
		s.wg.Add(1)
		go s.handleTCPConn(ctx, conn)
	}
}

func (s *Server) handleTCPConn(ctx context.Context, conn *net.TCPConn) {
	defer s.wg.Done()
	defer s.releaseTCPSlot()
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	for {
		var lenBuf [2]byte
		if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint16(lenBuf[:])
		packet := make([]byte, n)
		if _, err := io.ReadFull(conn, packet); err != nil {
			return
		}
		var msg mdns.Msg
		if err := msg.Unpack(packet); err != nil {
			return
		}
		resp := s.pipeline.Handle(ctx, &Request{Msg: &msg, SrcIP: remoteIP(conn.RemoteAddr()), Proto: "tcp", StartedAt: time.Now()})
		if resp == nil || resp.Msg == nil {
			return
		}
		wire, err := resp.Msg.Pack()
		if err != nil {
			return
		}
		var outLen [2]byte
		binary.BigEndian.PutUint16(outLen[:], uint16(len(wire)))
		if _, err := conn.Write(outLen[:]); err != nil {
			return
		}
		if _, err := conn.Write(wire); err != nil {
			return
		}
	}
}

func (s *Server) acquireTCPSlot() bool {
	if s.tcpSlots == nil {
		return true
	}
	select {
	case s.tcpSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Server) releaseTCPSlot() {
	if s.tcpSlots == nil {
		return
	}
	select {
	case <-s.tcpSlots:
	default:
	}
}

func remoteIP(addr net.Addr) net.IP {
	if tcp, ok := addr.(*net.TCPAddr); ok {
		return tcp.IP
	}
	return nil
}
