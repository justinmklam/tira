package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoad(t *testing.T) {
	// Setup a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	content := `
profiles:
  default:
    jira_url: https://default.atlassian.net
    email: default@example.com
    token: default-token
    project: PROJ1
    board_id: 1
  dev:
    jira_url: https://dev.atlassian.net
    email: dev@example.com
    token: dev-token
    project: PROJ2
    board_id: 2
`
	err := os.WriteFile(configPath, []byte(content), 0644)
	assert.NoError(t, err)

	t.Run("load default profile", func(t *testing.T) {
		cfg, err := Load("default", tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "https://default.atlassian.net", cfg.JiraURL)
		assert.Equal(t, "default@example.com", cfg.Email)
		assert.Equal(t, "default-token", cfg.Token)
		assert.Equal(t, "PROJ1", cfg.Project)
		assert.Equal(t, 1, cfg.BoardID)
	})

	t.Run("load dev profile", func(t *testing.T) {
		cfg, err := Load("dev", tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "https://dev.atlassian.net", cfg.JiraURL)
		assert.Equal(t, "dev@example.com", cfg.Email)
		assert.Equal(t, "dev-token", cfg.Token)
		assert.Equal(t, "PROJ2", cfg.Project)
		assert.Equal(t, 2, cfg.BoardID)
	})

	t.Run("load missing profile", func(t *testing.T) {
		cfg, err := Load("missing", tmpDir)
		assert.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "profile \"missing\" not found")
	})

	t.Run("load empty profile defaults to default", func(t *testing.T) {
		cfg, err := Load("", tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "https://default.atlassian.net", cfg.JiraURL)
	})
}

func TestLoad_EnvVarOverride(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Helper to write a config with a given token value
	writeConfig := func(tokenVal string) {
		content := "profiles:\n  default:\n    jira_url: https://from-config.atlassian.net\n    email: config@example.com\n    token: " + tokenVal + "\n    project: CFGPROJ\n    board_id: 99\n    classic_project: false\n    theme: default\n"
		err := os.WriteFile(configPath, []byte(content), 0644)
		assert.NoError(t, err)
	}

	// Helper to ensure env vars are clean before each sub-test
	resetEnvVars := func() func() {
		type envState struct {
			name string
			old  string
			had  bool
		}
		var states []envState
		for _, env := range []string{"TIRA_JIRA_URL", "TIRA_EMAIL", "TIRA_TOKEN", "TIRA_PROJECT", "TIRA_BOARD_ID", "TIRA_CLASSIC_PROJECT", "TIRA_THEME", "JIRA_TOKEN", "JIRA_API_TOKEN"} {
			old, had := os.LookupEnv(env)
			states = append(states, envState{env, old, had})
			os.Unsetenv(env) //nolint:errcheck
		}
		return func() {
			for _, s := range states {
				if s.had {
					os.Setenv(s.name, s.old) //nolint:errcheck
				} else {
					os.Unsetenv(s.name) //nolint:errcheck
				}
			}
		}
	}

	t.Run("config values used when no env vars set", func(t *testing.T) {
		cleanup := resetEnvVars()
		defer cleanup()
		writeConfig("cfg-token")

		cfg, err := Load("default", tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "https://from-config.atlassian.net", cfg.JiraURL)
		assert.Equal(t, "config@example.com", cfg.Email)
		assert.Equal(t, "cfg-token", cfg.Token)
		assert.Equal(t, "CFGPROJ", cfg.Project)
		assert.Equal(t, 99, cfg.BoardID)
	})

	t.Run("TIRA_* env vars override config values", func(t *testing.T) {
		cleanup := resetEnvVars()
		defer cleanup()
		writeConfig("cfg-token")
		t.Setenv("TIRA_JIRA_URL", "https://env.atlassian.net")
		t.Setenv("TIRA_EMAIL", "env@example.com")
		t.Setenv("TIRA_TOKEN", "env-token")
		t.Setenv("TIRA_PROJECT", "ENVPROJ")
		t.Setenv("TIRA_BOARD_ID", "42")

		cfg, err := Load("default", tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "https://env.atlassian.net", cfg.JiraURL)
		assert.Equal(t, "env@example.com", cfg.Email)
		assert.Equal(t, "env-token", cfg.Token)
		assert.Equal(t, "ENVPROJ", cfg.Project)
		assert.Equal(t, 42, cfg.BoardID)
	})

	t.Run("TIRA_TOKEN takes precedence over config file", func(t *testing.T) {
		cleanup := resetEnvVars()
		defer cleanup()
		writeConfig("cfg-token")
		t.Setenv("TIRA_TOKEN", "tira-token")

		cfg, err := Load("default", tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "tira-token", cfg.Token)
	})

	t.Run("JIRA_TOKEN used when config token is empty and no TIRA_TOKEN", func(t *testing.T) {
		cleanup := resetEnvVars()
		defer cleanup()
		writeConfig("")
		t.Setenv("JIRA_TOKEN", "jira-token")

		cfg, err := Load("default", tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "jira-token", cfg.Token)
	})

	t.Run("JIRA_API_TOKEN used as last resort", func(t *testing.T) {
		cleanup := resetEnvVars()
		defer cleanup()
		writeConfig("")
		t.Setenv("JIRA_API_TOKEN", "api-token")

		cfg, err := Load("default", tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "api-token", cfg.Token)
	})

	t.Run("TIRA_TOKEN takes precedence over JIRA_TOKEN and JIRA_API_TOKEN", func(t *testing.T) {
		cleanup := resetEnvVars()
		defer cleanup()
		writeConfig("")
		t.Setenv("TIRA_TOKEN", "tira-priority")
		t.Setenv("JIRA_TOKEN", "jira-fallback")
		t.Setenv("JIRA_API_TOKEN", "api-fallback")

		cfg, err := Load("default", tmpDir)
		assert.NoError(t, err)
		assert.Equal(t, "tira-priority", cfg.Token)
	})

	t.Run("error when token is empty everywhere", func(t *testing.T) {
		cleanup := resetEnvVars()
		defer cleanup()
		writeConfig("")

		cfg, err := Load("default", tmpDir)
		assert.Error(t, err)
		assert.Nil(t, cfg)
		assert.Contains(t, err.Error(), "missing required fields")
	})
}
