package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearAXMEnv unsets all AXM_* environment variables and returns a restore function.
func clearAXMEnv(t *testing.T) func() {
	t.Helper()
	saved := map[string]string{}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "AXM_") {
			k := strings.SplitN(kv, "=", 2)[0]
			saved[k] = os.Getenv(k)
			os.Unsetenv(k)
		}
	}
	return func() {
		for k, v := range saved {
			os.Setenv(k, v)
		}
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	restore := clearAXMEnv(t)
	defer restore()
	content := `
abm:
  client_id: "BUSINESSAPI.test-id"
  key_id: "test-key-id"
  private_key: "./test.pem"
snipe_it:
  url: "https://snipe.example.com"
  api_key: "test-api-key"
  manufacturer_id: 1
  default_status_id: 2
  computer_category_id: 3
  mobile_category_id: 4
  custom_fieldset_id: 5
sync:
  dry_run: true
  force: true
  rate_limit: true
  update_only: true
  product_families:
    - Mac
    - iPhone
  set_name: true
  field_mapping:
    _snipeit_color_1: color
    purchase_date: order_date
  supplier_mapping:
    APPLE: 1
    "1C71B60": 3
slack:
  enabled: true
  webhook_url: "https://hooks.slack.com/test"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// ABM
	if cfg.ABM.ClientID != "BUSINESSAPI.test-id" {
		t.Errorf("ABM.ClientID = %q", cfg.ABM.ClientID)
	}
	if cfg.ABM.KeyID != "test-key-id" {
		t.Errorf("ABM.KeyID = %q", cfg.ABM.KeyID)
	}

	// Snipe-IT
	if cfg.SnipeIT.URL != "https://snipe.example.com" {
		t.Errorf("SnipeIT.URL = %q", cfg.SnipeIT.URL)
	}
	if cfg.SnipeIT.ManufacturerID != 1 {
		t.Errorf("ManufacturerID = %d", cfg.SnipeIT.ManufacturerID)
	}
	if cfg.SnipeIT.CustomFieldsetID != 5 {
		t.Errorf("CustomFieldsetID = %d", cfg.SnipeIT.CustomFieldsetID)
	}

	// Sync
	if !cfg.Sync.DryRun {
		t.Error("DryRun should be true")
	}
	if !cfg.Sync.Force {
		t.Error("Force should be true")
	}
	if !cfg.Sync.UpdateOnly {
		t.Error("UpdateOnly should be true")
	}
	if len(cfg.Sync.ProductFamilies) != 2 {
		t.Errorf("ProductFamilies len = %d", len(cfg.Sync.ProductFamilies))
	}
	if cfg.Sync.FieldMapping["_snipeit_color_1"] != "color" {
		t.Errorf("FieldMapping color = %q", cfg.Sync.FieldMapping["_snipeit_color_1"])
	}
	if cfg.Sync.SupplierMapping["APPLE"] != 1 {
		t.Errorf("SupplierMapping APPLE = %d", cfg.Sync.SupplierMapping["APPLE"])
	}

	// Slack
	if !cfg.Slack.Enabled {
		t.Error("Slack.Enabled should be true")
	}
	if cfg.Slack.WebhookURL != "https://hooks.slack.com/test" {
		t.Errorf("Slack.WebhookURL = %q", cfg.Slack.WebhookURL)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	content := `
abm:
  client_id: "original"
  key_id: "original"
  private_key: "./test.pem"
snipe_it:
  url: "https://original.example.com"
  api_key: "original-key"
  manufacturer_id: 1
  default_status_id: 1
  category_id: 1
`
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AXM_ABM_CLIENT_ID", "env-client-id")
	t.Setenv("AXM_SNIPE_URL", "https://env.example.com")

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.ABM.ClientID != "env-client-id" {
		t.Errorf("env override ABM.ClientID = %q, want env-client-id", cfg.ABM.ClientID)
	}
	if cfg.SnipeIT.URL != "https://env.example.com" {
		t.Errorf("env override SnipeIT.URL = %q", cfg.SnipeIT.URL)
	}
	// Non-overridden should keep original
	if cfg.ABM.KeyID != "original" {
		t.Errorf("ABM.KeyID should be original, got %q", cfg.ABM.KeyID)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/settings.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")
	if err := os.WriteFile(path, []byte(":::invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// --- Validation tests ---

func TestValidate_FullConfig(t *testing.T) {
	cfg := &Config{
		ABM: ABMConfig{ClientID: "test", KeyID: "test", PrivateKey: "./test.pem"},
		SnipeIT: SnipeITConfig{
			URL: "https://test.example.com", APIKey: "test",
			ManufacturerID: 1, DefaultStatusID: 1, CategoryID: 1,
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config should pass: %v", err)
	}
}

func TestValidateABM_MissingClientID(t *testing.T) {
	cfg := &Config{ABM: ABMConfig{KeyID: "test", PrivateKey: "./test.pem"}}
	if err := cfg.ValidateABM(); err == nil {
		t.Error("expected error for missing client_id")
	}
}

func TestValidateABM_MissingKeyID(t *testing.T) {
	cfg := &Config{ABM: ABMConfig{ClientID: "test", PrivateKey: "./test.pem"}}
	if err := cfg.ValidateABM(); err == nil {
		t.Error("expected error for missing key_id")
	}
}

func TestValidateABM_MissingPrivateKey(t *testing.T) {
	cfg := &Config{ABM: ABMConfig{ClientID: "test", KeyID: "test"}}
	if err := cfg.ValidateABM(); err == nil {
		t.Error("expected error for missing private_key")
	}
}

func TestValidateSnipeIT_MissingURL(t *testing.T) {
	cfg := &Config{SnipeIT: SnipeITConfig{APIKey: "test", ManufacturerID: 1, DefaultStatusID: 1, CategoryID: 1}}
	if err := cfg.ValidateSnipeIT(); err == nil {
		t.Error("expected error for missing url")
	}
}

func TestValidateSnipeIT_MissingCategory(t *testing.T) {
	cfg := &Config{SnipeIT: SnipeITConfig{URL: "test", APIKey: "test", ManufacturerID: 1, DefaultStatusID: 1}}
	if err := cfg.ValidateSnipeIT(); err == nil {
		t.Error("expected error for missing category")
	}
}

func TestValidateSnipeIT_ComputerCategoryOnly(t *testing.T) {
	cfg := &Config{SnipeIT: SnipeITConfig{
		URL: "test", APIKey: "test", ManufacturerID: 1, DefaultStatusID: 1,
		ComputerCategoryID: 5,
	}}
	if err := cfg.ValidateSnipeIT(); err != nil {
		t.Errorf("computer_category_id alone should be valid: %v", err)
	}
}

// --- CategoryIDForFamily tests ---

func TestCategoryIDForFamily(t *testing.T) {
	s := &SnipeITConfig{
		CategoryID:         10,
		ComputerCategoryID: 20,
		MobileCategoryID:   30,
	}

	tests := []struct {
		family string
		want   int
	}{
		{"Mac", 20},
		{"iPhone", 30},
		{"iPad", 30},
		{"Watch", 30},
		{"Vision", 30},
		{"AppleTV", 10}, // falls back to CategoryID
		{"Unknown", 10},
	}

	for _, tt := range tests {
		got := s.CategoryIDForFamily(tt.family)
		if got != tt.want {
			t.Errorf("CategoryIDForFamily(%q) = %d, want %d", tt.family, got, tt.want)
		}
	}
}

func TestCategoryIDForFamily_Fallbacks(t *testing.T) {
	// Only ComputerCategoryID set
	s := &SnipeITConfig{ComputerCategoryID: 20}
	if got := s.CategoryIDForFamily("iPhone"); got != 20 {
		t.Errorf("fallback to ComputerCategoryID: got %d, want 20", got)
	}

	// Only MobileCategoryID set
	s2 := &SnipeITConfig{MobileCategoryID: 30}
	if got := s2.CategoryIDForFamily("Mac"); got != 30 {
		t.Errorf("fallback to MobileCategoryID: got %d, want 30", got)
	}
}

// --- MergeFieldMapping tests ---

func TestMergeFieldMapping(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")

	initial := `sync:
  field_mapping:
    _snipeit_color_1: color
`
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	newMappings := map[string]string{
		"_snipeit_color_1":  "color",    // existing — should not duplicate
		"_snipeit_status_2": "applecare_status",
	}

	if err := MergeFieldMapping(path, newMappings); err != nil {
		t.Fatal(err)
	}

	// Re-read and verify
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sync.FieldMapping["_snipeit_color_1"] != "color" {
		t.Errorf("existing mapping should be preserved")
	}
	if cfg.Sync.FieldMapping["_snipeit_status_2"] != "applecare_status" {
		t.Errorf("new mapping should be added")
	}
}

func TestMergeFieldMapping_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")

	if err := os.WriteFile(path, []byte("sync:\n  dry_run: true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	newMappings := map[string]string{
		"_snipeit_mac_1": "wifi_mac",
	}

	if err := MergeFieldMapping(path, newMappings); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sync.FieldMapping["_snipeit_mac_1"] != "wifi_mac" {
		t.Errorf("new mapping should be added to empty field_mapping")
	}
}

func TestMergeFieldMapping_SkipsEmptyKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")

	if err := os.WriteFile(path, []byte("sync:\n  field_mapping:\n    existing: value\n"), 0644); err != nil {
		t.Fatal(err)
	}

	newMappings := map[string]string{
		"":          "wifi_mac", // empty key — skip
		"valid_key": "",         // empty value — skip
	}

	if err := MergeFieldMapping(path, newMappings); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sync.FieldMapping) != 1 {
		t.Errorf("should only have 1 mapping, got %d", len(cfg.Sync.FieldMapping))
	}
}
