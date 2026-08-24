package cursor

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	oauthClientID    = "KbZUR41cY7W6zRSdpSUJ7I7mLYBKOCmB"
	tokenRefreshSkew = 60 * time.Second
)

type account struct {
	AccessToken  string
	RefreshToken string
	Plan         string
}

func stateDBPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Cursor", "User", "globalStorage", "state.vscdb")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb")
	default:
		return filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb")
	}
}

func readAccount() (account, error) {
	path := stateDBPath()
	if _, err := os.Stat(path); err != nil {
		return account{}, fmt.Errorf("Cursor is not signed in on this machine")
	}

	dsn := "file:" + filepath.ToSlash(path) + "?mode=ro&_pragma=busy_timeout(4000)&_pragma=query_only(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return account{}, fmt.Errorf("open Cursor state db: %w", err)
	}
	defer db.Close()

	get := func(key string) string {
		var v string
		_ = db.QueryRow(`SELECT value FROM ItemTable WHERE key = ?`, key).Scan(&v)
		return strings.TrimSpace(v)
	}

	acc := account{
		AccessToken:  get("cursorAuth/accessToken"),
		RefreshToken: get("cursorAuth/refreshToken"),
		Plan:         get("cursorAuth/stripeMembershipType"),
	}
	if acc.AccessToken == "" && acc.RefreshToken == "" {
		return account{}, fmt.Errorf("no Cursor token — sign in to the Cursor app")
	}
	return acc, nil
}

func jwtExpired(token string, skew time.Duration) bool {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return true
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		padded := parts[1]
		if m := len(padded) % 4; m != 0 {
			padded += strings.Repeat("=", 4-m)
		}
		payload, err = base64.URLEncoding.DecodeString(padded)
		if err != nil {
			return true
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &claims) != nil || claims.Exp == 0 {
		return false
	}
	return time.Now().Add(skew).After(time.Unix(claims.Exp, 0))
}
