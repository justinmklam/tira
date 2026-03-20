package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	JiraURL        string `mapstructure:"jira_url"`
	Email          string `mapstructure:"email"`
	Token          string `mapstructure:"token"`
	Project        string `mapstructure:"project"`
	BoardID        int    `mapstructure:"board_id"`
	ClassicProject bool   `mapstructure:"classic_project"`
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

	if cfg.JiraURL == "" || cfg.Email == "" || cfg.Token == "" {
		return nil, fmt.Errorf("profile %q is missing required fields: jira_url, email, token", profileName)
	}

	return &cfg, nil
}
