package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	DBPath                 string
	RaindropToken          string
	RefreshIntervalMinutes int
	DefaultTags            []string
}

func DefaultConfig() Config {
	return Config{
		DBPath:                 defaultDBPath(),
		RefreshIntervalMinutes: 30,
		DefaultTags:            []string{"rss"},
	}
}

func LoadConfig() (Config, error) {
	path := configPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			if err := SaveConfig(cfg); err != nil {
				return Config{}, err
			}
			return cfg, nil
		}
		return Config{}, err
	}

	cfg := DefaultConfig()
	if err := parseConfig(string(data), &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func SaveConfig(cfg Config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := renderConfig(cfg)
	return os.WriteFile(path, []byte(content), 0o600)
}

func configPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "config.toml"
	}
	return filepath.Join(configDir, "speedy-reader", "config.toml")
}

func defaultDBPath() string {
	dataDir := os.Getenv("XDG_DATA_HOME")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "feeds.db"
		}
		dataDir = filepath.Join(home, ".local", "share")
	}
	path := filepath.Join(dataDir, "speedy-reader")
	_ = os.MkdirAll(path, 0o755)
	return filepath.Join(path, "feeds.db")
}

func parseConfig(raw string, cfg *Config) error {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid config line: %q", line)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		switch key {
		case "db_path":
			cfg.DBPath = trimQuotes(value)
		case "raindrop_token":
			cfg.RaindropToken = trimQuotes(value)
		case "refresh_interval_minutes":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid refresh_interval_minutes: %w", err)
			}
			cfg.RefreshIntervalMinutes = parsed
		case "default_tags":
			items, err := parseStringArray(value)
			if err != nil {
				return err
			}
			cfg.DefaultTags = items
		default:
			// ignore unknown keys for forward compatibility
		}
	}
	return scanner.Err()
}

func trimQuotes(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	unquoted, err := strconv.Unquote(value)
	if err == nil {
		return unquoted
	}
	return strings.Trim(value, "\"")
}

func parseStringArray(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil, fmt.Errorf("invalid array value: %q", value)
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"))
	if inner == "" {
		return []string{}, nil
	}
	parts := strings.Split(inner, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		items = append(items, trimQuotes(item))
	}
	return items, nil
}

func renderConfig(cfg Config) string {
	lines := []string{
		"db_path = \"" + cfg.DBPath + "\"",
		"refresh_interval_minutes = " + strconv.Itoa(cfg.RefreshIntervalMinutes),
		"default_tags = " + renderStringArray(cfg.DefaultTags),
	}
	if cfg.RaindropToken != "" {
		lines = append(lines, "raindrop_token = \""+cfg.RaindropToken+"\"")
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderStringArray(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = strconv.Quote(item)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

