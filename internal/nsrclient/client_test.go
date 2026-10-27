package nsrclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	testUser = "user"
	testPass = "s3cret-pw"
)

// newTestClient serves handler and returns a client plus the request counter and log buffer.
func newTestClient(t *testing.T, trace bool, handler func(w http.ResponseWriter, r *http.Request, n int32)) (*Client, *int32, *bytes.Buffer) {
	t.Helper()
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler(w, r, atomic.AddInt32(&n, 1))
	}))
	t.Cleanup(srv.Close)
	c, buf := newClientFor(srv.URL, trace)
	return c, &n, buf
}

func newClientFor(host string, trace bool) (*Client, *bytes.Buffer) {
	logBuf := &bytes.Buffer{}
	log := logrus.New()
	log.SetOutput(logBuf)
	log.SetLevel(logrus.DebugLevel)
	return New(Options{Name: "t", Host: host, Username: testUser, Password: testPass, Timeout: 5 * time.Second, Trace: trace, Log: log}), logBuf
}

func status(code int) func(http.ResponseWriter, *http.Request, int32) {
	return func(w http.ResponseWriter, _ *http.Request, _ int32) { w.WriteHeader(code) }
}

func TestGet_401NotRetried(t *testing.T) {
	c, n, _ := newTestClient(t, false, status(http.StatusUnauthorized))
	err := c.Get(context.Background(), "/clients", QueryOpts{}, &struct{}{})
	if err == nil || !strings.Contains(err.Error(), "t") || !strings.Contains(err.Error(), "/clients") {
		t.Fatalf("want error naming system and path, got %v", err)
	}
	if got := atomic.LoadInt32(n); got != 1 {
		t.Fatalf("401 must not be retried: %d requests", got)
	}
}

func TestGet_503RetriedTwice(t *testing.T) {
	c, n, _ := newTestClient(t, false, status(http.StatusServiceUnavailable))
	if err := c.Get(context.Background(), "/clients", QueryOpts{}, &struct{}{}); err == nil {
		t.Fatal("want error")
	}
	if got := atomic.LoadInt32(n); got != 3 {
		t.Fatalf("want 3 requests, got %d", got)
	}
}

func TestGet_RetriesThenSucceeds(t *testing.T) {
	c, n, _ := newTestClient(t, false, func(w http.ResponseWriter, _ *http.Request, i int32) {
		if i == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"clients":[]}`))
	})
	if err := c.Get(context.Background(), "/clients", QueryOpts{}, &struct{}{}); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(n); got != 2 {
		t.Fatalf("want 2 requests, got %d", got)
	}
}

func TestGet_TransportErrorRetried(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var accepted int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(&accepted, 1)
			_ = conn.Close() // drop without a response: transport error
		}
	}()
	c, _ := newClientFor("http://"+ln.Addr().String(), false)
	if err := c.Get(context.Background(), "/clients", QueryOpts{}, &struct{}{}); err == nil {
		t.Fatal("want error")
	}
	if got := atomic.LoadInt32(&accepted); got < 2 {
		t.Fatalf("transport error should be retried, got %d connections", got)
	}
}

func TestGet_QueryParamsEncoded(t *testing.T) {
	var fl, q string
	c, _, _ := newTestClient(t, false, func(w http.ResponseWriter, r *http.Request, _ int32) {
		fl, q = r.URL.Query().Get("fl"), r.URL.Query().Get("q")
		_, _ = w.Write([]byte(`{}`))
	})
	opts := QueryOpts{Fields: []string{"a", "b"}, Filter: `saveTime:["24 hours"]`}
	if err := c.Get(context.Background(), "/backups", opts, &struct{}{}); err != nil {
		t.Fatal(err)
	}
	if fl != "a,b" || q != `saveTime:["24 hours"]` {
		t.Fatalf("fl=%q q=%q", fl, q)
	}
}

func TestGet_Non200ReturnsError(t *testing.T) {
	c, _, _ := newTestClient(t, false, status(http.StatusNotFound))
	err := c.Get(context.Background(), "/nope", QueryOpts{}, &struct{}{})
	if err == nil || !strings.Contains(err.Error(), "status 404") {
		t.Fatalf("got %v", err)
	}
}

func TestGet_SendsBasicAuth(t *testing.T) {
	var ok int32 = 1
	c, n, _ := newTestClient(t, false, func(w http.ResponseWriter, r *http.Request, i int32) {
		u, p, has := r.BasicAuth()
		if !has || u != testUser || p != testPass || !strings.HasPrefix(r.URL.Path, "/nwrestapi/v3/global/") {
			atomic.StoreInt32(&ok, 0)
		}
		if i == 1 {
			w.WriteHeader(http.StatusServiceUnavailable) // retried request must carry auth too
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	if err := c.Get(context.Background(), "/clients", QueryOpts{}, &struct{}{}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(n) != 2 || atomic.LoadInt32(&ok) != 1 {
		t.Fatalf("requests=%d authOK=%d", atomic.LoadInt32(n), atomic.LoadInt32(&ok))
	}
}

func TestTrace_DoesNotLeakCredentials(t *testing.T) {
	c, _, buf := newTestClient(t, true, func(w http.ResponseWriter, _ *http.Request, _ int32) {
		_, _ = w.Write([]byte(`{"clients":[]}`))
	})
	if err := c.Get(context.Background(), "/clients", QueryOpts{}, &struct{}{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"GET", "/nwrestapi/v3/global/clients", "status=200"} {
		if !strings.Contains(out, want) {
			t.Errorf("trace missing %q in %s", want, out)
		}
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(testUser + ":" + testPass))
	for _, secret := range []string{testPass, b64} {
		if strings.Contains(out, secret) {
			t.Errorf("trace leaks %q", secret)
		}
	}
}
