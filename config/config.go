package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for axm2snipe.
type Config struct {
	ABM     ABMConfig     `yaml:"abm"`
	SnipeIT SnipeITConfig `yaml:"snipe_it"`
	Sync    SyncConfig    `yaml:"sync"`
	Slack   SlackConfig   `yaml:"slack"`
	Log     LogConfig     `yaml:"log"`
}

// LogConfig holds logging settings that can be set in the config file.
// CLI flags (--log-file, --log-format, -v, -d) take precedence.
type LogConfig struct {
	File   string `yaml:"file"`   // path to log file (appended to stderr)
	Format string `yaml:"format"` // "text" (default) or "json"
	Level  string `yaml:"level"`  // "warn" (default), "info", or "debug"
}

// SlackConfig holds optional Slack webhook notification settings.
type SlackConfig struct {
	WebhookURL string `yaml:"webhook_url"` // Slack incoming webhook URL
	Enabled    bool   `yaml:"enabled"`     // whether to send notifications
}

// ABMConfig holds Apple Business/School Manager settings.
type ABMConfig struct {
	ClientID   string `yaml:"client_id"`
	KeyID      string `yaml:"key_id"`
	PrivateKey string `yaml:"private_key"`  // path to PEM file or raw PEM string
	OAuthScope string `yaml:"oauth_scope"`  // optional explicit override; auto-detected from client_id when empty
	APIBaseURL string `yaml:"api_base_url"` // optional explicit override; auto-detected from client_id when empty
}

// SnipeITConfig holds Snipe-IT API settings.
type SnipeITConfig struct {
	URL                      string `yaml:"url"`
	APIKey                   string `yaml:"api_key"`
	ManufacturerID           int    `yaml:"manufacturer_id"`              // Apple manufacturer ID in Snipe
	DefaultStatusID          int    `yaml:"default_status_id"`            // Status for newly created assets
	CategoryID               int    `yaml:"category_id"`                  // Default category for new models (fallback)
	ComputerCategoryID       int    `yaml:"computer_category_id"`         // Category for computer models (Mac)
	MobileCategoryID         int    `yaml:"mobile_category_id"`           // Category for mobile models (iPhone, iPad)
	CustomFieldsetID         int    `yaml:"custom_fieldset_id"`           // Optional fieldset for new models
}

// SyncConfig holds sync behavior settings.
type SyncConfig struct {
	DryRun           bool              `yaml:"dry_run"`
	Force            bool              `yaml:"force"`             // ignore timestamps, always update
	RateLimit        bool              `yaml:"rate_limit"`        // enable rate limiting
	UpdateOnly       bool              `yaml:"update_only"`       // only update existing assets, never create new ones
	UseCache         bool              `yaml:"use_cache"`         // use cached data instead of fetching from ABM API
	CacheDir         string            `yaml:"cache_dir"`         // directory for cached API responses (default ".cache")
	ProductFamilies  []string          `yaml:"product_families"`  // filter by product family (Mac, iPhone, iPad, etc.)
	SetName          bool              `yaml:"set_name"`          // set asset name on create (default false)
	FieldMapping     map[string]string `yaml:"field_mapping"`     // snipe field -> ABM attribute mapping
	MDMOnly          bool              `yaml:"mdm_only"`          // only sync devices assigned to an MDM server
	MDMOnlyCache     bool              `yaml:"mdm_only_cache"`    // also exclude non-MDM devices from cache (requires mdm_only)
	SupplierMapping  map[string]int    `yaml:"supplier_mapping"`  // ABM purchaseSourceId or purchaseSourceType -> snipe supplier ID
	ModelImages        bool              `yaml:"model_images"`         // fetch device images from appledb.dev for newly created models
}

// Load reads configuration from a YAML file and applies environment variable overrides.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Environment variable overrides
	if v := os.Getenv("AXM_ABM_CLIENT_ID"); v != "" {
		cfg.ABM.ClientID = v
	}
	if v := os.Getenv("AXM_ABM_KEY_ID"); v != "" {
		cfg.ABM.KeyID = v
	}
	if v := os.Getenv("AXM_ABM_PRIVATE_KEY"); v != "" {
		cfg.ABM.PrivateKey = v
	}
	if v := os.Getenv("AXM_SNIPE_URL"); v != "" {
		cfg.SnipeIT.URL = v
	}
	if v := os.Getenv("AXM_SNIPE_API_KEY"); v != "" {
		cfg.SnipeIT.APIKey = v
	}

	return cfg, nil
}

// Validate checks that all required fields are set for a full sync.
// ABM credentials are not required when UseCache is true.
func (c *Config) Validate() error {
	if !c.Sync.UseCache {
		if err := c.ValidateABM(); err != nil {
			return err
		}
	}
	if err := c.ValidateSnipeIT(); err != nil {
		return err
	}
	return nil
}

// PrivateKeyValue returns the private key in a form suitable for abm.NewAssertion:
//   - File path (value passes os.Stat): returned as-is; upstream reads the file.
//   - Inline PEM (starts with "-----BEGIN"): returned as-is.
//   - Bare base64 (anything else): wrapped in an EC PRIVATE KEY PEM block.
func (c *ABMConfig) PrivateKeyValue() string {
	key := strings.TrimSpace(c.PrivateKey)
	if strings.HasPrefix(key, "-----BEGIN") {
		return key
	}
	if _, err := os.Stat(c.PrivateKey); err == nil {
		return c.PrivateKey // file path
	}
	// Bare base64 — wrap in a PEM block so the upstream parser can handle it.
	return "-----BEGIN EC PRIVATE KEY-----\n" + key + "\n-----END EC PRIVATE KEY-----\n"
}

const (
	BusinessAPIClientPrefix = "BUSINESSAPI."
	SchoolAPIClientPrefix   = "SCHOOLAPI."
	BusinessAPIScope        = "business.api"
	SchoolAPIScope          = "school.api"
	BusinessAPIBaseURL      = "https://api-business.apple.com/"
	SchoolAPIBaseURL        = "https://api-school.apple.com/"
)

// IsSchoolManager reports whether the configured client ID belongs to ASM.
func (c *ABMConfig) IsSchoolManager() bool {
	return strings.HasPrefix(strings.TrimSpace(c.ClientID), SchoolAPIClientPrefix)
}

// OAuthScopeValue returns the configured OAuth scope, auto-detecting a default
// from the client ID prefix when not explicitly set.
func (c *ABMConfig) OAuthScopeValue() string {
	if v := strings.TrimSpace(c.OAuthScope); v != "" {
		return v
	}
	if c.IsSchoolManager() {
		return SchoolAPIScope
	}
	return BusinessAPIScope
}

// APIBaseURLValue returns the configured API base URL, auto-detecting a
// default from the client ID prefix when not explicitly set.
func (c *ABMConfig) APIBaseURLValue() string {
	if v := strings.TrimSpace(c.APIBaseURL); v != "" {
		return v
	}
	if c.IsSchoolManager() {
		return SchoolAPIBaseURL
	}
	return BusinessAPIBaseURL
}

// ValidateABM checks that ABM credentials are set.
func (c *Config) ValidateABM() error {
	if c.ABM.ClientID == "" {
		return fmt.Errorf("abm.client_id is required")
	}
	if c.ABM.KeyID == "" {
		return fmt.Errorf("abm.key_id is required")
	}
	if c.ABM.PrivateKey == "" {
		return fmt.Errorf("abm.private_key is required (file path, inline PEM, or bare base64)")
	}
	if strings.TrimSpace(c.ABM.OAuthScopeValue()) == "" {
		return fmt.Errorf("abm.oauth_scope could not be determined")
	}
	if strings.TrimSpace(c.ABM.APIBaseURLValue()) == "" {
		return fmt.Errorf("abm.api_base_url could not be determined")
	}
	return nil
}

// ValidateSnipeIT checks that Snipe-IT credentials and required IDs are set.
func (c *Config) ValidateSnipeIT() error {
	if c.SnipeIT.URL == "" {
		return fmt.Errorf("snipe_it.url is required")
	}
	if c.SnipeIT.APIKey == "" {
		return fmt.Errorf("snipe_it.api_key is required")
	}
	if c.SnipeIT.ManufacturerID == 0 {
		return fmt.Errorf("snipe_it.manufacturer_id is required")
	}
	if c.SnipeIT.DefaultStatusID == 0 {
		return fmt.Errorf("snipe_it.default_status_id is required")
	}
	if c.SnipeIT.CategoryID == 0 && c.SnipeIT.ComputerCategoryID == 0 && c.SnipeIT.MobileCategoryID == 0 {
		return fmt.Errorf("snipe_it.category_id (or computer_category_id/mobile_category_id) is required")
	}
	return nil
}

// MergeFieldMapping reads a YAML config file, merges new field mappings into
// sync.field_mapping and writes it back. If replaceValues is non-nil, any
// existing entries whose ABM attribute value is in that set are removed first
// (used by setup to replace stale field IDs with fresh ones).
// Comments and structure are preserved via yaml.v3 node API.
func MergeFieldMapping(path string, newMappings map[string]string, replaceValues map[string]bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing config file: %w", err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return fmt.Errorf("unexpected YAML structure")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping at root")
	}

	// Find or create "sync" mapping
	syncNode := findOrCreateMapping(root, "sync")

	// Find or create "field_mapping" mapping under sync
	fmNode := findOrCreateMapping(syncNode, "field_mapping")

	// Remove stale entries whose value is in replaceValues
	if len(replaceValues) > 0 {
		var kept []*yaml.Node
		for i := 0; i < len(fmNode.Content)-1; i += 2 {
			if !replaceValues[fmNode.Content[i+1].Value] {
				kept = append(kept, fmNode.Content[i], fmNode.Content[i+1])
			}
		}
		fmNode.Content = kept
	}

	// Build set of remaining keys
	existing := make(map[string]bool)
	for i := 0; i < len(fmNode.Content)-1; i += 2 {
		existing[fmNode.Content[i].Value] = true
	}

	// Add new mappings (skip if key already exists or is empty)
	for dbCol, abmAttr := range newMappings {
		if dbCol == "" || abmAttr == "" || existing[dbCol] {
			continue
		}
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: dbCol, Tag: "!!str"}
		valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: abmAttr, Tag: "!!str"}
		fmNode.Content = append(fmNode.Content, keyNode, valNode)
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, out, 0600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// findOrCreateMapping finds a key in a mapping node and returns its value node.
// If the key doesn't exist, it creates a new mapping entry. If the value node
// is null/scalar (e.g. "field_mapping:" with no value), it is converted to a
// mapping node in place.
func findOrCreateMapping(parent *yaml.Node, key string) *yaml.Node {
	for i := 0; i < len(parent.Content)-1; i += 2 {
		if parent.Content[i].Value == key {
			valNode := parent.Content[i+1]
			// Handle null/empty value — convert to mapping in place
			if valNode.Kind != yaml.MappingNode {
				valNode.Kind = yaml.MappingNode
				valNode.Tag = "!!map"
				valNode.Value = ""
				valNode.Content = nil
			}
			return valNode
		}
	}
	// Create new mapping
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"}
	valNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	parent.Content = append(parent.Content, keyNode, valNode)
	return valNode
}

// SupplierEntry represents a purchase source to write to the supplier_mapping config.
type SupplierEntry struct {
	Key     string // purchaseSourceId or purchaseSourceType
	Comment string // human-readable comment (e.g. "RESELLER" or "APPLE (id: 1745703)")
}

// MergeSupplierMapping reads a YAML config file and adds commented-out
// supplier_mapping entries for purchase sources not already mapped.
func MergeSupplierMapping(path string, entries []SupplierEntry) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parsing config file: %w", err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return fmt.Errorf("unexpected YAML structure")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping at root")
	}

	syncNode := findOrCreateMapping(root, "sync")
	smNode := findOrCreateMapping(syncNode, "supplier_mapping")

	// Build set of existing keys
	existing := make(map[string]bool)
	for i := 0; i < len(smNode.Content)-1; i += 2 {
		existing[smNode.Content[i].Value] = true
	}

	changed := false
	for _, e := range entries {
		if e.Key == "" || existing[e.Key] {
			continue
		}
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: e.Key, Tag: "!!str"}
		valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "0", Tag: "!!int"}
		if e.Comment != "" {
			keyNode.LineComment = e.Comment
		}
		// Add a TODO head comment above the generated mapping entry.
		todoLabel := e.Comment
		if todoLabel == "" {
			todoLabel = e.Key
		}
		keyNode.HeadComment = fmt.Sprintf("TODO: set Snipe-IT supplier ID for %s", todoLabel)
		smNode.Content = append(smNode.Content, keyNode, valNode)
		existing[e.Key] = true
		changed = true
	}

	if !changed {
		return nil
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.WriteFile(path, out, 0600); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// CategoryIDForFamily returns the appropriate Snipe-IT category ID for a given
// ABM product family (e.g. "Mac", "iPhone", "iPad"). Falls back to CategoryID.
func (c *SnipeITConfig) CategoryIDForFamily(productFamily string) int {
	switch productFamily {
	case "Mac":
		if c.ComputerCategoryID != 0 {
			return c.ComputerCategoryID
		}
	case "iPhone", "iPad", "Watch", "Vision":
		if c.MobileCategoryID != 0 {
			return c.MobileCategoryID
		}
	}
	if c.CategoryID != 0 {
		return c.CategoryID
	}
	// Last resort: return whichever specific one is set
	if c.ComputerCategoryID != 0 {
		return c.ComputerCategoryID
	}
	return c.MobileCategoryID
}
