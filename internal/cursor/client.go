package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	apiBase          = "https://api2.cursor.sh"
	usagePath        = "/aiserver.v1.DashboardService/GetCurrentPeriodUsage"
	planPath         = "/aiserver.v1.DashboardService/GetPlanInfo"
	oauthPath        = "/oauth/token"
	defaultClientVer = "3.12.29"
)

type Snapshot struct {
	PlanName     string
	Price        string
	IncludedPct  float64
	AutoPct      float64
	APIPct       float64
	IncludedUSD  float64
	LimitUSD     float64
	BonusUSD     float64
	RemainingUSD float64
	CycleEnd     time.Time
	Message      string
	AutoMessage  string
	APIMessage   string
	LimitHit     bool
	FetchedAt    time.Time
	Err          string
}

type Client struct {
	http     *http.Client
	version  string
	mu       sync.Mutex
	access   string
	refresh  string
	planHint string
}

func NewClient() *Client {
	return &Client{
		http:    &http.Client{Timeout: 20 * time.Second},
		version: detectClientVersion(),
	}
}

func detectClientVersion() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/usr/share/cursor/resources/app/package.json",
		"/opt/Cursor/resources/app/package.json",
		filepath.Join(home, ".local/share/cursor/resources/app/package.json"),
		"/Applications/Cursor.app/Contents/Resources/app/package.json",
	}
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var meta struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(b, &meta) == nil && meta.Version != "" {
			return meta.Version
		}
	}
	return defaultClientVer
}

func (c *Client) Fetch(ctx context.Context) Snapshot {
	if err := c.ensureToken(ctx); err != nil {
		return Snapshot{Err: err.Error(), FetchedAt: time.Now()}
	}

	snap, err := c.fetchOnce(ctx)
	if err != nil && isAuthStatus(err) {
		if rerr := c.refreshAccess(ctx); rerr == nil {
			snap, err = c.fetchOnce(ctx)
		}
	}
	if err != nil {
		return Snapshot{Err: err.Error(), FetchedAt: time.Now()}
	}
	snap.FetchedAt = time.Now()
	return snap
}

func isAuthStatus(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "401") || strings.Contains(msg, "403")
}

func (c *Client) ensureToken(ctx context.Context) error {
	acc, err := readAccount()
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.planHint = acc.Plan
	if acc.RefreshToken != "" {
		c.refresh = acc.RefreshToken
	}

	var expired, needRefresh bool
	switch {
	case c.access != "" && !jwtExpired(c.access, tokenRefreshSkew):
	case acc.AccessToken != "" && !jwtExpired(acc.AccessToken, tokenRefreshSkew):
		c.access = acc.AccessToken
	case c.refresh != "":
		needRefresh = true
	case acc.AccessToken != "":
		c.access = acc.AccessToken
	default:
		expired = true
	}
	c.mu.Unlock()

	if expired {
		return fmt.Errorf("Cursor token expired — sign in again")
	}
	if needRefresh {
		return c.refreshAccess(ctx)
	}
	return nil
}

func (c *Client) refreshAccess(ctx context.Context) error {
	c.mu.Lock()
	refresh := c.refresh
	c.mu.Unlock()
	if refresh == "" {
		acc, err := readAccount()
		if err != nil {
			return err
		}
		refresh = acc.RefreshToken
	}
	if refresh == "" {
		return fmt.Errorf("no refresh token — sign in to Cursor")
	}

	body, _ := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"client_id":     oauthClientID,
		"refresh_token": refresh,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+oauthPath, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("token refresh failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh HTTP %d", resp.StatusCode)
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ShouldLogout bool   `json:"shouldLogout"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("token refresh: bad response")
	}
	if out.ShouldLogout || out.AccessToken == "" {
		return fmt.Errorf("session expired — sign in to Cursor")
	}

	c.mu.Lock()
	c.access = out.AccessToken
	if out.RefreshToken != "" {
		c.refresh = out.RefreshToken
	}
	c.mu.Unlock()
	return nil
}

func (c *Client) fetchOnce(ctx context.Context) (Snapshot, error) {
	usageRaw, err := c.postRPC(ctx, usagePath)
	if err != nil {
		return Snapshot{}, err
	}
	planRaw, _ := c.postRPC(ctx, planPath)

	snap := parseUsage(usageRaw)
	applyPlan(planRaw, &snap)
	c.mu.Lock()
	if snap.PlanName == "" && c.planHint != "" {
		snap.PlanName = strings.ToUpper(c.planHint[:1]) + c.planHint[1:]
	}
	c.mu.Unlock()
	return snap, nil
}

func (c *Client) postRPC(ctx context.Context, path string) ([]byte, error) {
	c.mu.Lock()
	token := c.access
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+path, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Origin", "https://cursor.com")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("offline or unreachable")
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage API HTTP %d", resp.StatusCode)
	}
	return raw, nil
}

func (c *Client) setHeaders(req *http.Request) {
	ua := fmt.Sprintf("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Cursor/%s Chrome/132.0.0.0 Safari/537.36", c.version)
	req.Header.Set("User-Agent", ua)
	req.Header.Set("x-cursor-client-version", c.version)
}

func parseUsage(raw []byte) Snapshot {
	var body struct {
		BillingCycleEnd                  json.RawMessage `json:"billingCycleEnd"`
		DisplayMessage                   string          `json:"displayMessage"`
		AutoModelSelectedDisplayMessage  string          `json:"autoModelSelectedDisplayMessage"`
		NamedModelSelectedDisplayMessage string          `json:"namedModelSelectedDisplayMessage"`
		PlanUsage                        struct {
			IncludedSpend    float64 `json:"includedSpend"`
			BonusSpend       float64 `json:"bonusSpend"`
			Remaining        float64 `json:"remaining"`
			Limit            float64 `json:"limit"`
			AutoPercentUsed  float64 `json:"autoPercentUsed"`
			APIPercentUsed   float64 `json:"apiPercentUsed"`
			TotalPercentUsed float64 `json:"totalPercentUsed"`
		} `json:"planUsage"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return Snapshot{Err: "unexpected usage response"}
	}

	pu := body.PlanUsage
	includedPct := pu.TotalPercentUsed
	if pu.Limit > 0 {
		includedPct = (pu.IncludedSpend / pu.Limit) * 100
	}

	snap := Snapshot{
		IncludedPct:  includedPct,
		AutoPct:      pu.AutoPercentUsed,
		APIPct:       pu.APIPercentUsed,
		IncludedUSD:  pu.IncludedSpend / 100,
		LimitUSD:     pu.Limit / 100,
		BonusUSD:     pu.BonusSpend / 100,
		RemainingUSD: pu.Remaining / 100,
		CycleEnd:     parseMillis(body.BillingCycleEnd),
		Message:      body.DisplayMessage,
		AutoMessage:  body.AutoModelSelectedDisplayMessage,
		APIMessage:   body.NamedModelSelectedDisplayMessage,
		LimitHit:     pu.Limit > 0 && pu.IncludedSpend >= pu.Limit,
	}
	if snap.LimitHit && snap.RemainingUSD < 0 {
		snap.RemainingUSD = 0
	}
	return snap
}

func applyPlan(raw []byte, snap *Snapshot) {
	if len(raw) == 0 {
		return
	}
	var body struct {
		PlanInfo struct {
			PlanName            string          `json:"planName"`
			Price               string          `json:"price"`
			IncludedAmountCents float64         `json:"includedAmountCents"`
			BillingCycleEnd     json.RawMessage `json:"billingCycleEnd"`
		} `json:"planInfo"`
	}
	if json.Unmarshal(raw, &body) != nil {
		return
	}
	snap.PlanName = body.PlanInfo.PlanName
	snap.Price = body.PlanInfo.Price
	if snap.LimitUSD == 0 && body.PlanInfo.IncludedAmountCents > 0 {
		snap.LimitUSD = body.PlanInfo.IncludedAmountCents / 100
		if snap.LimitUSD > 0 {
			snap.IncludedPct = (snap.IncludedUSD / snap.LimitUSD) * 100
		}
	}
	if snap.CycleEnd.IsZero() {
		snap.CycleEnd = parseMillis(body.PlanInfo.BillingCycleEnd)
	}
}

func parseMillis(raw json.RawMessage) time.Time {
	s := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if s == "" || s == "null" {
		return time.Time{}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return time.Time{}
		}
		n = int64(f)
	}
	if n > 1e12 {
		return time.UnixMilli(n)
	}
	return time.Unix(n, 0)
}
