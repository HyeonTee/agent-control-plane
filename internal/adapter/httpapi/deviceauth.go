package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	appdevice "github.com/HyeonTee/agent-control-plane/internal/application/deviceauth"
	deviceauth "github.com/HyeonTee/agent-control-plane/internal/domain/deviceauth"
	model "github.com/HyeonTee/agent-control-plane/internal/domain/work"
)

const deviceClientID = "agent-control-plane"

type DeviceAuthConfig struct {
	PublicURL    string
	ClientID     string
	ClientSecret string
	OwnerID      int64
}

type DeviceAuthHandler struct {
	backend *appdevice.Service
	config  DeviceAuthConfig
	client  *http.Client
	key     []byte
}

func NewDeviceAuthHandler(backend *appdevice.Service, cfg DeviceAuthConfig, client *http.Client) (*DeviceAuthHandler, error) {
	base, err := url.Parse(cfg.PublicURL)
	if backend == nil || err != nil || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") ||
		base.Path != "" || base.RawQuery != "" || base.Fragment != "" || cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.OwnerID <= 0 {
		return nil, errors.New("invalid device authorization configuration")
	}
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return &DeviceAuthHandler{backend: backend, config: cfg, client: client, key: key}, nil
}

func (h *DeviceAuthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /oauth/device_authorization", h.startDevice)
	mux.HandleFunc("POST /oauth/token", h.token)
	mux.HandleFunc("GET /activate", h.activate)
	mux.HandleFunc("GET /auth/github/start", h.githubStart)
	mux.HandleFunc("GET /auth/github/callback", h.githubCallback)
	mux.HandleFunc("POST /activate/decision", h.decision)
	mux.HandleFunc("GET /devices", h.devices)
	mux.HandleFunc("POST /devices/revoke", h.revokeDevice)
}

func deviceHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func oauthError(w http.ResponseWriter, code string) {
	deviceHeaders(w)
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": code})
}

func parseDeviceForm(w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "form encoding required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_form", "invalid form")
		return false
	}
	return true
}

func singleForm(r *http.Request, key string) string {
	values := r.PostForm[key]
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

func (h *DeviceAuthHandler) startDevice(w http.ResponseWriter, r *http.Request) {
	deviceHeaders(w)
	if !parseDeviceForm(w, r) {
		return
	}
	if singleForm(r, "client_id") != deviceClientID {
		oauthError(w, "invalid_client")
		return
	}
	scopes := strings.Fields(singleForm(r, "scope"))
	started, err := h.backend.StartDevice(r.Context(), singleForm(r, "client_label"), scopes)
	if errors.Is(err, model.ErrInvalid) {
		oauthError(w, "invalid_scope")
		return
	}
	if err != nil {
		slog.Error("device authorization start failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_code": started.DeviceCode, "user_code": started.UserCode,
		"verification_uri":          h.config.PublicURL + "/activate",
		"verification_uri_complete": h.config.PublicURL + "/activate?code=" + url.QueryEscape(started.UserCode),
		"expires_in":                600, "interval": 5,
	})
}

func (h *DeviceAuthHandler) token(w http.ResponseWriter, r *http.Request) {
	deviceHeaders(w)
	if !parseDeviceForm(w, r) {
		return
	}
	if singleForm(r, "client_id") != deviceClientID {
		oauthError(w, "invalid_client")
		return
	}
	var result deviceauth.Tokens
	var err error
	switch singleForm(r, "grant_type") {
	case "urn:ietf:params:oauth:grant-type:device_code":
		result, err = h.backend.ExchangeDevice(r.Context(), singleForm(r, "device_code"))
	case "refresh_token":
		result, err = h.backend.RefreshDevice(r.Context(), singleForm(r, "refresh_token"))
	default:
		oauthError(w, "unsupported_grant_type")
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, deviceauth.ErrPending):
			oauthError(w, "authorization_pending")
		case errors.Is(err, deviceauth.ErrSlowDown):
			oauthError(w, "slow_down")
		case errors.Is(err, deviceauth.ErrDenied):
			oauthError(w, "access_denied")
		case errors.Is(err, deviceauth.ErrExpired):
			oauthError(w, "expired_token")
		case errors.Is(err, model.ErrUnauthorized), errors.Is(err, deviceauth.ErrRefreshReplay):
			oauthError(w, "invalid_grant")
		default:
			slog.Error("device token request failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *DeviceAuthHandler) secureCookie() bool {
	return strings.HasPrefix(h.config.PublicURL, "https://")
}

func (h *DeviceAuthHandler) githubStart(w http.ResponseWriter, r *http.Request) {
	deviceHeaders(w)
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		http.Error(w, "request failed", http.StatusInternalServerError)
		return
	}
	state := base64.RawURLEncoding.EncodeToString(stateBytes)
	http.SetCookie(w, &http.Cookie{Name: "hub_oauth_state", Value: state, Path: "/auth/github",
		HttpOnly: true, Secure: h.secureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: 300})
	query := url.Values{
		"client_id": {h.config.ClientID}, "redirect_uri": {h.config.PublicURL + "/auth/github/callback"},
		"state": {state}, "scope": {"read:user"},
	}
	http.Redirect(w, r, "https://github.com/login/oauth/authorize?"+query.Encode(), http.StatusFound)
}

func (h *DeviceAuthHandler) githubCallback(w http.ResponseWriter, r *http.Request) {
	deviceHeaders(w)
	stateCookie, err := r.Cookie("hub_oauth_state")
	if err != nil || len(stateCookie.Value) != 43 || len(r.URL.Query()["state"]) != 1 || len(r.URL.Query()["code"]) != 1 ||
		len(r.URL.Query().Get("state")) != 43 || r.URL.Query().Get("code") == "" ||
		!hmac.Equal([]byte(stateCookie.Value), []byte(r.URL.Query().Get("state"))) {
		h.page(w, http.StatusBadRequest, activationView{Message: "Your GitHub sign-in took too long or was already used. Enter the code to try again."})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "hub_oauth_state", Path: "/auth/github", MaxAge: -1,
		HttpOnly: true, Secure: h.secureCookie(), SameSite: http.SameSiteLaxMode})
	githubToken, err := h.exchangeGitHubCode(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		slog.Warn("GitHub login exchange failed", "error", err)
		h.page(w, http.StatusBadGateway, activationView{Message: "GitHub sign-in failed. Enter the code to try again."})
		return
	}
	id, err := h.githubUserID(r.Context(), githubToken)
	if err != nil {
		slog.Warn("GitHub identity check failed", "error", err)
		h.page(w, http.StatusBadGateway, activationView{Message: "GitHub sign-in failed. Enter the code to try again."})
		return
	}
	if id != h.config.OwnerID {
		h.page(w, http.StatusForbidden, activationView{Done: true,
			Message: "This GitHub account is not allowed to approve agents for this Hub. Sign out of GitHub and use the owner account."})
		return
	}
	nonceBytes := make([]byte, 24)
	if _, err := rand.Read(nonceBytes); err != nil {
		h.page(w, http.StatusInternalServerError, activationView{Message: "Something went wrong. Try again."})
		return
	}
	payload := fmt.Sprintf("%d:%s", time.Now().Add(30*time.Minute).Unix(), base64.RawURLEncoding.EncodeToString(nonceBytes))
	mac := hmac.New(sha256.New, h.key)
	mac.Write([]byte(payload))
	value := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	// Lax, not Strict: browsers withhold Strict cookies on the redirect that
	// ends GitHub's cross-site login, which left the owner signed out on
	// arrival. State-changing forms still require the CSRF token.
	http.SetCookie(w, &http.Cookie{Name: "hub_owner_session", Value: value, Path: "/",
		HttpOnly: true, Secure: h.secureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: 1800})
	target := "/activate"
	if pending, err := r.Cookie("hub_pending_code"); err == nil {
		if code := deviceauth.NormalizeUserCode(pending.Value); code != "" {
			target += "?code=" + url.QueryEscape(code)
		}
		http.SetCookie(w, &http.Cookie{Name: "hub_pending_code", Path: "/auth/github", MaxAge: -1,
			HttpOnly: true, Secure: h.secureCookie(), SameSite: http.SameSiteLaxMode})
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (h *DeviceAuthHandler) exchangeGitHubCode(ctx context.Context, code string) (string, error) {
	form := url.Values{"client_id": {h.config.ClientID}, "client_secret": {h.config.ClientSecret},
		"code": {code}, "redirect_uri": {h.config.PublicURL + "/auth/github/callback"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.New("GitHub token endpoint rejected code")
	}
	var body struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body); err != nil || body.Error != "" || body.AccessToken == "" {
		return "", errors.New("GitHub token response invalid")
	}
	return body.AccessToken, nil
}

func (h *DeviceAuthHandler) githubUserID(ctx context.Context, token string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "agent-control-plane")
	resp, err := h.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, errors.New("GitHub user endpoint rejected token")
	}
	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&body); err != nil || body.ID <= 0 {
		return 0, errors.New("GitHub user response invalid")
	}
	return body.ID, nil
}

func (h *DeviceAuthHandler) sessionNonce(r *http.Request) string {
	cookie, err := r.Cookie("hub_owner_session")
	if err != nil {
		return ""
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ""
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	mac := hmac.New(sha256.New, h.key)
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return ""
	}
	values := strings.Split(string(payload), ":")
	if len(values) != 2 {
		return ""
	}
	expires, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil || time.Now().Unix() >= expires {
		return ""
	}
	return values[1]
}

func (h *DeviceAuthHandler) csrf(nonce string) string {
	mac := hmac.New(sha256.New, h.key)
	mac.Write([]byte("csrf:" + nonce))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

type activationView struct {
	Authenticated bool
	CSRF          string
	Code          string
	Label         string
	Scopes        string
	Message       string
	Confirm       bool
	Done          bool
}

var activationTemplate = template.Must(template.New("activate").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Connect an agent</title><style>body{font:16px system-ui,sans-serif;max-width:36rem;margin:4rem auto;padding:0 1rem;color:#17212b}input,button{font:inherit;padding:.65rem}input{width:11rem;text-transform:uppercase;letter-spacing:.08em}button{cursor:pointer}main{border:1px solid #ccd5dd;border-radius:12px;padding:1.5rem}p{line-height:1.5}.actions{display:flex;gap:.7rem}.status{padding:.75rem 1rem;border-radius:8px;background:#eef3f7}.code{font:600 1.5rem ui-monospace,monospace;letter-spacing:.1em}</style></head>
<body><main><h1>Connect an agent</h1>
{{if .Message}}<p class="status" role="status">{{.Message}}</p>{{end}}
{{if .Confirm}}<p><strong>{{.Label}}</strong> requests access to your personal space.</p><p>Requested permissions: <strong>{{.Scopes}}</strong>.</p><p>Code: <span class="code">{{.Code}}</span></p><p>Only approve if this matches the code shown by your agent and you started this request.</p>
<form method="post" action="/activate/decision"><input type="hidden" name="csrf" value="{{.CSRF}}"><input type="hidden" name="code" value="{{.Code}}"><div class="actions"><button name="decision" value="approve">Approve</button><button name="decision" value="deny">Deny</button></div></form>
{{else if not .Done}}<p>Enter the one-time code shown by your agent. It expires after ten minutes.{{if not .Authenticated}} You will sign in with GitHub next.{{end}}</p>
<form method="get" action="/activate"><label>Code <input name="code" autocomplete="off" autocapitalize="characters" spellcheck="false" required placeholder="ABCD-EFGH"></label> <button>Continue</button></form>{{end}}
{{if .Authenticated}}<p><a href="/devices">Manage connected devices</a></p>{{else if not .Done}}<p><a href="/auth/github/start">Sign in to manage connected devices</a></p>{{end}}
</main></body></html>`))

type devicesView struct {
	CSRF    string
	Devices []deviceauth.Client
}

var devicesTemplate = template.Must(template.New("devices").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Connected devices</title><style>body{font:16px system-ui,sans-serif;max-width:42rem;margin:4rem auto;padding:0 1rem;color:#17212b}li{margin:1.5rem 0}button{font:inherit;padding:.5rem;cursor:pointer}</style></head>
<body><main><h1>Connected devices</h1><p><a href="/activate">Connect another agent</a></p>
{{if not .Devices}}<p>No connected devices.</p>{{else}}<ul>{{range .Devices}}<li><strong>{{.Label}}</strong> — {{range .Scopes}}{{.}} {{end}}<br>
{{if .RevokedAt}}Revoked{{else}}Refresh expires {{.ExpiresAt.Format "2006-01-02"}}<form method="post" action="/devices/revoke"><input type="hidden" name="csrf" value="{{$.CSRF}}"><input type="hidden" name="id" value="{{.ID}}"><button>Revoke this device</button></form>{{end}}</li>{{end}}</ul>{{end}}
</main></body></html>`))

func (h *DeviceAuthHandler) page(w http.ResponseWriter, status int, view activationView) {
	deviceHeaders(w)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := activationTemplate.Execute(w, view); err != nil {
		slog.Error("activation page failed", "error", err)
	}
}

// activate serves the code form and, given ?code=, the approval screen. The
// code may arrive from verification_uri_complete before the owner has signed
// in; it is then held in a short-lived cookie across the GitHub login.
func (h *DeviceAuthHandler) activate(w http.ResponseWriter, r *http.Request) {
	nonce := h.sessionNonce(r)
	view := activationView{Authenticated: nonce != "", CSRF: h.csrf(nonce)}
	if !r.URL.Query().Has("code") {
		h.page(w, http.StatusOK, view)
		return
	}
	code := deviceauth.NormalizeUserCode(r.URL.Query().Get("code"))
	if code == "" {
		view.Message = "That does not look like a code. Codes have eight letters and digits, like ABCD-EFGH."
		h.page(w, http.StatusOK, view)
		return
	}
	if nonce == "" {
		http.SetCookie(w, &http.Cookie{Name: "hub_pending_code", Value: code, Path: "/auth/github",
			HttpOnly: true, Secure: h.secureCookie(), SameSite: http.SameSiteLaxMode, MaxAge: 600})
		http.Redirect(w, r, "/auth/github/start", http.StatusSeeOther)
		return
	}
	device, err := h.backend.FindDevice(r.Context(), code)
	if errors.Is(err, model.ErrNotFound) {
		view.Message = "Code " + code + " was not found or has expired. Ask your agent for a new code."
		h.page(w, http.StatusOK, view)
		return
	}
	if err != nil {
		slog.Error("activation lookup failed", "error", err)
		view.Message = "Something went wrong. Try again."
		h.page(w, http.StatusInternalServerError, view)
		return
	}
	view.Confirm, view.Code, view.Label, view.Scopes = true, device.UserCode, device.Label, strings.Join(device.Scopes, ", ")
	h.page(w, http.StatusOK, view)
}

func (h *DeviceAuthHandler) browserForm(w http.ResponseWriter, r *http.Request) (string, bool) {
	nonce := h.sessionNonce(r)
	if nonce == "" {
		http.Redirect(w, r, "/activate", http.StatusSeeOther)
		return "", false
	}
	deviceHeaders(w)
	if !parseDeviceForm(w, r) {
		return "", false
	}
	if !hmac.Equal([]byte(singleForm(r, "csrf")), []byte(h.csrf(nonce))) {
		h.page(w, http.StatusForbidden, activationView{Authenticated: true, CSRF: h.csrf(nonce),
			Message: "This form expired. Enter the code again."})
		return "", false
	}
	return nonce, true
}

func (h *DeviceAuthHandler) decision(w http.ResponseWriter, r *http.Request) {
	nonce, ok := h.browserForm(w, r)
	if !ok {
		return
	}
	decision := singleForm(r, "decision")
	if decision != "approve" && decision != "deny" {
		h.page(w, http.StatusBadRequest, activationView{Authenticated: true, CSRF: h.csrf(nonce),
			Message: "Choose Approve or Deny."})
		return
	}
	err := h.backend.DecideDevice(r.Context(), singleForm(r, "code"), decision == "approve")
	if errors.Is(err, model.ErrNotFound) {
		h.page(w, http.StatusNotFound, activationView{Authenticated: true, CSRF: h.csrf(nonce),
			Message: "That code was not found or has expired. Ask your agent for a new code."})
		return
	}
	if err != nil {
		slog.Error("activation decision failed", "error", err)
		h.page(w, http.StatusInternalServerError, activationView{Authenticated: true, CSRF: h.csrf(nonce),
			Message: "Something went wrong. Try again."})
		return
	}
	message := "Connection denied. You can return to your agent."
	if decision == "approve" {
		message = "Connection approved. You can return to your agent."
	}
	h.page(w, http.StatusOK, activationView{Authenticated: true, Done: true, Message: message})
}

func (h *DeviceAuthHandler) devices(w http.ResponseWriter, r *http.Request) {
	nonce := h.sessionNonce(r)
	if nonce == "" {
		http.Redirect(w, r, "/activate", http.StatusSeeOther)
		return
	}
	devices, err := h.backend.ListDevices(r.Context())
	if err != nil {
		slog.Error("device list failed", "error", err)
		http.Error(w, "request failed", http.StatusInternalServerError)
		return
	}
	deviceHeaders(w)
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := devicesTemplate.Execute(w, devicesView{CSRF: h.csrf(nonce), Devices: devices}); err != nil {
		slog.Error("device list page failed", "error", err)
	}
}

func (h *DeviceAuthHandler) revokeDevice(w http.ResponseWriter, r *http.Request) {
	_, ok := h.browserForm(w, r)
	if !ok {
		return
	}
	err := h.backend.RevokeDevice(r.Context(), singleForm(r, "id"))
	if errors.Is(err, model.ErrNotFound) {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	if err != nil {
		slog.Error("device revoke failed", "error", err)
		http.Error(w, "request failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/devices", http.StatusSeeOther)
}
