// Package config loads gitai configuration from ~/.gitai/gitai.json with
// environment variable overrides, and validates that the minimum required
// settings are present.
package config

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/spf13/viper"
)

// Load reads the config file and environment variables, validates the
// result, and returns the resolved viper instance — with api_base, api_key,
// and model set to their final values — plus the config directory path
// (used to locate system prompt overrides).
func Load() (*viper.Viper, string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, "", fmt.Errorf("could not get current user: %w", err)
	}

	configDir := filepath.Join(currentUser.HomeDir, ".gitai")
	configFile := filepath.Join(configDir, "gitai.json")

	v := viper.New()
	v.SetConfigName("gitai")
	v.SetConfigType("json")
	v.AddConfigPath(configDir)
	_ = v.ReadInConfig() // config file is optional

	// Environment variables override the config file.
	apiBase := v.GetString("api_base")
	if envBase := os.Getenv("GEMINI_API_BASE"); envBase != "" {
		apiBase = envBase
	}
	if apiBase == "" {
		apiBase = os.Getenv("API_BASE")
	}

	apiKey := v.GetString("api_key")
	if envKey := os.Getenv("GEMINI_API_KEY"); envKey != "" {
		apiKey = envKey
	}
	if apiKey == "" {
		apiKey = os.Getenv("API_KEY")
	}

	model := v.GetString("model")
	if envModel := os.Getenv("MODEL"); envModel != "" {
		model = envModel
	}

	if apiBase == "" {
		return nil, "", fmt.Errorf("no API base URL found. Set the API_BASE environment variable, or add api_base to %s", configFile)
	}

	// A top-level "model" is optional if per-task models are set.
	// At least one must exist.
	hasTaskModel := v.GetString("commit.model") != "" ||
		v.GetString("review.model") != "" ||
		v.GetString("pullreq.model") != ""
	if model == "" && !hasTaskModel {
		return nil, "", fmt.Errorf("no model found. Set the MODEL environment variable, add \"model\" to %s, or add a per-task \"model\" block (e.g. \"commit\")", configFile)
	}

	v.Set("api_base", apiBase)
	v.Set("api_key", apiKey)
	v.Set("model", model)

	return v, configDir, nil
}
