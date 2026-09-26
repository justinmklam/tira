package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	JiraURL        string `mapstructure:"jira_url"`
	Email          string `mapstructure:"email"`
	Token          string `mapstructure:"token"`
	Project        string `mapstructure:"project"`
	BoardID        int    `mapstructure:"board_id"`
	ClassicProject bool   `mapstructure:"classic_project"`
	Theme          string `mapstructure:"theme"`
}

func Load(profileName string, searchPaths ...string) (*Config, error) {
	return load(profileName, true, searchPaths...)
}

// LoadDev behaves like Load but tolerates a missing config file and does not
// require jira_url, email, or token. It still applies TIRA_* environment
// overrides and still fails on a malformed config file.
func LoadDev(profileName string, searchPaths ...string) (*Config, error) {
	return load(profileName, false, searchPaths...)
}

// isConfigNotFound reports whether viper failed because no config file exists in
// any of the search paths (rather than because a file could not be parsed).
func isConfigNotFound(err error) bool {
	var notFound viper.ConfigFileNotFoundError
	return errors.As(err, &notFound)
}

func load(profileName string, requireCredentials bool, searchPaths ...string) (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")

	if len(searchPaths) > 0 {
		for _, path := range searchPaths {
			v.AddConfigPath(path)
		}
	} else {
		home, err := os.UserHomeDir()
		if err == nil {
			v.AddConfigPath(filepath.Join(home, ".config", "tira"))
		}
		v.AddConfigPath(".") // Also look in current directory for convenience
	}

	cfg := &Config{}

	if err := v.ReadInConfig(); err != nil {
		if requireCredentials || !isConfigNotFound(err) {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		applyEnv(cfg)
		return cfg, nil
	}

	profiles := v.GetStringMap("profiles")
	if len(profiles) == 0 {
		if requireCredentials {
			return nil, fmt.Errorf("no profiles found in config file")
		}
		applyEnv(cfg)
		return cfg, nil
	}

	if profileName == "" {
		profileName = "default"
	}

	profileKey := fmt.Sprintf("profiles.%s", profileName)
	if !v.IsSet(profileKey) {
		if requireCredentials {
			return nil, fmt.Errorf("profile %q not found in config", profileName)
		}
		applyEnv(cfg)
		return cfg, nil
	}

	var loaded Config
	if err := v.UnmarshalKey(profileKey, &loaded); err != nil {
		return nil, fmt.Errorf("failed to unmarshal profile %q: %w", profileName, err)
	}
	*cfg = loaded

	applyEnv(cfg)

	if requireCredentials && (cfg.JiraURL == "" || cfg.Email == "" || cfg.Token == "") {
		return nil, fmt.Errorf("profile %q is missing required fields: jira_url, email, token", profileName)
	}

	return cfg, nil
}

// applyEnv overrides cfg with TIRA_* environment variables (which take
// precedence over the config file) and applies the token fallbacks.
func applyEnv(cfg *Config) {
	if v := os.Getenv("TIRA_JIRA_URL"); v != "" {
		cfg.JiraURL = v
	}
	if v := os.Getenv("TIRA_EMAIL"); v != "" {
		cfg.Email = v
	}
	if v := os.Getenv("TIRA_TOKEN"); v != "" {
		cfg.Token = v
	}
	if v := os.Getenv("TIRA_PROJECT"); v != "" {
		cfg.Project = v
	}
	if v := os.Getenv("TIRA_BOARD_ID"); v != "" {
		if id, err := strconv.Atoi(v); err == nil {
			cfg.BoardID = id
		}
	}
	if v := os.Getenv("TIRA_CLASSIC_PROJECT"); v != "" {
		cfg.ClassicProject = strings.EqualFold(v, "true")
	}
	if v := os.Getenv("TIRA_THEME"); v != "" {
		cfg.Theme = v
	}

	// Fallback for token: JIRA_TOKEN or JIRA_API_TOKEN
	if cfg.Token == "" {
		if v := os.Getenv("JIRA_TOKEN"); v != "" {
			cfg.Token = v
		} else if v := os.Getenv("JIRA_API_TOKEN"); v != "" {
			cfg.Token = v
		}
	}
}
