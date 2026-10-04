package dhcpwatch

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"time"
)

// ListenRelay reads relay copies from conn until ctx ends or conn closes. It
// never writes to conn.
func ListenRelay(ctx context.Context, conn net.PacketConn, b *Book) error {
	go func() {
		<-ctx.Done()
		conn.Close()
	}()
	buf := make([]byte, 2048)
	for {
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		var src netip.Addr
		if u, ok := from.(*net.UDPAddr); ok {
			src, _ = netip.AddrFromSlice(u.IP)
		}
		b.Relayed(src, buf[:n])
	}
}

// knockHandshakes is how many TLS handshakes the option 224 listener runs at
// once; more connections wait.
const knockHandshakes = 16

// ServeKnocks accepts connections on ln until ctx ends or ln closes. Each is
// a knock: it shakes hands with cert, asking for, but not checking, the
// client's certificate; records what the client showed; and closes. It
// never speaks uCentral.
func ServeKnocks(ctx context.Context, ln net.Listener, cert tls.Certificate, b *Book) error {
	go func() {
		<-ctx.Done()
		ln.Close()
	}()
	slots := make(chan struct{}, knockHandshakes)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		slots <- struct{}{}
		go func() {
			defer func() { <-slots }()
			knock(conn, cert, b)
		}()
	}
}

func knock(conn net.Conn, cert tls.Certificate, b *Book) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	var h Hello
	if a, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		h.Source, _ = netip.AddrFromSlice(a.IP)
	}
	heard := false
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequestClientCert,
		MinVersion:   tls.VersionTLS10,
		GetConfigForClient: func(hi *tls.ClientHelloInfo) (*tls.Config, error) {
			h.SNI, h.Versions, heard = hi.ServerName, hi.SupportedVersions, true
			return nil, nil
		},
	}
	t := tls.Server(conn, cfg)
	err := t.Handshake()
	if err == nil {
		if cs := t.ConnectionState().PeerCertificates; len(cs) > 0 {
			h.Subject = cs[0].Subject.String()
		}
	}
	if !heard && h.Subject == "" {
		return // not TLS: a port scan, not a knock
	}
	if err := b.Knocked(h); err != nil {
		slog.Error("recording a knock", "source", h.Source, "err", err)
	}
}
