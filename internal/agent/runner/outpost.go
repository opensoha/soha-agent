package runner

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	sohaapi "github.com/opensoha/soha-contracts/gen/go/sohaapi"
	"go.uber.org/zap"
)

const maxOutpostEventAttempts = 3
const outpostConfigClockSkew = time.Minute

var (
	errOutpostUnavailable = errors.New("outpost configuration unavailable")
	errOutpostDenied      = errors.New("outpost access denied")
)

type ForwardAuthRequest struct {
	Method       string
	OriginalURL  string
	RequestHost  string
	RequestPath  string
	SessionToken string
	SourceIP     string
}

type ForwardAuthResult struct {
	Decision    string
	Headers     map[string]string
	RedirectURL string
	StatusCode  int
}

type OutpostStatus struct {
	Enabled              bool   `json:"enabled"`
	Ready                bool   `json:"ready"`
	OutpostID            string `json:"outpostId,omitempty"`
	ConfigurationVersion int64  `json:"configurationVersion,omitempty"`
	DesiredVersion       int64  `json:"desiredConfigurationVersion,omitempty"`
	ExpiresAt            string `json:"expiresAt,omitempty"`
	LastClaimAt          string `json:"lastClaimAt,omitempty"`
	LastHeartbeatAt      string `json:"lastHeartbeatAt,omitempty"`
	ClaimFailures        int64  `json:"claimFailures"`
	HeartbeatFailures    int64  `json:"heartbeatFailures"`
	CheckFailures        int64  `json:"checkFailures"`
	Denied               int64  `json:"denied"`
}

type outpostRuntime struct {
	mu                sync.RWMutex
	config            *sohaapi.IdentityOutpostRuntimeConfig
	publicKey         ed25519.PublicKey
	lastClaimAt       time.Time
	lastHeartbeatAt   time.Time
	claimFailures     int64
	heartbeatFailures int64
	checkFailures     int64
	denied            int64
	desiredVersion    int64
}

func newOutpostRuntime(encodedPublicKey string) (*outpostRuntime, error) {
	key, err := decodeBase64(strings.TrimSpace(encodedPublicKey))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("decode outpost trust public key: expected %d-byte Ed25519 key", ed25519.PublicKeySize)
	}
	return &outpostRuntime{publicKey: ed25519.PublicKey(key)}, nil
}

func (r *Runner) startOutpost(ctx context.Context) {
	if !r.cfg.Outpost.Enabled {
		return
	}
	runtime, err := newOutpostRuntime(r.cfg.Outpost.TrustPublicKey)
	if err != nil {
		r.logger.Error("outpost runtime disabled", zap.Error(err))
		return
	}
	r.outpost = runtime
	go r.outpostLoop(ctx)
}

func (r *Runner) outpostLoop(ctx context.Context) {
	poll := positiveDuration(r.cfg.Outpost.PollInterval, 5*time.Second)
	heartbeat := positiveDuration(r.cfg.Outpost.HeartbeatInterval, 15*time.Second)
	pollTicker := time.NewTicker(poll)
	heartbeatTicker := time.NewTicker(heartbeat)
	defer pollTicker.Stop()
	defer heartbeatTicker.Stop()
	r.claimOutpostConfig(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-pollTicker.C:
			r.claimOutpostConfig(ctx)
		case <-heartbeatTicker.C:
			r.heartbeatOutpost(ctx)
		}
	}
}

func (r *Runner) claimOutpostConfig(ctx context.Context) {
	current := r.OutpostStatus().ConfigurationVersion
	config, err := r.apiClient().ClaimIdentityOutpostConfig(ctx, sohaapi.IdentityOutpostClaimRequest{
		AgentID: r.cfg.Outpost.AgentID, CurrentConfigurationVersion: current,
		SupportedProtocolVersion: r.cfg.Outpost.ProtocolVersion,
	})
	if err != nil {
		r.outpost.markClaimFailure()
		return
	}
	if config == nil {
		return
	}
	if err := r.outpost.apply(*config, r.cfg.Outpost.TrustKeyID, time.Now()); err != nil {
		r.outpost.markClaimFailure()
		return
	}
	r.reportOutpostEvent(ctx, sohaapi.ConfigurationApplied, "configuration_applied", "")
}

func (r *Runner) heartbeatOutpost(ctx context.Context) {
	status := r.OutpostStatus()
	if status.OutpostID == "" {
		return
	}
	state := sohaapi.IdentityOutpostHeartbeatRequestStatusHealthy
	errorCode := ""
	if !status.Ready {
		state = sohaapi.IdentityOutpostHeartbeatRequestStatusUnavailable
		errorCode = "configuration_unavailable"
	}
	response, err := r.apiClient().HeartbeatIdentityOutpost(ctx, status.OutpostID, sohaapi.IdentityOutpostHeartbeatRequest{
		AgentID: r.cfg.Outpost.AgentID, CheckedAt: time.Now().UTC(), ConfigurationVersion: status.ConfigurationVersion,
		Status: state, ErrorCode: errorCode,
	})
	if err == nil {
		r.outpost.markDesiredVersion(response.DesiredConfigurationVersion)
	}
	if err != nil || !response.Accepted {
		r.outpost.markHeartbeatFailure()
		return
	}
	r.outpost.markHeartbeat()
	if response.DesiredConfigurationVersion > status.ConfigurationVersion {
		r.claimOutpostConfig(ctx)
	}
}

func (r *Runner) CheckOutpostAccess(ctx context.Context, request ForwardAuthRequest) (ForwardAuthResult, error) {
	if r == nil || r.outpost == nil {
		return ForwardAuthResult{Decision: "deny", StatusCode: http.StatusServiceUnavailable}, errOutpostUnavailable
	}
	config, route, skip, err := r.outpost.route(request.RequestHost, request.RequestPath, time.Now())
	if err != nil {
		return ForwardAuthResult{Decision: "deny", StatusCode: http.StatusServiceUnavailable}, err
	}
	if skip {
		return ForwardAuthResult{Decision: "allow", StatusCode: http.StatusNoContent}, nil
	}
	checked, err := r.apiClient().CheckIdentityOutpostAccess(ctx, config.OutpostID, sohaapi.IdentityOutpostAccessCheckRequest{
		ConfigurationVersion: config.ConfigurationVersion, Method: request.Method, OriginalURL: request.OriginalURL,
		ProviderID: route.ProviderID, RequestHost: request.RequestHost, RequestPath: request.RequestPath,
		SessionToken: request.SessionToken, SourceIP: request.SourceIP,
	})
	if err != nil || !validOutpostAccessCheck(checked) {
		r.outpost.markCheckFailure()
		r.reportOutpostEvent(ctx, sohaapi.CheckFailed, "check_failed", "core_check_failed")
		return ForwardAuthResult{Decision: "deny", StatusCode: http.StatusServiceUnavailable}, errOutpostUnavailable
	}
	result := ForwardAuthResult{
		Decision: string(checked.Decision), Headers: cleanOutpostHeaders(checked.Headers),
		RedirectURL: checked.RedirectURL, StatusCode: checked.StatusCode,
	}
	if checked.Decision == sohaapi.IdentityOutpostAccessCheckDecisionAllow {
		return result, nil
	}
	r.outpost.markDenied()
	r.reportOutpostEvent(ctx, sohaapi.CheckDenied, "check_denied", checked.Reason)
	return result, errOutpostDenied
}

func validOutpostAccessCheck(checked sohaapi.IdentityOutpostAccessCheck) bool {
	if !checked.Decision.Valid() {
		return false
	}
	switch checked.Decision {
	case sohaapi.IdentityOutpostAccessCheckDecisionAllow:
		return checked.StatusCode >= 200 && checked.StatusCode < 300
	case sohaapi.IdentityOutpostAccessCheckDecisionDeny:
		return checked.StatusCode >= 400 && checked.StatusCode < 500
	case sohaapi.IdentityOutpostAccessCheckDecisionRedirect:
		if checked.StatusCode < 300 || checked.StatusCode >= 400 {
			return false
		}
		redirect := strings.TrimSpace(checked.RedirectURL)
		if strings.HasPrefix(redirect, "/") {
			return !strings.HasPrefix(redirect, "//")
		}
		parsed, err := url.Parse(redirect)
		return err == nil && parsed.Scheme == "https" && parsed.Host != ""
	default:
		return false
	}
}

func (r *Runner) OutpostStatus() OutpostStatus {
	if r == nil || !r.cfg.Outpost.Enabled {
		return OutpostStatus{}
	}
	if r.outpost == nil {
		return OutpostStatus{Enabled: true}
	}
	return r.outpost.status(time.Now())
}

func (r *Runner) reportOutpostEvent(ctx context.Context, eventType sohaapi.IdentityOutpostRuntimeEventType, code, message string) {
	status := r.OutpostStatus()
	if status.OutpostID == "" {
		return
	}
	request := sohaapi.IdentityOutpostEventBatchRequest{AgentID: r.cfg.Outpost.AgentID, Events: []sohaapi.IdentityOutpostRuntimeEvent{{
		ID: uuid.NewString(), Type: eventType, Code: code, Message: truncate(message, 256),
		ConfigurationVersion: status.ConfigurationVersion, OccurredAt: time.Now().UTC(),
	}}}
	for attempt := 0; attempt < maxOutpostEventAttempts; attempt++ {
		if _, err := r.apiClient().AppendIdentityOutpostEvents(ctx, status.OutpostID, request); err == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(attempt+1) * 100 * time.Millisecond):
		}
	}
}

func (o *outpostRuntime) apply(config sohaapi.IdentityOutpostRuntimeConfig, trustKeyID string, now time.Time) error {
	if strings.TrimSpace(config.KeyID) != strings.TrimSpace(trustKeyID) {
		return errors.New("outpost configuration uses an untrusted key")
	}
	if strings.TrimSpace(config.OutpostID) == "" || config.ProtocolVersion != "v1" || strings.TrimSpace(config.CheckURL) == "" ||
		config.ConfigurationVersion <= 0 || !config.IssuedAt.Before(config.ExpiresAt) || config.IssuedAt.After(now.Add(outpostConfigClockSkew)) || !now.Before(config.ExpiresAt) {
		return errors.New("outpost configuration is malformed or expired")
	}
	for _, route := range config.Routes {
		if strings.TrimSpace(route.Host) == "" || strings.TrimSpace(route.ProviderID) == "" ||
			(strings.TrimSpace(route.PathPrefix) != "" && !strings.HasPrefix(route.PathPrefix, "/")) {
			return errors.New("outpost configuration contains an invalid route")
		}
		for _, skipPath := range route.SkipPaths {
			if !strings.HasPrefix(strings.TrimSpace(skipPath), "/") {
				return errors.New("outpost configuration contains an invalid skip path")
			}
		}
	}
	signature, err := decodeBase64(config.Signature)
	if err != nil || !ed25519.Verify(o.publicKey, outpostConfigPayload(config), signature) {
		return errors.New("outpost configuration signature is invalid")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.config != nil {
		if config.ConfigurationVersion < o.config.ConfigurationVersion {
			return errors.New("outpost configuration version is not monotonic")
		}
		if config.ConfigurationVersion == o.config.ConfigurationVersion && !config.ExpiresAt.After(o.config.ExpiresAt) {
			return errors.New("outpost configuration lease is not newer")
		}
	}
	copy := config
	o.config = &copy
	o.desiredVersion = config.ConfigurationVersion
	o.lastClaimAt = now.UTC()
	return nil
}

func outpostConfigPayload(config sohaapi.IdentityOutpostRuntimeConfig) []byte {
	payload := struct {
		OutpostID            string                         `json:"outpostId"`
		ProtocolVersion      string                         `json:"protocolVersion"`
		ConfigurationVersion int64                          `json:"configurationVersion"`
		IssuedAt             time.Time                      `json:"issuedAt"`
		ExpiresAt            time.Time                      `json:"expiresAt"`
		KeyID                string                         `json:"keyId"`
		CheckURL             string                         `json:"checkUrl"`
		Routes               []sohaapi.IdentityOutpostRoute `json:"routes"`
	}{config.OutpostID, config.ProtocolVersion, config.ConfigurationVersion, config.IssuedAt, config.ExpiresAt, config.KeyID, config.CheckURL, config.Routes}
	encoded, _ := json.Marshal(payload)
	return encoded
}

func (o *outpostRuntime) route(host, requestPath string, now time.Time) (sohaapi.IdentityOutpostRuntimeConfig, sohaapi.IdentityOutpostRoute, bool, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if o.config == nil || !now.Before(o.config.ExpiresAt) {
		return sohaapi.IdentityOutpostRuntimeConfig{}, sohaapi.IdentityOutpostRoute{}, false, errOutpostUnavailable
	}
	host = strings.ToLower(strings.TrimSpace(strings.Split(host, ":")[0]))
	cleanPath := path.Clean("/" + strings.TrimPrefix(requestPath, "/"))
	var selected *sohaapi.IdentityOutpostRoute
	for _, route := range o.config.Routes {
		prefix := path.Clean(firstNonEmpty(strings.TrimSpace(route.PathPrefix), "/"))
		if strings.EqualFold(host, strings.TrimSpace(route.Host)) && hasPathPrefix(cleanPath, prefix) {
			if selected == nil || len(prefix) > len(path.Clean(firstNonEmpty(selected.PathPrefix, "/"))) {
				copy := route
				selected = &copy
			}
		}
	}
	if selected != nil {
		for _, skipPath := range selected.SkipPaths {
			if hasPathPrefix(cleanPath, path.Clean(skipPath)) {
				return *o.config, *selected, true, nil
			}
		}
		return *o.config, *selected, false, nil
	}
	return sohaapi.IdentityOutpostRuntimeConfig{}, sohaapi.IdentityOutpostRoute{}, false, errOutpostUnavailable
}

func (o *outpostRuntime) status(now time.Time) OutpostStatus {
	o.mu.RLock()
	defer o.mu.RUnlock()
	status := OutpostStatus{Enabled: true, ClaimFailures: o.claimFailures, HeartbeatFailures: o.heartbeatFailures, CheckFailures: o.checkFailures, Denied: o.denied}
	status.DesiredVersion = o.desiredVersion
	if o.config != nil {
		status.Ready = now.Before(o.config.ExpiresAt)
		status.OutpostID = o.config.OutpostID
		status.ConfigurationVersion = o.config.ConfigurationVersion
		status.ExpiresAt = o.config.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if !o.lastClaimAt.IsZero() {
		status.LastClaimAt = o.lastClaimAt.Format(time.RFC3339)
	}
	if !o.lastHeartbeatAt.IsZero() {
		status.LastHeartbeatAt = o.lastHeartbeatAt.Format(time.RFC3339)
	}
	return status
}

func (o *outpostRuntime) markClaimFailure()     { o.mu.Lock(); o.claimFailures++; o.mu.Unlock() }
func (o *outpostRuntime) markHeartbeatFailure() { o.mu.Lock(); o.heartbeatFailures++; o.mu.Unlock() }
func (o *outpostRuntime) markHeartbeat() {
	o.mu.Lock()
	o.lastHeartbeatAt = time.Now().UTC()
	o.mu.Unlock()
}
func (o *outpostRuntime) markCheckFailure() { o.mu.Lock(); o.checkFailures++; o.mu.Unlock() }
func (o *outpostRuntime) markDenied()       { o.mu.Lock(); o.denied++; o.mu.Unlock() }
func (o *outpostRuntime) markDesiredVersion(version int64) {
	o.mu.Lock()
	o.desiredVersion = version
	o.mu.Unlock()
}

func cleanOutpostHeaders(headers map[string]string) map[string]string {
	allowed := map[string]bool{"x-auth-request-user": true, "x-auth-request-email": true, "x-auth-request-groups": true, "x-soha-user": true, "x-soha-email": true, "x-soha-groups": true}
	clean := make(map[string]string, len(headers))
	for name, value := range headers {
		name = http.CanonicalHeaderKey(strings.TrimSpace(name))
		value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\n", ""))
		if allowed[strings.ToLower(name)] && value != "" && len(value) <= 4096 {
			clean[name] = value
		}
	}
	return clean
}

func hasPathPrefix(requestPath, prefix string) bool {
	return prefix == "/" || requestPath == prefix || strings.HasPrefix(requestPath, strings.TrimSuffix(prefix, "/")+"/")
}

func decodeBase64(value string) ([]byte, error) {
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.DecodeString(strings.TrimSpace(value)); err == nil {
			return decoded, nil
		}
	}
	return nil, errors.New("invalid base64")
}

func positiveDuration(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
func truncate(value string, size int) string {
	if len(value) <= size {
		return value
	}
	return value[:size]
}
