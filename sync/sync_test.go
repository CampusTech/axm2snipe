package sync

import (
	"context"
	"testing"
	"time"

	snipeit "github.com/michellepellon/go-snipeit"
	"github.com/zchee/abm"

	"github.com/CampusTech/axm2snipe/abmclient"
	"github.com/CampusTech/axm2snipe/config"
)

// --- Pure function tests ---

func TestTitleCase(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"SILVER", "Silver"},
		{"SPACE GRAY", "Space Gray"},
		{"SKY BLUE", "Sky Blue"},
		{"Paid_up_front", "Paid Up Front"},
		{"MANUALLY_ADDED", "Manually Added"},
		{"active", "Active"},
		{"", ""},
		{"SPACE  BLACK", "Space Black"},
	}
	for _, tt := range tests {
		got := titleCase(tt.input)
		if got != tt.want {
			t.Errorf("titleCase(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatMAC(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"2CCA164BD29D", "2C:CA:16:4B:D2:9D"},
		{"aabbccddeeff", "AA:BB:CC:DD:EE:FF"},
		{"2C:CA:16:4B:D2:9D", "2C:CA:16:4B:D2:9D"}, // already formatted
		{"AA-BB-CC-DD-EE-FF", "AA:BB:CC:DD:EE:FF"},   // dash-separated
		{"short", "short"},                             // too short
		{"", ""},
	}
	for _, tt := range tests {
		got := formatMAC(tt.input)
		if got != tt.want {
			t.Errorf("formatMAC(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCleanOrderNumber(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"CDW/1CJ6QLW/002", "1CJ6QLW"},
		{"SIMPLE-ORDER", "SIMPLE-ORDER"},
		{"A/B/C", "B"},
		{"NO-SLASHES", "NO-SLASHES"},
		{"ONE/SLASH", "ONE/SLASH"},
		{"", ""},
	}
	for _, tt := range tests {
		got := cleanOrderNumber(tt.input)
		if got != tt.want {
			t.Errorf("cleanOrderNumber(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNormalizeStorage(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"256GB", "256"},
		{"512GB", "512"},
		{"1TB", "1024"},
		{"2TB", "2048"},
		{"128", "128"},
		{" 256GB ", "256"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeStorage(tt.input)
		if got != tt.want {
			t.Errorf("normalizeStorage(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDeviceSerial(t *testing.T) {
	// With attributes
	d := abmclient.Device{
		OrgDevice: abm.OrgDevice{
			ID: "DEV001",
			Attributes: &abm.OrgDeviceAttributes{
				SerialNumber: "ABC123",
			},
		},
	}
	if got := deviceSerial(d); got != "ABC123" {
		t.Errorf("deviceSerial with attrs = %q, want ABC123", got)
	}

	// Without attributes
	d2 := abmclient.Device{
		OrgDevice: abm.OrgDevice{
			ID: "DEV002",
		},
	}
	if got := deviceSerial(d2); got != "DEV002" {
		t.Errorf("deviceSerial without attrs = %q, want DEV002", got)
	}
}

// --- Filter tests ---

func TestFilterByProductFamily(t *testing.T) {
	devices := []abmclient.Device{
		makeDevice("D1", "SN001", "Mac", "MacBookPro18,1"),
		makeDevice("D2", "SN002", "iPhone", "iPhone14,5"),
		makeDevice("D3", "SN003", "iPad", "iPad13,1"),
		makeDevice("D4", "SN004", "Mac", "Mac14,7"),
	}

	tests := []struct {
		name     string
		families []string
		want     int
	}{
		{"no filter", nil, 4},
		{"mac only", []string{"Mac"}, 2},
		{"iphone only", []string{"iPhone"}, 1},
		{"mac+ipad", []string{"Mac", "iPad"}, 3},
		{"case insensitive", []string{"mac"}, 2},
		{"no match", []string{"Watch"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &Engine{cfg: &config.Config{Sync: config.SyncConfig{ProductFamilies: tt.families}}}
			got := e.filterByProductFamily(devices)
			if len(got) != tt.want {
				t.Errorf("filterByProductFamily(%v) returned %d devices, want %d", tt.families, len(got), tt.want)
			}
		})
	}
}

// --- diffAsset tests ---

func TestDiffAsset_NoChanges(t *testing.T) {
	e := &Engine{cfg: &config.Config{}}
	desired := &snipeit.Asset{
		CommonFields: snipeit.CommonFields{
			CustomFields: map[string]string{
				"_snipeit_color_1": "Silver",
			},
		},
	}
	existing := &snipeit.Asset{
		CommonFields: snipeit.CommonFields{
			CustomFields: map[string]string{
				"_snipeit_color_1": "Silver",
			},
		},
	}
	if diff := e.diffAsset(desired, existing); diff != nil {
		t.Errorf("diffAsset should return nil when no changes, got %+v", diff)
	}
}

func TestDiffAsset_CustomFieldDiff(t *testing.T) {
	e := &Engine{cfg: &config.Config{}}
	desired := &snipeit.Asset{
		CommonFields: snipeit.CommonFields{
			CustomFields: map[string]string{
				"_snipeit_color_1":  "Silver",
				"_snipeit_status_2": "Active",
			},
		},
	}
	existing := &snipeit.Asset{
		CommonFields: snipeit.CommonFields{
			CustomFields: map[string]string{
				"_snipeit_color_1":  "Silver",
				"_snipeit_status_2": "Expired",
			},
		},
	}
	diff := e.diffAsset(desired, existing)
	if diff == nil {
		t.Fatal("diffAsset should return diff when custom fields differ")
	}
	if diff.CustomFields["_snipeit_status_2"] != "Active" {
		t.Errorf("expected _snipeit_status_2=Active, got %q", diff.CustomFields["_snipeit_status_2"])
	}
	if _, ok := diff.CustomFields["_snipeit_color_1"]; ok {
		t.Error("unchanged field _snipeit_color_1 should not be in diff")
	}
}

func TestDiffAsset_SupplierDiff(t *testing.T) {
	e := &Engine{cfg: &config.Config{}}
	desired := &snipeit.Asset{
		CommonFields: snipeit.CommonFields{CustomFields: map[string]string{}},
	}
	desired.Supplier = snipeit.Supplier{CommonFields: snipeit.CommonFields{ID: 5}}

	existing := &snipeit.Asset{
		CommonFields: snipeit.CommonFields{CustomFields: map[string]string{}},
	}
	existing.Supplier = snipeit.Supplier{CommonFields: snipeit.CommonFields{ID: 3}}

	diff := e.diffAsset(desired, existing)
	if diff == nil {
		t.Fatal("diffAsset should detect supplier change")
	}
	if diff.Supplier.ID != 5 {
		t.Errorf("expected supplier ID 5, got %d", diff.Supplier.ID)
	}
}

func TestDiffAsset_WarrantyMonthsDiff(t *testing.T) {
	e := &Engine{cfg: &config.Config{}}
	desired := &snipeit.Asset{
		CommonFields: snipeit.CommonFields{CustomFields: map[string]string{}},
	}
	desired.WarrantyMonths = snipeit.FlexInt(36)

	existing := &snipeit.Asset{
		CommonFields: snipeit.CommonFields{CustomFields: map[string]string{}},
	}
	existing.WarrantyMonths = snipeit.FlexInt(24)

	diff := e.diffAsset(desired, existing)
	if diff == nil {
		t.Fatal("diffAsset should detect warranty_months change")
	}
	if diff.WarrantyMonths.Int() != 36 {
		t.Errorf("expected warranty_months 36, got %d", diff.WarrantyMonths.Int())
	}
}

// --- applyFieldMapping tests ---

func TestApplyFieldMapping(t *testing.T) {
	orderDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	addedDate := time.Date(2024, 7, 1, 12, 0, 0, 0, time.UTC)
	acEnd := time.Date(2027, 6, 15, 0, 0, 0, 0, time.UTC)
	acStart := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

	e := &Engine{cfg: &config.Config{
		Sync: config.SyncConfig{
			FieldMapping: map[string]string{
				"_snipeit_color_1":   "color",
				"_snipeit_storage_2": "device_capacity",
				"_snipeit_mac_3":     "wifi_mac",
				"_snipeit_status_4":  "applecare_status",
				"_snipeit_start_5":   "applecare_start",
				"_snipeit_end_6":     "applecare_end",
				"_snipeit_pay_7":     "applecare_payment_type",
				"_snipeit_renew_8":   "applecare_renewable",
				"_snipeit_server_9":  "assigned_server",
				"_snipeit_added_10":  "added_to_org",
				"order_number":       "order_number",
				"purchase_date":      "order_date",
				"_snipeit_model_11":  "device_model",
				"_snipeit_part_12":   "part_number",
				"_snipeit_family_13": "product_family",
				"_snipeit_type_14":   "product_type",
				"_snipeit_src_15":    "purchase_source",
				"_snipeit_stat_16":   "status",
			},
		},
	}}

	device := abmclient.Device{
		OrgDevice: abm.OrgDevice{
			Attributes: &abm.OrgDeviceAttributes{
				Color:              "SPACE GRAY",
				DeviceCapacity:     "512GB",
				WifiMacAddress:     abm.FlexStringSlice{"AABBCCDDEEFF"},
				OrderNumber:        "CDW/1TESTORD/002",
				OrderDateTime:      orderDate,
				AddedToOrgDateTime: addedDate,
				DeviceModel:        "MacBook Pro (16-inch, 2024)",
				PartNumber:         "TEST1LL/A",
				ProductFamily:      abm.OrgDeviceAttributesProductFamily("Mac"),
				ProductType:        "Mac16,1",
				PurchaseSourceType: abm.OrgDeviceAttributesPurchaseSourceType("RESELLER"),
				Status:             abm.OrgDeviceAttributesStatus("ASSIGNED"),
			},
		},
		AssignedServer: "TestMDM",
	}

	ac := &abmclient.AppleCareCoverage{
		Status:        "ACTIVE",
		StartDateTime: acStart,
		EndDateTime:   acEnd,
		PaymentType:   "Paid_up_front",
		IsRenewable:   true,
	}

	asset := snipeit.Asset{
		CommonFields: snipeit.CommonFields{CustomFields: make(map[string]string)},
	}
	e.applyFieldMapping(&asset, device, ac)

	checks := map[string]string{
		"_snipeit_color_1":   "Space Gray",
		"_snipeit_storage_2": "512",
		"_snipeit_mac_3":     "AA:BB:CC:DD:EE:FF",
		"_snipeit_status_4":  "Active",
		"_snipeit_start_5":   "2024-06-15",
		"_snipeit_end_6":     "2027-06-15",
		"_snipeit_pay_7":     "Paid Up Front",
		"_snipeit_renew_8":   "true",
		"_snipeit_server_9":  "TestMDM",
		"_snipeit_added_10":  "2024-07-01",
		"order_number":       "1TESTORD",
		"purchase_date":      "2024-06-15",
		"_snipeit_model_11":  "MacBook Pro (16-inch, 2024)",
		"_snipeit_part_12":   "TEST1LL/A",
		"_snipeit_family_13": "Mac",
		"_snipeit_type_14":   "Mac16,1",
		"_snipeit_src_15":    "RESELLER",
		"_snipeit_stat_16":   "true",
	}

	for field, want := range checks {
		got := asset.CustomFields[field]
		if got != want {
			t.Errorf("field %q = %q, want %q", field, got, want)
		}
	}

	// warranty_months auto-calculated
	expectedMonths := int(acEnd.Sub(orderDate).Hours() / (24 * 30))
	if asset.WarrantyMonths.Int() != expectedMonths {
		t.Errorf("warranty_months = %d, want %d", asset.WarrantyMonths.Int(), expectedMonths)
	}
}

func TestApplyFieldMapping_NoAppleCare(t *testing.T) {
	e := &Engine{cfg: &config.Config{
		Sync: config.SyncConfig{
			FieldMapping: map[string]string{
				"_snipeit_status_1": "applecare_status",
				"_snipeit_color_2":  "color",
			},
		},
	}}

	device := abmclient.Device{
		OrgDevice: abm.OrgDevice{
			Attributes: &abm.OrgDeviceAttributes{
				Color: "SILVER",
			},
		},
	}

	asset := snipeit.Asset{
		CommonFields: snipeit.CommonFields{CustomFields: make(map[string]string)},
	}
	e.applyFieldMapping(&asset, device, nil)

	if v, ok := asset.CustomFields["_snipeit_status_1"]; ok {
		t.Errorf("applecare_status should be empty with nil AC, got %q", v)
	}
	if asset.CustomFields["_snipeit_color_2"] != "Silver" {
		t.Errorf("color = %q, want Silver", asset.CustomFields["_snipeit_color_2"])
	}
}

func TestApplyFieldMapping_StatusUnassigned(t *testing.T) {
	e := &Engine{cfg: &config.Config{
		Sync: config.SyncConfig{
			FieldMapping: map[string]string{
				"_snipeit_mdm_1": "status",
			},
		},
	}}

	device := abmclient.Device{
		OrgDevice: abm.OrgDevice{
			Attributes: &abm.OrgDeviceAttributes{
				Status: abm.OrgDeviceAttributesStatus("UNASSIGNED"),
			},
		},
	}

	asset := snipeit.Asset{
		CommonFields: snipeit.CommonFields{CustomFields: make(map[string]string)},
	}
	e.applyFieldMapping(&asset, device, nil)

	if asset.CustomFields["_snipeit_mdm_1"] != "false" {
		t.Errorf("status UNASSIGNED should map to 'false', got %q", asset.CustomFields["_snipeit_mdm_1"])
	}
}

func TestApplyFieldMapping_EthernetMAC(t *testing.T) {
	e := &Engine{cfg: &config.Config{
		Sync: config.SyncConfig{
			FieldMapping: map[string]string{
				"_snipeit_eth_1": "ethernet_mac",
			},
		},
	}}

	device := abmclient.Device{
		OrgDevice: abm.OrgDevice{
			Attributes: &abm.OrgDeviceAttributes{
				EthernetMacAddress: []string{"112233445566"},
			},
		},
	}

	asset := snipeit.Asset{
		CommonFields: snipeit.CommonFields{CustomFields: make(map[string]string)},
	}
	e.applyFieldMapping(&asset, device, nil)

	if asset.CustomFields["_snipeit_eth_1"] != "11:22:33:44:55:66" {
		t.Errorf("ethernet_mac = %q, want 11:22:33:44:55:66", asset.CustomFields["_snipeit_eth_1"])
	}
}

// --- ensureModel tests ---

func TestEnsureModel_MatchByProductType(t *testing.T) {
	e := &Engine{
		cfg:    &config.Config{},
		models: map[string]int{"Mac16,10": 42},
	}
	attrs := &abm.OrgDeviceAttributes{
		ProductType: "Mac16,10",
		DeviceModel: "Mac mini (2024)",
	}
	id, err := e.ensureModel(context.Background(), attrs)
	if err != nil {
		t.Fatal(err)
	}
	if id != 42 {
		t.Errorf("expected model ID 42, got %d", id)
	}
}

func TestEnsureModel_MatchByDeviceModel(t *testing.T) {
	e := &Engine{
		cfg:    &config.Config{},
		models: map[string]int{"Mac mini (2024)": 99},
	}
	attrs := &abm.OrgDeviceAttributes{
		ProductType: "Mac16,10",
		DeviceModel: "Mac mini (2024)",
	}
	id, err := e.ensureModel(context.Background(), attrs)
	if err != nil {
		t.Fatal(err)
	}
	if id != 99 {
		t.Errorf("expected model ID 99, got %d", id)
	}
}

func TestEnsureModel_MatchByPartNumber(t *testing.T) {
	e := &Engine{
		cfg:    &config.Config{},
		models: map[string]int{"MW0Y3LL/A": 55},
	}
	attrs := &abm.OrgDeviceAttributes{
		ProductType: "Mac16,10",
		DeviceModel: "Mac mini (2024)",
		PartNumber:  "MW0Y3LL/A",
	}
	id, err := e.ensureModel(context.Background(), attrs)
	if err != nil {
		t.Fatal(err)
	}
	if id != 55 {
		t.Errorf("expected model ID 55, got %d", id)
	}
}

func TestEnsureModel_NoIdentifier(t *testing.T) {
	e := &Engine{
		cfg:    &config.Config{},
		models: map[string]int{},
	}
	attrs := &abm.OrgDeviceAttributes{}
	_, err := e.ensureModel(context.Background(), attrs)
	if err == nil {
		t.Error("expected error for device with no model identifier")
	}
}

func TestEnsureModel_UpdateOnlyMode(t *testing.T) {
	e := &Engine{
		cfg:    &config.Config{Sync: config.SyncConfig{UpdateOnly: true}},
		models: map[string]int{},
	}
	attrs := &abm.OrgDeviceAttributes{
		ProductType: "Mac16,10",
		DeviceModel: "Mac mini (2024)",
	}
	_, err := e.ensureModel(context.Background(), attrs)
	if err == nil {
		t.Error("expected error in update_only mode when model not found")
	}
}

func TestEnsureModel_DryRunMode(t *testing.T) {
	e := &Engine{
		cfg:    &config.Config{Sync: config.SyncConfig{DryRun: true}},
		models: map[string]int{},
		stats:  Stats{},
	}
	attrs := &abm.OrgDeviceAttributes{
		ProductType: "Mac16,10",
		DeviceModel: "Mac mini (2024)",
	}
	id, err := e.ensureModel(context.Background(), attrs)
	if err != nil {
		t.Fatal(err)
	}
	if id != 0 {
		t.Errorf("dry run should return ID 0, got %d", id)
	}
	if e.stats.ModelNew != 1 {
		t.Errorf("dry run should increment ModelNew, got %d", e.stats.ModelNew)
	}
}

// --- ensureSupplier tests ---

func TestEnsureSupplier_MappingByID(t *testing.T) {
	e := &Engine{
		cfg: &config.Config{Sync: config.SyncConfig{
			SupplierMapping: map[string]int{"1C71B60": 10},
		}},
		suppliers: map[string]int{},
	}
	attrs := &abm.OrgDeviceAttributes{
		PurchaseSourceType: "RESELLER",
		PurchaseSourceID:   "1C71B60",
	}
	id, err := e.ensureSupplier(context.Background(), attrs)
	if err != nil {
		t.Fatal(err)
	}
	if id != 10 {
		t.Errorf("expected supplier ID 10, got %d", id)
	}
}

func TestEnsureSupplier_MappingByType(t *testing.T) {
	e := &Engine{
		cfg: &config.Config{Sync: config.SyncConfig{
			SupplierMapping: map[string]int{"APPLE": 5},
		}},
		suppliers: map[string]int{},
	}
	attrs := &abm.OrgDeviceAttributes{
		PurchaseSourceType: "APPLE",
	}
	id, err := e.ensureSupplier(context.Background(), attrs)
	if err != nil {
		t.Fatal(err)
	}
	if id != 5 {
		t.Errorf("expected supplier ID 5, got %d", id)
	}
}

func TestEnsureSupplier_AppleResolvesName(t *testing.T) {
	e := &Engine{
		cfg:       &config.Config{},
		suppliers: map[string]int{"apple": 7},
	}
	attrs := &abm.OrgDeviceAttributes{
		PurchaseSourceType: "APPLE",
	}
	id, err := e.ensureSupplier(context.Background(), attrs)
	if err != nil {
		t.Fatal(err)
	}
	if id != 7 {
		t.Errorf("expected supplier ID 7, got %d", id)
	}
}

func TestEnsureSupplier_ManuallyAddedSkipped(t *testing.T) {
	e := &Engine{
		cfg:       &config.Config{},
		suppliers: map[string]int{},
	}
	attrs := &abm.OrgDeviceAttributes{
		PurchaseSourceType: "MANUALLY_ADDED",
	}
	id, err := e.ensureSupplier(context.Background(), attrs)
	if err != nil {
		t.Fatal(err)
	}
	if id != 0 {
		t.Errorf("MANUALLY_ADDED should return 0, got %d", id)
	}
}

func TestEnsureSupplier_EmptySource(t *testing.T) {
	e := &Engine{
		cfg:       &config.Config{},
		suppliers: map[string]int{},
	}
	attrs := &abm.OrgDeviceAttributes{}
	id, err := e.ensureSupplier(context.Background(), attrs)
	if err != nil {
		t.Fatal(err)
	}
	if id != 0 {
		t.Errorf("empty source should return 0, got %d", id)
	}
}

func TestEnsureSupplier_DryRunMode(t *testing.T) {
	e := &Engine{
		cfg:       &config.Config{Sync: config.SyncConfig{DryRun: true}},
		suppliers: map[string]int{},
	}
	attrs := &abm.OrgDeviceAttributes{
		PurchaseSourceType: "RESELLER",
	}
	id, err := e.ensureSupplier(context.Background(), attrs)
	if err != nil {
		t.Fatal(err)
	}
	if id != 0 {
		t.Errorf("dry run should return 0, got %d", id)
	}
}

// --- processDevice skip conditions ---

func TestProcessDevice_NilAttributes(t *testing.T) {
	e := &Engine{cfg: &config.Config{}, stats: Stats{}}
	device := abmclient.Device{OrgDevice: abm.OrgDevice{}}
	err := e.processDevice(context.Background(), device)
	if err != nil {
		t.Fatal(err)
	}
	if e.stats.Skipped != 1 {
		t.Errorf("nil attrs should skip, got skipped=%d", e.stats.Skipped)
	}
}

func TestProcessDevice_EmptySerial(t *testing.T) {
	e := &Engine{cfg: &config.Config{}, stats: Stats{}}
	device := abmclient.Device{
		OrgDevice: abm.OrgDevice{
			Attributes: &abm.OrgDeviceAttributes{SerialNumber: ""},
		},
	}
	err := e.processDevice(context.Background(), device)
	if err != nil {
		t.Fatal(err)
	}
	if e.stats.Skipped != 1 {
		t.Errorf("empty serial should skip, got skipped=%d", e.stats.Skipped)
	}
}

func TestProcessDevice_NotAvailableSerial(t *testing.T) {
	e := &Engine{cfg: &config.Config{}, stats: Stats{}}
	device := abmclient.Device{
		OrgDevice: abm.OrgDevice{
			Attributes: &abm.OrgDeviceAttributes{SerialNumber: "Not Available"},
		},
	}
	err := e.processDevice(context.Background(), device)
	if err != nil {
		t.Fatal(err)
	}
	if e.stats.Skipped != 1 {
		t.Errorf("'Not Available' serial should skip, got skipped=%d", e.stats.Skipped)
	}
}

// --- Cache round-trip tests ---

func TestCacheRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	devices := []abmclient.Device{
		makeDevice("D1", "SN001", "Mac", "Mac16,10"),
		makeDevice("D2", "SN002", "Mac", "Mac14,7"),
	}

	appleCare := map[string]*abmclient.AppleCareCoverage{
		"D1": {
			Status:      "ACTIVE",
			Description: "AppleCare+ for Mac",
		},
	}

	// Write
	if err := writeJSON(tmpDir, "devices.json", devices); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(tmpDir, "applecare.json", appleCare); err != nil {
		t.Fatal(err)
	}

	// Read back
	e := &Engine{cfg: &config.Config{Sync: config.SyncConfig{CacheDir: tmpDir}}}
	if err := e.LoadCache(); err != nil {
		t.Fatal(err)
	}

	if len(e.cache.Devices) != 2 {
		t.Errorf("expected 2 devices, got %d", len(e.cache.Devices))
	}
	if len(e.cache.AppleCare) != 1 {
		t.Errorf("expected 1 AppleCare entry, got %d", len(e.cache.AppleCare))
	}
	if e.cache.Devices[0].Attributes.SerialNumber != "SN001" {
		t.Errorf("device serial = %q, want SN001", e.cache.Devices[0].Attributes.SerialNumber)
	}
	if e.cache.AppleCare["D1"].Status != "ACTIVE" {
		t.Errorf("AppleCare status = %q, want ACTIVE", e.cache.AppleCare["D1"].Status)
	}
}

func TestLoadCache_MissingAppleCare(t *testing.T) {
	tmpDir := t.TempDir()

	devices := []abmclient.Device{makeDevice("D1", "SN001", "Mac", "Mac16,10")}
	if err := writeJSON(tmpDir, "devices.json", devices); err != nil {
		t.Fatal(err)
	}

	// No applecare.json — should still work
	e := &Engine{cfg: &config.Config{Sync: config.SyncConfig{CacheDir: tmpDir}}}
	if err := e.LoadCache(); err != nil {
		t.Fatal(err)
	}
	if len(e.cache.Devices) != 1 {
		t.Errorf("expected 1 device, got %d", len(e.cache.Devices))
	}
}

func TestCacheDir_Default(t *testing.T) {
	e := &Engine{cfg: &config.Config{}}
	if e.CacheDir() != ".cache" {
		t.Errorf("default CacheDir = %q, want .cache", e.CacheDir())
	}
}

func TestCacheDir_Custom(t *testing.T) {
	e := &Engine{cfg: &config.Config{Sync: config.SyncConfig{CacheDir: "/tmp/test"}}}
	if e.CacheDir() != "/tmp/test" {
		t.Errorf("custom CacheDir = %q, want /tmp/test", e.CacheDir())
	}
}

// --- formatAssetDiff tests ---

func TestFormatAssetDiff(t *testing.T) {
	a := &snipeit.Asset{
		CommonFields: snipeit.CommonFields{
			CustomFields: map[string]string{
				"_snipeit_color_1": "Silver",
			},
		},
	}
	a.Supplier = snipeit.Supplier{CommonFields: snipeit.CommonFields{ID: 5}}
	a.WarrantyMonths = snipeit.FlexInt(36)

	m := formatAssetDiff(a)
	if m["supplier_id"] != 5 {
		t.Errorf("supplier_id = %v, want 5", m["supplier_id"])
	}
	if m["warranty_months"] != 36 {
		t.Errorf("warranty_months = %v, want 36", m["warranty_months"])
	}
	if m["_snipeit_color_1"] != "Silver" {
		t.Errorf("_snipeit_color_1 = %v, want Silver", m["_snipeit_color_1"])
	}
}

// --- helpers ---

func makeDevice(id, serial, family, productType string) abmclient.Device {
	return abmclient.Device{
		OrgDevice: abm.OrgDevice{
			ID: id,
			Attributes: &abm.OrgDeviceAttributes{
				SerialNumber:  serial,
				ProductFamily: abm.OrgDeviceAttributesProductFamily(family),
				ProductType:   productType,
				DeviceModel:   "Test Device",
				Color:         "SILVER",
			},
		},
	}
}
