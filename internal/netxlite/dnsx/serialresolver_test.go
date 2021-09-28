package dnsx

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"testing"

	"github.com/miekg/dns"
	"github.com/ooni/probe-cli/v3/internal/atomicx"
	"github.com/ooni/probe-cli/v3/internal/netxlite/dnsx/mocks"
	"github.com/ooni/probe-cli/v3/internal/netxlite/errorsx"
)

func TestOONIGettingTransport(t *testing.T) {
	txp := NewDNSOverTLS((&tls.Dialer{}).DialContext, "8.8.8.8:853")
	r := NewSerialResolver(txp)
	rtx := r.Transport()
	if rtx.Network() != "dot" || rtx.Address() != "8.8.8.8:853" {
		t.Fatal("not the transport we expected")
	}
	if r.Network() != rtx.Network() {
		t.Fatal("invalid network seen from the resolver")
	}
	if r.Address() != rtx.Address() {
		t.Fatal("invalid address seen from the resolver")
	}
}

func TestOONIEncodeError(t *testing.T) {
	mocked := errors.New("mocked error")
	txp := NewDNSOverTLS((&tls.Dialer{}).DialContext, "8.8.8.8:853")
	r := SerialResolver{
		Encoder: &mocks.DNSEncoder{
			MockEncode: func(domain string, qtype uint16, padding bool) ([]byte, error) {
				return nil, mocked
			},
		},
		Txp: txp,
	}
	addrs, err := r.LookupHost(context.Background(), "www.gogle.com")
	if !errors.Is(err, mocked) {
		t.Fatal("not the error we expected")
	}
	if addrs != nil {
		t.Fatal("expected nil address here")
	}
}

func TestOONIRoundTripError(t *testing.T) {
	mocked := errors.New("mocked error")
	txp := &mocks.DNSTransport{
		MockRoundTrip: func(ctx context.Context, query []byte) (reply []byte, err error) {
			return nil, mocked
		},
		MockRequiresPadding: func() bool {
			return true
		},
	}
	r := NewSerialResolver(txp)
	addrs, err := r.LookupHost(context.Background(), "www.gogle.com")
	if !errors.Is(err, mocked) {
		t.Fatal("not the error we expected")
	}
	if addrs != nil {
		t.Fatal("expected nil address here")
	}
}

func TestOONIWithEmptyReply(t *testing.T) {
	txp := &mocks.DNSTransport{
		MockRoundTrip: func(ctx context.Context, query []byte) (reply []byte, err error) {
			return genReplySuccess(t, dns.TypeA), nil
		},
		MockRequiresPadding: func() bool {
			return true
		},
	}
	r := NewSerialResolver(txp)
	addrs, err := r.LookupHost(context.Background(), "www.gogle.com")
	if !errors.Is(err, errorsx.ErrOODNSNoAnswer) {
		t.Fatal("not the error we expected", err)
	}
	if addrs != nil {
		t.Fatal("expected nil address here")
	}
}

func TestOONIWithAReply(t *testing.T) {
	txp := &mocks.DNSTransport{
		MockRoundTrip: func(ctx context.Context, query []byte) (reply []byte, err error) {
			return genReplySuccess(t, dns.TypeA, "8.8.8.8"), nil
		},
		MockRequiresPadding: func() bool {
			return true
		},
	}
	r := NewSerialResolver(txp)
	addrs, err := r.LookupHost(context.Background(), "www.gogle.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(addrs) != 1 || addrs[0] != "8.8.8.8" {
		t.Fatal("not the result we expected")
	}
}

func TestOONIWithAAAAReply(t *testing.T) {
	txp := &mocks.DNSTransport{
		MockRoundTrip: func(ctx context.Context, query []byte) (reply []byte, err error) {
			return genReplySuccess(t, dns.TypeAAAA, "::1"), nil
		},
		MockRequiresPadding: func() bool {
			return true
		},
	}
	r := NewSerialResolver(txp)
	addrs, err := r.LookupHost(context.Background(), "www.gogle.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(addrs) != 1 || addrs[0] != "::1" {
		t.Fatal("not the result we expected")
	}
}

func TestOONIWithTimeout(t *testing.T) {
	txp := &mocks.DNSTransport{
		MockRoundTrip: func(ctx context.Context, query []byte) (reply []byte, err error) {
			return nil, &net.OpError{Err: errorsx.ETIMEDOUT, Op: "dial"}
		},
		MockRequiresPadding: func() bool {
			return true
		},
	}
	r := NewSerialResolver(txp)
	addrs, err := r.LookupHost(context.Background(), "www.gogle.com")
	if !errors.Is(err, errorsx.ETIMEDOUT) {
		t.Fatal("not the error we expected")
	}
	if addrs != nil {
		t.Fatal("expected nil address here")
	}
	if r.NumTimeouts.Load() <= 0 {
		t.Fatal("we didn't actually take the timeouts")
	}
}

func TestSerialResolverCloseIdleConnections(t *testing.T) {
	var called bool
	r := &SerialResolver{
		Txp: &mocks.DNSTransport{
			MockCloseIdleConnections: func() {
				called = true
			},
		},
	}
	r.CloseIdleConnections()
	if !called {
		t.Fatal("not called")
	}
}

func TestSerialResolverLookupHTTPS(t *testing.T) {
	t.Run("for encoding error", func(t *testing.T) {
		expected := errors.New("mocked error")
		r := &SerialResolver{
			Encoder: &mocks.DNSEncoder{
				MockEncode: func(domain string, qtype uint16, padding bool) ([]byte, error) {
					return nil, expected
				},
			},
			Decoder:     nil,
			NumTimeouts: &atomicx.Int64{},
			Txp: &mocks.DNSTransport{
				MockRequiresPadding: func() bool {
					return false
				},
			},
		}
		ctx := context.Background()
		https, err := r.LookupHTTPS(ctx, "example.com")
		if !errors.Is(err, expected) {
			t.Fatal("unexpected err", err)
		}
		if https != nil {
			t.Fatal("unexpected result")
		}
	})

	t.Run("for round-trip error", func(t *testing.T) {
		expected := errors.New("mocked error")
		r := &SerialResolver{
			Encoder: &mocks.DNSEncoder{
				MockEncode: func(domain string, qtype uint16, padding bool) ([]byte, error) {
					return make([]byte, 64), nil
				},
			},
			Decoder:     nil,
			NumTimeouts: &atomicx.Int64{},
			Txp: &mocks.DNSTransport{
				MockRoundTrip: func(ctx context.Context, query []byte) (reply []byte, err error) {
					return nil, expected
				},
				MockRequiresPadding: func() bool {
					return false
				},
			},
		}
		ctx := context.Background()
		https, err := r.LookupHTTPS(ctx, "example.com")
		if !errors.Is(err, expected) {
			t.Fatal("unexpected err", err)
		}
		if https != nil {
			t.Fatal("unexpected result")
		}
	})

	t.Run("for decode error", func(t *testing.T) {
		expected := errors.New("mocked error")
		r := &SerialResolver{
			Encoder: &mocks.DNSEncoder{
				MockEncode: func(domain string, qtype uint16, padding bool) ([]byte, error) {
					return make([]byte, 64), nil
				},
			},
			Decoder: &mocks.DNSDecoder{
				MockDecodeHTTPS: func(reply []byte) (*mocks.HTTPSSvc, error) {
					return nil, expected
				},
			},
			NumTimeouts: &atomicx.Int64{},
			Txp: &mocks.DNSTransport{
				MockRoundTrip: func(ctx context.Context, query []byte) (reply []byte, err error) {
					return make([]byte, 128), nil
				},
				MockRequiresPadding: func() bool {
					return false
				},
			},
		}
		ctx := context.Background()
		https, err := r.LookupHTTPS(ctx, "example.com")
		if !errors.Is(err, expected) {
			t.Fatal("unexpected err", err)
		}
		if https != nil {
			t.Fatal("unexpected result")
		}
	})
}