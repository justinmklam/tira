package config

import (
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

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	profiles := v.GetStringMap("profiles")
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no profiles found in config file")
	}

	if profileName == "" {
		profileName = "default"
	}

	profileKey := fmt.Sprintf("profiles.%s", profileName)
	if !v.IsSet(profileKey) {
		return nil, fmt.Errorf("profile %q not found in config", profileName)
	}

	var cfg Config
	if err := v.UnmarshalKey(profileKey, &cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal profile %q: %w", profileName, err)
	}

	// Override with TIRA_* env vars if set (take precedence over config file)
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

	if cfg.JiraURL == "" || cfg.Email == "" || cfg.Token == "" {
		return nil, fmt.Errorf("profile %q is missing required fields: jira_url, email, token", profileName)
	}

	return &cfg, nil
}
