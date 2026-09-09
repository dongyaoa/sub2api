package tlsfingerprint

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProfileCacheKeyTracksHandshakeSettings(t *testing.T) {
	first := &Profile{Name: "one", Curves: []uint16{29}}
	renamed := &Profile{Name: "two", Curves: []uint16{29}}
	changed := &Profile{Name: "one", Curves: []uint16{29, 23}}
	require.Equal(t, first.CacheKey(), renamed.CacheKey())
	require.NotEqual(t, first.CacheKey(), changed.CacheKey())
	require.Empty(t, (*Profile)(nil).CacheKey())
}

func TestConfigureTransportRejectsUnsupportedALPN(t *testing.T) {
	err := ConfigureTransport(&http.Transport{}, &Profile{ALPNProtocols: []string{"h2", "http/1.1"}}, nil)
	require.ErrorContains(t, err, "supports only http/1.1")
}

func TestDefaultDialerEmitsConfiguredClientHello(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	hellos := make(chan *tls.ClientHelloInfo, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		server := tls.Server(conn, &tls.Config{GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) {
			hellos <- info
			return nil, errors.New("capture complete")
		}})
		_ = server.Handshake()
	}()
	profile := &Profile{Curves: []uint16{29, 23}, CipherSuites: []uint16{0x1301, 0x1302}}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_, err = NewDialer(profile, nil).DialTLSContext(ctx, "tcp", listener.Addr().String())
	require.Error(t, err, "capture server deliberately stops after ClientHello")
	<-done
	select {
	case hello := <-hellos:
		require.Equal(t, profile.CipherSuites, hello.CipherSuites)
		require.Equal(t, []tls.CurveID{tls.X25519, tls.CurveP256}, hello.SupportedCurves)
		require.Equal(t, []string{"http/1.1"}, hello.SupportedProtos)
	default:
		t.Fatal("no ClientHello captured")
	}
}

func TestHTTPProxyDialerAuthenticatesInsideHTTPSProxyTLS(t *testing.T) {
	requests := make(chan *http.Request, 1)
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(context.Background())
		w.WriteHeader(http.StatusProxyAuthRequired)
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	proxyURL.User = url.UserPassword("test-user", "test-password")
	roots := x509.NewCertPool()
	roots.AddCert(proxy.Certificate())
	dialer := NewHTTPProxyDialer(&Profile{}, proxyURL)
	dialer.proxyTLSConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	_, err = dialer.DialTLSContext(ctx, "tcp", "upstream.example:443")
	require.ErrorContains(t, err, "407")
	select {
	case req := <-requests:
		require.NotNil(t, req.TLS, "proxy authentication must be encrypted")
		require.Equal(t, http.MethodConnect, req.Method)
		require.Equal(t, "upstream.example:443", req.Host)
		require.Equal(t, "Basic dGVzdC11c2VyOnRlc3QtcGFzc3dvcmQ=", req.Header.Get("Proxy-Authorization"))
	default:
		t.Fatal("HTTPS proxy did not receive CONNECT")
	}
}

func TestHTTPProxyDialerCancelsStalledCONNECT(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer proxy.Close()
	proxyURL, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	_, err = NewHTTPProxyDialer(&Profile{}, proxyURL).DialTLSContext(ctx, "tcp", "upstream.example:443")
	require.Error(t, err)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
}
