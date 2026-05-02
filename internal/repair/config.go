package repair

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gede-cahya/Smara-CLI/internal/config"
	"gopkg.in/yaml.v3"
)

// CheckConfigHealth validates the config file.
func CheckConfigHealth(cfgPath string) CheckResult {
	res := CheckResult{
		Module: "config",
		Status: StatusOK,
	}

	if cfgPath == "" {
		cfgPath = filepath.Join(config.SmaraDir(), "config.yaml")
	}

	info, err := os.Stat(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			res.Status = StatusFail
			res.Message = fmt.Sprintf("Config file tidak ditemukan: %s", cfgPath)
			res.Fixable = true
			res.Suggestion = "Tulis default config"
			return res
		}
		res.Status = StatusFail
		res.Message = fmt.Sprintf("Gagal membaca config: %v", err)
		res.Fixable = false
		return res
	}

	// Check permission (too open?)
	mode := info.Mode().Perm()
	if mode&0o044 != 0 {
		res.Status = StatusWarn
		res.Message = fmt.Sprintf("Config file readable by others (perm %04o): %s", mode, cfgPath)
		res.Fixable = true
		res.Suggestion = "chmod 600 config.yaml"
	}

	// Validate YAML
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		res.Status = StatusFail
		res.Message = fmt.Sprintf("Gagal membaca config: %v", err)
		res.Fixable = false
		return res
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		res.Status = StatusFail
		res.Message = fmt.Sprintf("Config invalid YAML: %v", err)
		res.Fixable = true
		res.Suggestion = "Backup & tulis default config"
		return res
	}

	// Check required keys
	if _, ok := raw["provider"]; !ok {
		res.Status = StatusWarn
		res.Message = "Key 'provider' tidak ditemukan di config"
		res.Fixable = true
		res.Suggestion = "Tulis default config"
		return res
	}

	if _, ok := raw["model"]; !ok {
		res.Status = StatusWarn
		res.Message = "Key 'model' tidak ditemukan di config"
		res.Fixable = true
		res.Suggestion = "Tulis default config"
		return res
	}

	if res.Status == StatusOK {
		res.Message = fmt.Sprintf("Config OK: %s (%04o)", cfgPath, mode)
	}
	return res
}

// RepairConfig backs up and writes a minimal default config.
func RepairConfig(cfgPath string) error {
	if cfgPath == "" {
		cfgPath = filepath.Join(config.SmaraDir(), "config.yaml")
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return fmt.Errorf("gagal buat config dir: %w", err)
	}

	// Backup if exists
	if _, err := os.Stat(cfgPath); err == nil {
		if _, err := BackupFile(cfgPath); err != nil {
			return fmt.Errorf("gagal backup config: %w", err)
		}
	}

	defaultCfg := config.DefaultConfig()
	defaultData, err := yaml.Marshal(defaultCfg)
	if err != nil {
		return fmt.Errorf("gagal marshal default config: %w", err)
	}

	if err := os.WriteFile(cfgPath, defaultData, 0o600); err != nil {
		return fmt.Errorf("gagal tulis default config: %w", err)
	}

	return nil
}

// FixConfigPermissions sets the config file to 600.
func FixConfigPermissions(cfgPath string) error {
	if cfgPath == "" {
		cfgPath = filepath.Join(config.SmaraDir(), "config.yaml")
	}
	return os.Chmod(cfgPath, 0o600)
}
