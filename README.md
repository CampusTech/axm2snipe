# axm2snipe

Sync devices from Apple Business Manager (ABM) / Apple School Manager (ASM) into [Snipe-IT](https://snipeitapp.com/) asset management.

Inspired by [jamf2snipe](https://github.com/grokability/jamf2snipe), but connects directly to Apple's Business/School Manager API instead of Jamf.

## Features

- Syncs all devices from Apple Business Manager / Apple School Manager into Snipe-IT as hardware assets
- Matches existing Snipe-IT models by hardware identifier (e.g. `Mac16,10`), marketing name, or part number
- Automatically creates Snipe-IT asset models for new device types
- Automatically creates Snipe-IT suppliers from ABM/ASM purchase sources
- Fetches AppleCare coverage details for each device
- Matches existing assets by serial number (create or update)
- Sync a single device by serial number with `sync --serial`
- Timestamp-based change detection to skip unchanged records
- Fully configurable field mapping between ABM/AppleCare attributes and Snipe-IT fields
- Filter by product family (Mac, iPhone, iPad, AppleTV, Watch, Vision)
- Update-only mode to sync data without creating new assets or models
- Resolves assigned MDM server name for each device
- Optional Slack webhook notifications when new assets are created
- Automatic setup of Snipe-IT custom fields with `setup` command
- Automatic retry with exponential backoff for API rate limits
- Dry-run mode with HTTP-level write protection
- MAC address auto-formatting (raw hex to colon-separated)
- Title-casing for colors and AppleCare values

## Prerequisites

### Apple Business Manager / Apple School Manager

1. Sign in to [Apple Business Manager](https://business.apple.com) or [Apple School Manager](https://school.apple.com)
2. Go to **Settings > API**
3. Create a new API key and note the **Client ID** and **Key ID**
4. Download the private key `.pem` file

### Snipe-IT

1. Generate an API key at **Admin > API Keys**
2. Note the IDs for:
   - **Apple manufacturer** (create one under Manufacturers if needed)
   - **Default status label** for new assets
   - **Category** for auto-created models (can be per product family)
   - (Optional) **Custom fieldset** for ABM/AppleCare fields

## Installation

```bash
go install github.com/CampusTech/axm2snipe@latest
```

Or build from source:

```bash
git clone https://github.com/CampusTech/axm2snipe.git
cd axm2snipe
go build -o axm2snipe .
```

## Configuration

Copy the example config and fill in your values:

```bash
cp settings.example.yaml settings.yaml
```

See [settings.example.yaml](settings.example.yaml) for all options and documentation.

## Usage

```bash
# Test connections to ABM and Snipe-IT
axm2snipe test

# Download ABM data to local cache
axm2snipe download -v

# Dry run sync (no changes made - writes blocked at HTTP level)
axm2snipe sync --dry-run -v

# Full sync
axm2snipe sync -v

# Sync from cached data (avoids ABM API calls)
axm2snipe sync --use-cache -v

# Sync a single device by serial number
axm2snipe sync --serial ABCD1234XYZ -v

# Force update all assets (ignore timestamps)
axm2snipe sync --force -v

# Auto-create AXM custom fields in Snipe-IT and save mappings to config
axm2snipe setup -v

# Debug logging
axm2snipe sync -d

# Print an ABM access token (useful for manual API testing with curl)
axm2snipe access-token

# Make an authenticated ABM API request
axm2snipe request https://mdmenrollment.apple.com/server/devices
```

### Commands

| Command | Description |
| --- | --- |
| `sync` | Sync ABM/ASM devices into Snipe-IT |
| `download` | Download ABM/ASM data to local cache |
| `setup` | Create AXM custom fields in Snipe-IT and save mappings to config |
| `test` | Test connections to ABM/ASM and Snipe-IT |
| `access-token` | Print an ABM API access token |
| `request <url>` | Make an authenticated ABM API GET request |

### Global Flags

| Flag | Description |
| --- | --- |
| `--config` | Path to config file (default: `settings.yaml`) |
| `-v, --verbose` | Verbose output (INFO level) |
| `-d, --debug` | Debug output (DEBUG level) |
| `--version` | Show version and exit |

### Sync Flags

| Flag | Description |
| --- | --- |
| `--dry-run` | Simulate sync without making changes |
| `--force` | Ignore timestamps, always update |
| `--serial` | Sync a single device by serial number (implies `--force`) |
| `--use-cache` | Use cached data instead of fetching from ABM API |
| `--update-only` | Only update existing assets, never create new ones |
| `--cache-dir` | Directory for cached API responses (default: `.cache`) |

### Config Options

| Key | Description |
| --- | --- |
| `sync.dry_run` | Same as `--dry-run` flag |
| `sync.force` | Same as `--force` flag |
| `sync.rate_limit` | Enable rate limiting for API calls |
| `sync.update_only` | Only update existing assets, never create new assets/models/suppliers |
| `sync.product_families` | Filter devices by product family (`Mac`, `iPhone`, `iPad`, `Watch`, `Vision`) |
| `sync.use_cache` | Same as `--use-cache` flag |
| `sync.set_name` | Set asset name to "Model (Color)" on create |
| `sync.supplier_mapping` | Map ABM/ASM purchase source IDs/types to Snipe-IT supplier IDs |
| `sync.field_mapping` | Map Snipe-IT fields to ABM/AppleCare source values |
| `snipe_it.computer_category_id` | Category ID for Mac models (overrides `category_id` for Macs) |
| `snipe_it.mobile_category_id` | Category ID for iPhone/iPad/Watch/Vision models |
| `snipe_it.custom_fieldset_id` | Fieldset ID to attach to auto-created models |
| `slack.enabled` | Enable Slack webhook notifications |
| `slack.webhook_url` | Slack incoming webhook URL |

## How It Works

1. **Connects** to Apple Business Manager / Apple School Manager and Snipe-IT APIs
2. **Fetches** all Snipe-IT models and suppliers to build local caches
3. **Retrieves** all devices from ABM/ASM (with optional product family filter)
4. For each device:
   - **Looks up** the asset in Snipe-IT by serial number
   - If not found and `update_only` is true: **skips**
   - If not found: **creates** a new asset (after resolving model and supplier)
   - **Resolves** model by matching ProductType (`Mac16,10`), then DeviceModel (`Mac mini (2024)`), then PartNumber (`MW0Y3LL/A`) against Snipe-IT model numbers and names
   - If found: **updates** the asset with mapped fields (if ABM data is newer or `--force`)
   - If multiple matches: **skips** with a warning
   - **Fetches** AppleCare coverage details from ABM/ASM
   - **Resolves** supplier from ABM/ASM purchase source (auto-creates if needed)

### Field Mapping

All field mappings are configured in `settings.yaml` under `sync.field_mapping`. The left side is the Snipe-IT field name (custom fields use the `_snipeit_*` DB column name), the right side is the ABM/AppleCare source value.

**ABM device attributes:**

| Source value | Description |
| --- | --- |
| `serial_number` | Device serial number |
| `device_model` | Marketing name (e.g. "Mac mini (2024)") |
| `color` | Device color (auto title-cased: SILVER -> Silver) |
| `device_capacity` | Storage capacity (e.g. "256GB") |
| `part_number` | Apple part number (e.g. "MW0Y3LL/A") |
| `product_family` | Product family (Mac, iPhone, iPad, etc.) |
| `product_type` | Hardware model identifier (e.g. "Mac16,10") -- used first for model matching |
| `order_number` | Order number (CDW-style orders auto-cleaned) |
| `order_date` | Order date (formatted YYYY-MM-DD) |
| `purchase_source` | Purchase source name |
| `status` | ABM device status |
| `imei` | IMEI number(s) |
| `meid` | MEID number(s) |
| `wifi_mac` | Wi-Fi MAC address, auto-formatted with colons |
| `bluetooth_mac` | Bluetooth MAC address, auto-formatted with colons |
| `ethernet_mac` | Ethernet MAC address(es), auto-formatted with colons |
| `eid` | eSIM EID |
| `added_to_org` | Date added to organization |
| `released_from_org` | Date released from organization |
| `assigned_server` | Assigned MDM server name |

**AppleCare coverage:**

| Source value | Description |
| --- | --- |
| `applecare_status` | Coverage status (auto title-cased: Active, Inactive, Expired) |
| `applecare_agreement` | Agreement/contract number |
| `applecare_description` | Coverage description |
| `applecare_start` | Coverage start date |
| `applecare_end` | Coverage end date |
| `applecare_renewable` | Whether coverage is renewable (true/false) |
| `applecare_payment_type` | Payment type (auto title-cased: Paid Up Front, Free, etc.) |

**Standard Snipe-IT fields** (use as the left side of field_mapping):

| Field | Description |
| --- | --- |
| `purchase_date` | Asset purchase date |
| `order_number` | Purchase order number |
| `warranty_months` | Auto-calculated from purchase date to AppleCare end date (not configurable) |

Example field mapping:

```yaml
sync:
  field_mapping:
    # ABM device attributes -> Snipe-IT custom fields
    _snipeit_mac_address_1: wifi_mac
    _snipeit_storage_3: device_capacity
    _snipeit_color_7: color
    purchase_date: order_date
    order_number: order_number
    # AppleCare coverage -> Snipe-IT custom fields
    _snipeit_warranty_end_date_4: applecare_end
    _snipeit_warranty_id_5: applecare_agreement
    _snipeit_applecare_status_9: applecare_status
    _snipeit_applecare_description_10: applecare_description
    _snipeit_applecare_start_date_11: applecare_start
    _snipeit_applecare_renewable_12: applecare_renewable
    _snipeit_applecare_payment_type_13: applecare_payment_type
    _snipeit_assigned_mdm_server_14: assigned_server
```

## Snipe-IT Custom Fields Setup

Use the `setup` command to automatically create custom fields in Snipe-IT:

```bash
axm2snipe setup -v
```

This will:
1. Connect to ABM/ASM to fetch MDM server names (used as listbox options) and all purchase sources
2. Create AXM custom fields in Snipe-IT
3. Associate them with your configured fieldset (`snipe_it.custom_fieldset_id`)
4. Save the resulting field mappings to your config file under `sync.field_mapping`
5. Scaffold `sync.supplier_mapping` entries for every purchase source found (excluding `MANUALLY_ADDED` sources, which require manual mapping), with TODO comments to fill in Snipe-IT supplier IDs

Alternatively, create them manually:

1. Create custom fields under **Admin > Custom Fields**
2. Note the **DB column name** for each field (e.g. `_snipeit_applecare_status_9`)
3. Create a **Custom Fieldset** grouping these fields
4. Assign the fieldset to your asset models
5. Set `custom_fieldset_id` in your config so new models get the fieldset automatically
6. Add entries to `field_mapping` using the DB column names as keys

### Recommended Field Types

| Field | Element | Format | Values |
| --- | --- | --- | --- |
| MAC Address | text | MAC | -- |
| Color | text | ANY | -- |
| Storage | text | ANY | -- |
| Warranty End Date | text | DATE | -- |
| AppleCare Status | radio | ANY | Active, Inactive, Expired |
| AppleCare Start Date | text | DATE | -- |
| AppleCare Renewable | listbox | BOOLEAN | true, false |
| AppleCare Payment Type | radio | ANY | Paid Up Front, Free, Included, None |
| Assigned MDM Server | text | ANY | -- |

## Slack Notifications

axm2snipe can send a Slack notification whenever a new device is created in Snipe-IT. Messages use Slack Block Kit with device details, AppleCare status, MDM server, and a link to the asset in Snipe-IT.

To enable, [create an incoming webhook](https://api.slack.com/messaging/webhooks) and add to your config:

```yaml
slack:
  enabled: true
  webhook_url: "https://hooks.slack.com/services/T.../B.../..."
```

## License

[MIT](LICENSE.md)
