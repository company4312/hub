package store

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadAgentConfigs reads all .yaml files from dir and returns agent configs.
func LoadAgentConfigs(dir string) ([]AgentConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read agents dir: %w", err)
	}

	var configs []AgentConfig
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		cfg, err := LoadAgentConfig(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", e.Name(), err)
		}
		configs = append(configs, *cfg)
	}
	return configs, nil
}

// LoadAgentConfig reads a single agent config from a YAML file.
func LoadAgentConfig(path string) (*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg AgentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if cfg.Name == "" {
		return nil, fmt.Errorf("agent config missing 'name' field")
	}
	return &cfg, nil
}
