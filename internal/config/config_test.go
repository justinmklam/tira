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
