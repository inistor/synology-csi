package webapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Credentials in a URL end up in the DSM's web server log and any proxy's, so
// every parameter must travel in the request body.
func TestLoginSendsCredentialsInThePostBodyOnly(t *testing.T) {
	var method, rawQuery, contentType, account, passwd string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, rawQuery, contentType = r.Method, r.URL.RawQuery, r.Header.Get("Content-Type")
		account, passwd = r.PostFormValue("account"), r.PostFormValue("passwd")
		fakeDSMHandler(w, r)
	}))
	defer server.Close()

	host, port := serverHostPort(t, server)
	dsm := &DSM{Ip: host, Port: port, Username: "test-user", Password: "test-password"}
	if err := dsm.Login(); err != nil {
		t.Fatal(err)
	}

	if method != http.MethodPost {
		t.Errorf("method = %s, want POST", method)
	}
	if rawQuery != "" {
		t.Errorf("URL carries a query string: %q", rawQuery)
	}
	if !strings.HasPrefix(contentType, "application/x-www-form-urlencoded") {
		t.Errorf("Content-Type = %q", contentType)
	}
	if account != "test-user" || passwd != "test-password" {
		t.Errorf("body carried account=%q passwd=%q", account, passwd)
	}
}

func TestDebugLogRedactsSecrets(t *testing.T) {
	params := url.Values{"account": {"csi"}, "passwd": {"hunter2"}, "password": {"chap"}, "_sid": {"sid"}}
	logged := redactedParams(params)
	for _, secret := range []string{"hunter2", "chap", "=sid"} {
		if strings.Contains(logged, secret) {
			t.Errorf("debug log line %q contains %q", logged, secret)
		}
	}
	if !strings.Contains(logged, "account=csi") {
		t.Errorf("debug log line %q lost the non-secret parameters", logged)
	}
}
