package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	JiraURL string
	Email   string
	Token   string
	Project string
	BoardID int
}

func Load() (*Config, error) {
	viper.AutomaticEnv()

	cfg := &Config{
		JiraURL: viper.GetString("JIRA_URL"),
		Email:   viper.GetString("JIRA_EMAIL"),
		Token:   viper.GetString("JIRA_API_TOKEN"),
	}

	if cfg.JiraURL == "" || cfg.Email == "" || cfg.Token == "" {
		return nil, fmt.Errorf("missing required env vars: JIRA_URL, JIRA_EMAIL, JIRA_API_TOKEN")
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME/.config/lazyjira")
	_ = viper.ReadInConfig()
	cfg.Project = viper.GetString("default_project")
	cfg.BoardID = viper.GetInt("default_board_id")

	return cfg, nil
}
