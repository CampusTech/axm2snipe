package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/CampusTech/axm2snipe/config"
	"github.com/CampusTech/axm2snipe/snipe"
)

// NewSetupCmd creates the setup command.
func NewSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Create AXM custom fields in Snipe-IT",
		Long:  "Creates AXM custom fields in Snipe-IT, associates them with the configured fieldset, and saves the resulting field mappings to the config file.",
		RunE:  runSetup,
	}
}

func runSetup(cmd *cobra.Command, args []string) error {
	if err := Cfg.ValidateABM(); err != nil {
		return err
	}
	if err := Cfg.ValidateSnipeIT(); err != nil {
		return err
	}
	if Cfg.SnipeIT.CustomFieldsetID == 0 {
		return fmt.Errorf("snipe_it.custom_fieldset_id must be set to use setup")
	}

	if Cfg.Sync.DryRun {
		log.Info("Running in DRY RUN mode - no changes will be made")
	}

	ctx, cancel := contextWithSignal()
	defer cancel()

	abmClient, err := newABMClient(ctx)
	if err != nil {
		return err
	}

	snipeClient, err := newSnipeClient()
	if err != nil {
		return err
	}

	// Fetch MDM server names from ABM for the Assigned MDM Server field options
	log.Info("Fetching MDM servers from ABM...")
	mdmServerNames, err := abmClient.GetMDMServers(ctx)
	if err != nil {
		log.Warnf("Could not fetch MDM servers: %v (Assigned MDM Server field will be a text field)", err)
	}

	mdmServerField := snipe.FieldDef{Name: "AXM: Assigned MDM Server", Element: "text", Format: "ANY", HelpText: "MDM server assigned in Apple Business/School Manager"}
	if len(mdmServerNames) > 0 {
		var names []string
		for _, name := range mdmServerNames {
			if name != "" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			mdmServerField.Element = "listbox"
			mdmServerField.FieldValues = strings.Join(names, "\n")
			log.Infof("Found %d MDM servers: %s", len(names), strings.Join(names, ", "))
		}
	}

	fields := []snipe.FieldDef{
		{Name: "AXM: MDM Assigned?", Element: "text", Format: "BOOLEAN", HelpText: "Whether this device is assigned to an MDM server in ABM/ASM"},
		{Name: "AXM: Added to Org", Element: "text", Format: "DATE", HelpText: "Date device was added to ABM/ASM organization"},
		{Name: "AXM: AppleCare Description", Element: "text", Format: "ANY", HelpText: "AppleCare coverage description"},
		{Name: "AXM: AppleCare Payment Type", Element: "radio", Format: "ANY", HelpText: "AppleCare payment type", FieldValues: "Paid Up Front\nFree\nIncluded\nNone"},
		{Name: "AXM: AppleCare Renewable", Element: "listbox", Format: "BOOLEAN", HelpText: "Whether AppleCare coverage is renewable", FieldValues: "true\nfalse"},
		{Name: "AXM: AppleCare Start Date", Element: "text", Format: "DATE", HelpText: "AppleCare coverage start date"},
		{Name: "AXM: AppleCare Status", Element: "radio", Format: "ANY", HelpText: "AppleCare coverage status", FieldValues: "Active\nInactive\nExpired"},
		mdmServerField,
		{Name: "AXM: Part Number", Element: "text", Format: "ANY", HelpText: "Apple part number (e.g. MW0Y3LL/A)"},
		{Name: "AXM: Released from Org", Element: "text", Format: "DATE", HelpText: "Date device was released from ABM/ASM organization"},
	}

	log.Info("Creating custom fields in Snipe-IT...")
	results, err := snipeClient.SetupFields(Cfg.SnipeIT.CustomFieldsetID, fields)
	if err != nil {
		return fmt.Errorf("setting up fields: %w", err)
	}

	// Map field names to their suggested ABM attribute
	abmAttr := map[string]string{
		"AXM: Added to Org":           "added_to_org",
		"AXM: Released from Org":      "released_from_org",
		"AXM: MDM Assigned?":          "status",
		"AXM: AppleCare Status":       "applecare_status",
		"AXM: AppleCare Description":  "applecare_description",
		"AXM: AppleCare Start Date":   "applecare_start",
		"AXM: AppleCare Renewable":    "applecare_renewable",
		"AXM: AppleCare Payment Type": "applecare_payment_type",
		"AXM: Assigned MDM Server":    "assigned_server",
		"AXM: Part Number":            "part_number",
	}

	// Build field mapping: DB column -> ABM attribute
	fieldMapping := make(map[string]string)
	for name, dbCol := range results {
		if attr, ok := abmAttr[name]; ok {
			fieldMapping[dbCol] = attr
		}
	}

	// Save to config file
	if err := config.MergeFieldMapping(ConfigFile, fieldMapping); err != nil {
		log.Warnf("Could not save field mappings to %s: %v", ConfigFile, err)
		fmt.Println("\nAdd these to your settings.yaml field_mapping manually:")
		for dbCol, attr := range fieldMapping {
			fmt.Printf("    %s: %s\n", dbCol, attr)
		}
	} else {
		fmt.Printf("\nField mappings saved to %s\n", ConfigFile)
	}

	fmt.Println("\nCustom fields created and associated with fieldset:")
	for name, dbCol := range results {
		if attr, ok := abmAttr[name]; ok {
			fmt.Printf("  %s: %s -> %s\n", name, dbCol, attr)
		} else {
			fmt.Printf("  %s: %s\n", name, dbCol)
		}
	}

	// Fetch purchase sources from ABM and write supplier_mapping scaffold
	log.Info("Fetching purchase sources from ABM (this fetches all devices)...")
	purchaseSources, err := abmClient.GetAllPurchaseSources(ctx)
	if err != nil {
		log.Warnf("Could not fetch purchase sources: %v", err)
	} else if len(purchaseSources) > 0 {
		var entries []config.SupplierEntry
		for _, ps := range purchaseSources {
			if ps.Type == "MANUALLY_ADDED" {
				continue // no supplier to map for manually added devices
			}
			if ps.ID != "" {
				entries = append(entries, config.SupplierEntry{
					Key:     ps.ID,
					Comment: fmt.Sprintf("%s (id: %s)", ps.Type, ps.ID),
				})
			} else {
				entries = append(entries, config.SupplierEntry{
					Key:     ps.Type,
					Comment: ps.Type,
				})
			}
		}

		if len(entries) > 0 {
			if Cfg.Sync.DryRun {
				fmt.Println("\nDRY RUN - no changes will be made. Add these to your settings.yaml supplier_mapping manually:")
				for _, e := range entries {
					fmt.Printf("    # %s\n", e.Comment)
					fmt.Printf("    %s: 0  # TODO: set Snipe-IT supplier ID\n", e.Key)
				}
			} else if err := config.MergeSupplierMapping(ConfigFile, entries); err != nil {
				log.Warnf("Could not save supplier mappings to %s: %v", ConfigFile, err)
				fmt.Println("\nAdd these to your settings.yaml supplier_mapping manually:")
				for _, e := range entries {
					fmt.Printf("    # %s\n", e.Comment)
					fmt.Printf("    %s: 0  # TODO: set Snipe-IT supplier ID\n", e.Key)
				}
			} else {
				fmt.Printf("\nSupplier mapping scaffold saved to %s (set the Snipe-IT supplier IDs)\n", ConfigFile)
			}

			fmt.Println("\nPurchase sources found:")
			for _, e := range entries {
				fmt.Printf("  %s: %s\n", e.Key, e.Comment)
			}
		}
	}

	return nil
}
