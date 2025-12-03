package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveDevContainerJSON(t *testing.T) {
	type args struct {
		config *DevContainerConfig
	}
	tests := []struct {
		name     string
		args     args
		wantErr  bool
		wantJSON string
	}{
		{
			name: "test omit build field in devcontainer.json",
			args: args{
				config: &DevContainerConfig{
					ImageContainer: ImageContainer{
						Image: "test",
					},
				},
			},
			wantErr:  false,
			wantJSON: `{"image":"test"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir, err := os.MkdirTemp(os.TempDir(), "test-devcontainer")
			if err != nil {
				t.Fatalf("Failed to create temp dir: %v", err)
			}
			defer func() { _ = os.RemoveAll(tmpDir) }()

			tt.args.config.Origin = filepath.Join(tmpDir, "devcontainer.json")

			if err := SaveDevContainerJSON(tt.args.config); (err != nil) != tt.wantErr {
				t.Errorf("SaveDevContainerJSON() error = %v, wantErr %v", err, tt.wantErr)
			}

			contents, err := os.ReadFile(tt.args.config.Origin)
			if err != nil {
				t.Fatalf("Failed to read file contents: %v", err)
			}
			if string(contents) != tt.wantJSON {
				t.Errorf("Expected JSON = %v, got %v", tt.wantJSON, string(contents))
			}
		})
	}
}

func TestFindDevContainerConfigs(t *testing.T) {
	tmpDir := t.TempDir()

	configs := []string{
		".devcontainer/python/devcontainer.json",
		".devcontainer/node/devcontainer.json",
	}

	for _, cfg := range configs {
		fullPath := filepath.Join(tmpDir, cfg)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(`{"name":"test"}`), 0644); err != nil {
			t.Fatal(err)
		}
	}

	found, err := findDevContainerConfigs(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != 2 {
		t.Errorf("expected 2 configs, got %d", len(found))
	}
}

func TestListDevContainerIDs(t *testing.T) {
	tmpDir := t.TempDir()

	configs := map[string]string{
		".devcontainer/python/devcontainer.json": `{"name":"Python"}`,
		".devcontainer/node/devcontainer.json":   `{"name":"Node"}`,
	}

	for cfg, content := range configs {
		fullPath := filepath.Join(tmpDir, cfg)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	ids, err := ListDevContainerIDs(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(ids) != 2 {
		t.Errorf("expected 2 IDs, got %d: %v", len(ids), ids)
	}

	hasNode, hasPython := false, false
	for _, id := range ids {
		if id == "node" {
			hasNode = true
		}
		if id == "python" {
			hasPython = true
		}
	}

	if !hasNode || !hasPython {
		t.Errorf("expected 'node' and 'python' IDs, got: %v", ids)
	}
}

func TestParseDevContainerJSONWithSelector(t *testing.T) {
	t.Run("explicit path", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, "custom.json")
		if err := os.WriteFile(configPath, []byte(`{"name":"Custom"}`), 0644); err != nil {
			t.Fatal(err)
		}

		config, err := ParseDevContainerJSONWithSelector(tmpDir, "custom.json", nil)
		if err != nil {
			t.Fatal(err)
		}
		if config.Name != "Custom" {
			t.Errorf("expected Custom, got %s", config.Name)
		}
	})

	t.Run("explicit path not found", func(t *testing.T) {
		tmpDir := t.TempDir()
		_, err := ParseDevContainerJSONWithSelector(tmpDir, "missing.json", nil)
		if err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run(".devcontainer/devcontainer.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, ".devcontainer", "devcontainer.json")
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configPath, []byte(`{"name":"Standard"}`), 0644); err != nil {
			t.Fatal(err)
		}

		config, err := ParseDevContainerJSONWithSelector(tmpDir, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if config.Name != "Standard" {
			t.Errorf("expected Standard, got %s", config.Name)
		}
	})

	t.Run(".devcontainer.json", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, ".devcontainer.json")
		if err := os.WriteFile(configPath, []byte(`{"name":"Root"}`), 0644); err != nil {
			t.Fatal(err)
		}

		config, err := ParseDevContainerJSONWithSelector(tmpDir, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if config.Name != "Root" {
			t.Errorf("expected Root, got %s", config.Name)
		}
	})

	t.Run("single subfolder", func(t *testing.T) {
		tmpDir := t.TempDir()
		configPath := filepath.Join(tmpDir, ".devcontainer/python/devcontainer.json")
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(configPath, []byte(`{"name":"Python"}`), 0644); err != nil {
			t.Fatal(err)
		}

		config, err := ParseDevContainerJSONWithSelector(tmpDir, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if config.Name != "Python" {
			t.Errorf("expected Python, got %s", config.Name)
		}
	})

	t.Run("multiple subfolders with selector", func(t *testing.T) {
		tmpDir := t.TempDir()
		pythonPath := filepath.Join(tmpDir, ".devcontainer/python/devcontainer.json")
		nodePath := filepath.Join(tmpDir, ".devcontainer/node/devcontainer.json")

		if err := os.MkdirAll(filepath.Dir(pythonPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(nodePath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pythonPath, []byte(`{"name":"Python"}`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(nodePath, []byte(`{"name":"Node"}`), 0644); err != nil {
			t.Fatal(err)
		}

		config, err := ParseDevContainerJSONWithSelector(tmpDir, "", func(matches []string) (string, error) {
			for _, match := range matches {
				if filepath.Base(filepath.Dir(match)) == "python" {
					return match, nil
				}
			}
			return "", errors.New("not found")
		})
		if err != nil {
			t.Fatal(err)
		}
		if config.Name != "Python" {
			t.Errorf("expected Python, got %s", config.Name)
		}
	})

	t.Run("multiple subfolders without selector", func(t *testing.T) {
		tmpDir := t.TempDir()
		pythonPath := filepath.Join(tmpDir, ".devcontainer/python/devcontainer.json")
		nodePath := filepath.Join(tmpDir, ".devcontainer/node/devcontainer.json")

		if err := os.MkdirAll(filepath.Dir(pythonPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(nodePath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pythonPath, []byte(`{"name":"Python"}`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(nodePath, []byte(`{"name":"Node"}`), 0644); err != nil {
			t.Fatal(err)
		}

		config, err := ParseDevContainerJSONWithSelector(tmpDir, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if config == nil {
			t.Error("expected config, got nil")
		}
	})

	t.Run("selector error", func(t *testing.T) {
		tmpDir := t.TempDir()
		pythonPath := filepath.Join(tmpDir, ".devcontainer/python/devcontainer.json")
		nodePath := filepath.Join(tmpDir, ".devcontainer/node/devcontainer.json")
		if err := os.MkdirAll(filepath.Dir(pythonPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(nodePath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pythonPath, []byte(`{"name":"Python"}`), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(nodePath, []byte(`{"name":"Node"}`), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := ParseDevContainerJSONWithSelector(tmpDir, "", func(matches []string) (string, error) {
			return "", errors.New("selector failed")
		})
		if err == nil || err.Error() != "selector failed" {
			t.Errorf("expected selector error, got %v", err)
		}
	})

	t.Run("no config found", func(t *testing.T) {
		tmpDir := t.TempDir()
		config, err := ParseDevContainerJSONWithSelector(tmpDir, "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if config != nil {
			t.Error("expected nil config")
		}
	})
}
