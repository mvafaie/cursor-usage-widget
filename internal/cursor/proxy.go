package cursor

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var (
	httpProxySettingRE = regexp.MustCompile(`"http\.proxy"\s*:\s*"([^"]*)"`)
	errInvalidProxy    = errors.New("invalid http.proxy URL")
)

func cursorSettingsPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "Cursor", "User", "settings.json")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Cursor", "User", "settings.json")
	default:
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "Cursor", "User", "settings.json")
		}
		return filepath.Join(home, ".config", "Cursor", "User", "settings.json")
	}
}

// readCursorHTTPProxy returns Cursor settings.json http.proxy, or "" if unset/unreadable.
func readCursorHTTPProxy() string {
	b, err := os.ReadFile(cursorSettingsPath())
	if err != nil {
		return ""
	}
	m := httpProxySettingRE.FindSubmatch(b)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(string(m[1]))
}

func httpClientWithProxy(raw string) (*http.Client, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, errInvalidProxy
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(u)
	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
	}, nil
}
