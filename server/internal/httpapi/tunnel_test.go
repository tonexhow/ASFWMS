package httpapi

import (
	"asfwms.local/scheduler/internal/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicTunnelHostHTTPSOriginAndCookies(t *testing.T) {
	h := newHarness(t)
	_, body := h.call("GET", "/api/status", nil, map[string]string{"Host": "school.example"}, 200)
	if !strings.Contains(string(body), `"local_only":false`) {
		t.Fatalf("incorrect status: %s", body)
	}
	h.call("POST", "/api/setup", map[string]string{"username": "hijack", "password": "pass123456789"}, map[string]string{"Host": "school.example"}, 403)
	credentials := map[string]string{"username": "test_admin", "password": "ASFWMS-test-password-42"}
	h.call("POST", "/api/login", credentials, map[string]string{"Host": "school.example", "Origin": "https://school.example"}, 403)
	h.call("POST", "/api/login", credentials, map[string]string{"Host": "school.example", "Origin": "https://other.example", "X-Forwarded-Proto": "https"}, 403)
	response, _ := h.call("POST", "/api/login", credentials, map[string]string{"Host": "school.example", "Origin": "https://school.example", "X-Forwarded-Proto": "https"}, 200)
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("proxied HTTPS session cookie is insecure: %+v", cookies)
	}
	response, _ = h.call("POST", "/api/login", credentials, map[string]string{"Origin": h.server.URL}, 200)
	if len(response.Cookies()) != 1 || response.Cookies()[0].Secure {
		t.Fatal("HTTP local cookie should not require HTTPS")
	}
}

func TestConfiguredPublicOriginRestrictsOtherHostnames(t *testing.T) {
	app := &App{Config: config.Config{PublicURL: "https://asfwms.school.example"}}
	for _, tc := range []struct {
		host string
		want bool
	}{
		{"asfwms.school.example", true}, {"localhost:20000", true}, {"127.0.0.1:20000", true}, {"evil.example", false}, {"asfwms.school.example.evil", false},
	} {
		req := httptest.NewRequest("GET", "http://127.0.0.1:20000/api/status", nil)
		req.Host = tc.host
		if actual := app.allowedHost(req); actual != tc.want {
			t.Errorf("host %q: allowed=%v want=%v", tc.host, actual, tc.want)
		}
	}
}

// This test needs no SQLite driver: it exercises the tunnel-facing middleware
// directly and can run even while a database migration is being prepared.
func TestProxyMiddlewareWithoutDatabase(t *testing.T) {
	app := &App{Config: config.Config{Port: "20000"}}
	handler := app.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	makeReq := func(method, origin, proto string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://asfwms.tunnel.example/api/ping", nil)
		req.Host = "asfwms.tunnel.example"
		req.RemoteAddr = "127.0.0.1:31337"
		if method == "POST" {
			req.Header.Set("X-ASFWMS-Request", "1")
			req.Header.Set("Content-Type", "application/json")
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if proto != "" {
			req.Header.Set("X-Forwarded-Proto", proto)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	for _, tc := range []struct {
		method, origin, proto string
		expected              int
	}{
		{"GET", "", "", 204},
		{"POST", "https://asfwms.tunnel.example", "https", 204},
		{"POST", "https://evil.example", "https", 403},
		{"POST", "https://asfwms.tunnel.example", "", 403},
		{"POST", "http://asfwms.tunnel.example", "https", 403},
	} {
		actual := makeReq(tc.method, tc.origin, tc.proto).Code
		if actual != tc.expected {
			t.Errorf("%+v: response status=%d", tc, actual)
		}
	}
	req := httptest.NewRequest("POST", "http://asfwms.tunnel.example/api/setup", nil)
	req.Host = "asfwms.tunnel.example"
	req.RemoteAddr = "127.0.0.1:31337"
	res := httptest.NewRecorder()
	app.setup(res, req)
	if res.Code != 403 {
		t.Fatal("setup should be unavailable via a public Host")
	}
	req = httptest.NewRequest("POST", "http://127.0.0.1:20000/api/setup", nil)
	req.RemoteAddr = "127.0.0.1:31337"
	req.Header.Set("X-Forwarded-Proto", "https")
	res = httptest.NewRecorder()
	app.setup(res, req)
	if res.Code != 403 {
		t.Fatal("setup should reject requests marked as forwarded")
	}
	req = httptest.NewRequest("POST", "http://asfwms.tunnel.example/api/login", nil)
	req.RemoteAddr = "127.0.0.1:31337"
	req.Header.Set("X-Forwarded-Proto", "https")
	res = httptest.NewRecorder()
	setCookie(res, req, strings.Repeat("a", 64))
	if cookies := res.Result().Cookies(); len(cookies) != 1 || !cookies[0].Secure {
		t.Fatal("HTTPS tunnel cookie must be Secure")
	}
}
