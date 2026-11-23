//go:build windows

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type guiState struct {
	Mode string `json:"mode"`
}

func resolveInitialMode(hasPAC bool, statePath string) proxyMode {
	if saved, ok := loadSavedProxyMode(statePath); ok {
		if saved == modePAC && !hasPAC {
			return modeDisabled
		}
		return saved
	}
	if hasPAC {
		return modePAC
	}
	return modeGlobal
}

func loadSavedProxyMode(path string) (proxyMode, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return modeDisabled, false
	}
	var st guiState
	if err := json.Unmarshal(b, &st); err != nil {
		return modeDisabled, false
	}
	return parseProxyMode(st.Mode)
}

func saveProxyMode(path string, mode proxyMode) error {
	data, err := json.MarshalIndent(guiState{Mode: formatProxyMode(mode)}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".s5tc-gui-state-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func parseProxyMode(s string) (proxyMode, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "global":
		return modeGlobal, true
	case "pac":
		return modePAC, true
	case "disabled":
		return modeDisabled, true
	default:
		return modeDisabled, false
	}
}

func formatProxyMode(mode proxyMode) string {
	switch mode {
	case modeGlobal:
		return "global"
	case modePAC:
		return "pac"
	default:
		return "disabled"
	}
}

