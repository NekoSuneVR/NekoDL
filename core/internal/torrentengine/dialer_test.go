package torrentengine

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/things-go/go-socks5"
)

// startTestSOCKS5Proxy runs a real (if minimal) SOCKS5 server for tests —
// production code only ever needs a SOCKS5 *client* (dialer.go), but
// verifying that client actually speaks correct SOCKS5 needs a real server
// on the other end, not just trusting the client library.
func startTestSOCKS5Proxy(t *testing.T) (addr string) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := socks5.NewServer()
	go func() {
		_ = server.Serve(l)
	}()
	t.Cleanup(func() { l.Close() })
	return l.Addr().String()
}

func TestSOCKS5DialerConnectsThroughRealProxy(t *testing.T) {
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello through the proxy"))
	}))
	defer echo.Close()

	proxyAddr := startTestSOCKS5Proxy(t)

	d, err := newSOCKS5Dialer(proxyAddr)
	if err != nil {
		t.Fatalf("newSOCKS5Dialer: %v", err)
	}
	if d.DialerNetwork() != "tcp" {
		t.Fatalf("DialerNetwork() = %q, want tcp", d.DialerNetwork())
	}

	echoAddr := echo.Listener.Addr().String()
	conn, err := d.Dial(context.Background(), echoAddr)
	if err != nil {
		t.Fatalf("Dial through proxy: %v", err)
	}
	defer conn.Close()

	req, err := http.NewRequest(http.MethodGet, "http://"+echoAddr+"/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if err := req.Write(conn); err != nil {
		t.Fatalf("writing request over proxied conn: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		t.Fatalf("reading response over proxied conn: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if string(body) != "hello through the proxy" {
		t.Fatalf("got body %q", body)
	}
}

func TestSOCKS5HTTPDialContextRoutesRequestThroughProxy(t *testing.T) {
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("http client via proxy"))
	}))
	defer echo.Close()

	proxyAddr := startTestSOCKS5Proxy(t)

	dialCtx, err := socks5HTTPDialContext(proxyAddr)
	if err != nil {
		t.Fatalf("socks5HTTPDialContext: %v", err)
	}

	client := &http.Client{Transport: &http.Transport{DialContext: dialCtx}}
	resp, err := client.Get(echo.URL)
	if err != nil {
		t.Fatalf("Get via proxied client: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	if string(body) != "http client via proxy" {
		t.Fatalf("got body %q", body)
	}
}

func TestNewSOCKS5DialerRejectsUnreachableProxy(t *testing.T) {
	// Dial errors for an unreachable proxy surface from Dial(), not from
	// newSOCKS5Dialer() itself (which just builds the dialer) — this
	// documents that split rather than asserting a construction-time error.
	d, err := newSOCKS5Dialer("127.0.0.1:1")
	if err != nil {
		t.Fatalf("newSOCKS5Dialer: %v", err)
	}
	if _, err := d.Dial(context.Background(), "example.com:80"); err == nil {
		t.Fatal("expected an error dialing through an unreachable proxy")
	}
}
