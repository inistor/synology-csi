package webapi

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// chunkedDSMHandler sends the terminating chunk after the JSON, as DSM's nginx does.
func chunkedDSMHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fakeDSMResponse))
	w.(http.Flusher).Flush()
	time.Sleep(20 * time.Millisecond)
}

func TestSequentialRequestsReuseOneConnection(t *testing.T) {
	for _, https := range []bool{false, true} {
		t.Run(fmt.Sprintf("https=%t", https), func(t *testing.T) {
			var conns atomic.Int32
			server := httptest.NewUnstartedServer(http.HandlerFunc(chunkedDSMHandler))
			server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew {
					conns.Add(1)
				}
			}

			dsm := &DSM{Username: "admin", Password: "secret"}
			if https {
				certPEM, tlsCert := generateSelfSignedCert(t)
				server.TLS = &tls.Config{Certificates: []tls.Certificate{tlsCert}}
				server.StartTLS()
				dsm.Https, dsm.TLSCACert = true, certPEM
			} else {
				server.Start()
			}
			t.Cleanup(server.Close)
			dsm.Ip, dsm.Port = serverHostPort(t, server)

			const requests = 3
			for i := 0; i < requests; i++ {
				if err := dsm.Login(); err != nil {
					t.Fatalf("login %d: %v", i, err)
				}
			}
			if got := conns.Load(); got != 1 {
				t.Errorf("%d requests opened %d connections, want 1", requests, got)
			}
		})
	}
}
