package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/HyeonTee/agent-control-plane/internal/adapter/httpapi"
	"github.com/HyeonTee/agent-control-plane/internal/adapter/postgres"
	appdevice "github.com/HyeonTee/agent-control-plane/internal/application/deviceauth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeGitHubTransport struct{ userID int64 }

func (f fakeGitHubTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var body string
	switch request.URL.String() {
	case "https://github.com/login/oauth/access_token":
		body = `{"access_token":"github-login-token"}`
	case "https://api.github.com/user":
		if request.Header.Get("Authorization") != "Bearer github-login-token" {
			return nil, fmt.Errorf("GitHub request omitted token")
		}
		body = fmt.Sprintf(`{"id":%d}`, f.userID)
	default:
		return nil, fmt.Errorf("unexpected GitHub request %s", request.URL)
	}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestGitHubApprovalRejectsWrongStateAndUnlistedOwner(t *testing.T) {
	auth, err := httpapi.NewDeviceAuthHandler(appdevice.New(postgres.NewDeviceAuthStore(nil)), httpapi.DeviceAuthConfig{
		PublicURL: "http://localhost:8080", ClientID: "test-github-app", ClientSecret: "test-secret", OwnerID: 42,
	}, &http.Client{Transport: fakeGitHubTransport{userID: 43}})
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	auth.Register(mux)
	start := httptest.NewRecorder()
	mux.ServeHTTP(start, httptest.NewRequest(http.MethodGet, "/auth/github/start", nil))
	if start.Code != http.StatusFound || len(start.Result().Cookies()) != 1 {
		t.Fatalf("login start = %d", start.Code)
	}
	state := start.Result().Cookies()[0]
	callback := func(value string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/auth/github/callback?code=valid&state="+value, nil)
		req.AddCookie(state)
		result := httptest.NewRecorder()
		mux.ServeHTTP(result, req)
		return result
	}
	if got := callback("wrong").Code; got != http.StatusBadRequest {
		t.Fatalf("wrong OAuth state accepted: %d", got)
	}
	owner := callback(state.Value)
	if owner.Code != http.StatusForbidden {
		t.Fatalf("unlisted GitHub account accepted: %d", owner.Code)
	}
	for _, cookie := range owner.Result().Cookies() {
		if cookie.Name == "hub_owner_session" {
			t.Fatal("unlisted GitHub account received an owner session")
		}
	}
}

func TestDeviceAuthorizationRequiresBrowserApprovalAndRotatesRefresh(t *testing.T) {
	databaseURL := os.Getenv("HUB_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("HUB_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("device_test_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		defer adminPool.Close()
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := adminPool.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
	})
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	if _, err := postgres.BootstrapOwner(ctx, pool, "personal", "owner"); err != nil {
		t.Fatal(err)
	}
	var server *httptest.Server
	auth, err := httpapi.NewDeviceAuthHandler(appdevice.New(postgres.NewDeviceAuthStore(pool)), httpapi.DeviceAuthConfig{
		PublicURL: "http://localhost:8080", ClientID: "test-github-app", ClientSecret: "test-secret", OwnerID: 42,
	}, &http.Client{Transport: fakeGitHubTransport{userID: 42}})
	if err != nil {
		t.Fatal(err)
	}
	server = httptest.NewServer(httpapi.NewHandlerWithDeviceAuth(pool, postgres.NewStore(pool), auth))
	defer server.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	form := func(path string, values url.Values, cookies ...*http.Cookie) (*http.Response, map[string]any) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+path, strings.NewReader(values.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var result map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&result)
		return resp, result
	}
	get := func(path string, cookies ...*http.Cookie) (*http.Response, string) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, string(body)
	}
	start, data := form("/oauth/device_authorization", url.Values{
		"client_id": {"agent-control-plane"}, "client_label": {"Claude on laptop"},
		"scope": {"context:read work:write"},
	})
	if start.StatusCode != 200 || data["verification_uri"] != "http://localhost:8080/activate" {
		t.Fatalf("start = %d %v", start.StatusCode, data)
	}
	deviceCode := data["device_code"].(string)
	userCode := data["user_code"].(string)
	exchange := url.Values{"client_id": {"agent-control-plane"},
		"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {deviceCode}}
	pending, data := form("/oauth/token", exchange)
	if pending.StatusCode != 400 || data["error"] != "authorization_pending" {
		t.Fatalf("before approval = %d %v", pending.StatusCode, data)
	}
	login, _ := get("/auth/github/start")
	if login.StatusCode != http.StatusFound || len(login.Cookies()) != 1 {
		t.Fatalf("login redirect = %d %v", login.StatusCode, login.Cookies())
	}
	state := login.Cookies()[0]
	callback, _ := get("/auth/github/callback?code=valid&state="+url.QueryEscape(state.Value), state)
	if callback.StatusCode != http.StatusSeeOther || len(callback.Cookies()) < 2 {
		t.Fatalf("callback = %d %v", callback.StatusCode, callback.Cookies())
	}
	var session *http.Cookie
	for _, cookie := range callback.Cookies() {
		if cookie.Name == "hub_owner_session" {
			session = cookie
		}
	}
	if session == nil {
		t.Fatal("callback omitted owner session")
	}
	_, page := get("/activate", session)
	csrfMatch := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(page)
	if len(csrfMatch) != 2 {
		t.Fatal("activation page omitted CSRF token")
	}
	csrf := csrfMatch[1]
	lookup, _ := form("/activate/lookup", url.Values{"csrf": {csrf}, "code": {userCode}}, session)
	if lookup.StatusCode != 200 {
		t.Fatalf("lookup = %d", lookup.StatusCode)
	}
	bad, _ := form("/activate/decision", url.Values{
		"csrf": {"wrong"}, "code": {userCode}, "decision": {"approve"},
	}, session)
	if bad.StatusCode != 403 {
		t.Fatalf("missing CSRF defense = %d", bad.StatusCode)
	}
	approved, _ := form("/activate/decision", url.Values{
		"csrf": {csrf}, "code": {userCode}, "decision": {"approve"},
	}, session)
	if approved.StatusCode != 200 {
		t.Fatalf("approval = %d", approved.StatusCode)
	}
	var approvalRequestID string
	if err := pool.QueryRow(ctx, `SELECT request_id FROM audit_events
		WHERE action = 'device.approved'`).Scan(&approvalRequestID); err != nil || approvalRequestID == "" {
		t.Fatalf("approval audit omitted request ID: %q, %v", approvalRequestID, err)
	}
	tokenResponse, tokens := form("/oauth/token", exchange)
	if tokenResponse.StatusCode != 200 || tokens["access_token"] == "" || tokens["refresh_token"] == "" {
		t.Fatalf("exchange = %d %v", tokenResponse.StatusCode, tokens)
	}
	access := tokens["access_token"].(string)
	refresh := tokens["refresh_token"].(string)
	consumed, data := form("/oauth/token", exchange)
	if consumed.StatusCode != 400 || data["error"] != "expired_token" {
		t.Fatalf("reused device code = %d %v", consumed.StatusCode, data)
	}
	apiGet := func(token string) int {
		t.Helper()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/v1/spaces", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	if got := apiGet(access); got != 200 {
		t.Fatalf("new access token status = %d", got)
	}
	refreshForm := url.Values{"client_id": {"agent-control-plane"},
		"grant_type": {"refresh_token"}, "refresh_token": {refresh}}
	rotated, second := form("/oauth/token", refreshForm)
	if rotated.StatusCode != 200 || second["refresh_token"] == refresh {
		t.Fatalf("refresh = %d %v", rotated.StatusCode, second)
	}
	if apiGet(access) != 401 || apiGet(second["access_token"].(string)) != 200 {
		t.Fatal("access token rotation did not replace old token")
	}
	if _, err := pool.Exec(ctx, `UPDATE device_refresh_used SET used_at = now() - interval '91 days'`); err != nil {
		t.Fatal(err)
	}
	replayed, data := form("/oauth/token", refreshForm)
	if replayed.StatusCode != 400 || data["error"] != "invalid_grant" {
		t.Fatalf("refresh replay = %d %v", replayed.StatusCode, data)
	}
	if got := apiGet(second["access_token"].(string)); got != 401 {
		t.Fatalf("replay did not revoke device: %d", got)
	}
	deniedStart, deniedCode := form("/oauth/device_authorization", url.Values{
		"client_id": {"agent-control-plane"}, "client_label": {"Unknown agent"}, "scope": {"context:read"},
	})
	if deniedStart.StatusCode != 200 {
		t.Fatalf("second start = %d", deniedStart.StatusCode)
	}
	deniedUserCode := deniedCode["user_code"].(string)
	deniedDecision, _ := form("/activate/decision", url.Values{
		"csrf": {csrf}, "code": {deniedUserCode}, "decision": {"deny"},
	}, session)
	if deniedDecision.StatusCode != 200 {
		t.Fatalf("deny = %d", deniedDecision.StatusCode)
	}
	deniedExchange := url.Values{"client_id": {"agent-control-plane"},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {deniedCode["device_code"].(string)}}
	denied, data := form("/oauth/token", deniedExchange)
	if denied.StatusCode != 400 || data["error"] != "access_denied" {
		t.Fatalf("denied exchange = %d %v", denied.StatusCode, data)
	}
	thirdStart, thirdCode := form("/oauth/device_authorization", url.Values{
		"client_id": {"agent-control-plane"}, "client_label": {"Second laptop"}, "scope": {"context:read"},
	})
	if thirdStart.StatusCode != 200 {
		t.Fatalf("third start = %d", thirdStart.StatusCode)
	}
	thirdDecision, _ := form("/activate/decision", url.Values{
		"csrf": {csrf}, "code": {thirdCode["user_code"].(string)}, "decision": {"approve"},
	}, session)
	if thirdDecision.StatusCode != 200 {
		t.Fatalf("third approval = %d", thirdDecision.StatusCode)
	}
	thirdExchange := url.Values{"client_id": {"agent-control-plane"},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {thirdCode["device_code"].(string)}}
	thirdResponse, thirdTokens := form("/oauth/token", thirdExchange)
	if thirdResponse.StatusCode != 200 {
		t.Fatalf("third exchange = %d", thirdResponse.StatusCode)
	}
	var thirdID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM client_tokens WHERE display_name = 'Second laptop'`).Scan(&thirdID); err != nil {
		t.Fatal(err)
	}
	devicePage, html := get("/devices", session)
	if devicePage.StatusCode != 200 || !strings.Contains(html, "Second laptop") {
		t.Fatalf("device list = %d %s", devicePage.StatusCode, html)
	}
	revoked, _ := form("/devices/revoke", url.Values{"csrf": {csrf}, "id": {thirdID}}, session)
	if revoked.StatusCode != http.StatusSeeOther {
		t.Fatalf("revoke = %d", revoked.StatusCode)
	}
	if got := apiGet(thirdTokens["access_token"].(string)); got != 401 {
		t.Fatalf("revoked access still works: %d", got)
	}
	blockedRefresh, data := form("/oauth/token", url.Values{"client_id": {"agent-control-plane"},
		"grant_type": {"refresh_token"}, "refresh_token": {thirdTokens["refresh_token"].(string)}})
	if blockedRefresh.StatusCode != 400 || data["error"] != "invalid_grant" {
		t.Fatalf("revoked refresh = %d %v", blockedRefresh.StatusCode, data)
	}
}
