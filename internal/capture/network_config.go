package capture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	configFilename   = "network.json"
	legacyIPFilename = "ip.txt"
)

type PersistedInterface struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type LoggingConfig struct {
	ServerLogsEnabled bool `json:"serverLogsEnabled"`
	PcapRecording     bool `json:"pcapRecording"`
}

// HubConfig points this radar client at a self-hosted OpenRadar Hub instance for
// sharing discovered Avalonian Road edges with a group. See internal/hub.
type HubConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
	Secret  string `json:"secret"`
}

// MarketConfig selects which of the Albion Online Data Project's regional API hosts to use
// as the market-price fallback source when no Hub is configured/reachable (the Hub, if used,
// picks its own region independently via its AODP_REGION env var). See internal/adp.
type MarketConfig struct {
	Region string `json:"region"` // "europe" (default), "americas", or "asia"
}

// UpdateCheckConfig caches the result of the launch-time GitHub release check (see
// internal/updatecheck) so the web UI's GET /api/settings/update can report "is an update
// available" without making its own outbound call, and so a dismissed version stays dismissed
// across restarts. LatestVersion/ReleaseURL/LastChecked are facts about what GitHub reported as
// of the last check - "is there actually an update" is always recomputed against the running
// binary's own Version at read time, never baked into what's persisted here.
type UpdateCheckConfig struct {
	LatestVersion    string    `json:"latestVersion"`
	ReleaseURL       string    `json:"releaseUrl"`
	LastChecked      time.Time `json:"lastChecked"`
	DismissedVersion string    `json:"dismissedVersion"`
}

type Config struct {
	CaptureInterfaces []PersistedInterface `json:"captureInterfaces"`
	Logging           LoggingConfig        `json:"logging"`
	Hub               HubConfig            `json:"hub"`
	Market            MarketConfig         `json:"market"`
	UpdateCheck       UpdateCheckConfig    `json:"updateCheck"`
}

func ReadConfig(appDir string) (Config, error) {
	path := filepath.Join(appDir, configFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

func WriteConfig(appDir string, cfg Config) error {
	path := filepath.Join(appDir, configFilename)
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename tmp: %w", err)
	}
	return nil
}

// MutateConfig reads the config, applies the mutator, and writes atomically.
func MutateConfig(appDir string, mutate func(*Config)) error {
	cfg, err := ReadConfig(appDir)
	if err != nil {
		return err
	}
	mutate(&cfg)
	return WriteConfig(appDir, cfg)
}

type IPResolver func(ip string) (PersistedInterface, error)

func MigrateIPTxt(appDir string, resolve IPResolver) (bool, error) {
	ipPath := filepath.Join(appDir, legacyIPFilename)
	data, err := os.ReadFile(ipPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %s: %w", ipPath, err)
	}
	ip := strings.TrimSpace(string(data))
	if ip == "" {
		_ = os.Remove(ipPath)
		return false, nil
	}
	existing, err := ReadConfig(appDir)
	if err != nil {
		return false, fmt.Errorf("read existing config before migration: %w", err)
	}
	if len(existing.CaptureInterfaces) > 0 {
		_ = os.Remove(ipPath)
		return false, nil
	}
	if resolve == nil {
		return false, fmt.Errorf("no IP resolver provided for migration")
	}
	entry, err := resolve(ip)
	if err != nil {
		_ = os.Remove(ipPath)
		return false, fmt.Errorf("resolve legacy ip %q: %w", ip, err)
	}
	cfg := Config{CaptureInterfaces: []PersistedInterface{entry}}
	if err := WriteConfig(appDir, cfg); err != nil {
		return false, err
	}
	_ = os.Remove(ipPath)
	return true, nil
}
