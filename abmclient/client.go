// Package abmclient wraps the abm library's Client with axm2snipe-specific
// logic: MDM server name resolution via device linkages, flattened AppleCare
// coverage, and a simplified constructor.
package abmclient

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/CampusTech/abm"
)

var log = logrus.New()

// SetLogLevel sets the logger level for the abmclient package.
func SetLogLevel(level logrus.Level) {
	log.SetLevel(level)
}

// SetLogFormatter sets the logger formatter for the abmclient package.
func SetLogFormatter(formatter logrus.Formatter) {
	log.SetFormatter(formatter)
}

// SetLogOutput sets the logger output for the abmclient package.
func SetLogOutput(output io.Writer) {
	log.SetOutput(output)
}

// Client wraps an abm.Client with axm2snipe-specific helpers.
type Client struct {
	abm *abm.Client
}

// NewClient creates a new ABM/ASM client using the abm library for auth.
// Rate limiting and retry logic are handled by the upstream abm library.
func NewClient(ctx context.Context, clientID, keyID, privateKey, scope, baseURL string) (*Client, error) {
	assertion, err := abm.NewAssertion(ctx, clientID, keyID, privateKey)
	if err != nil {
		return nil, fmt.Errorf("creating ABM assertion: %w", err)
	}

	ts, err := abm.NewTokenSource(ctx, nil, clientID, assertion, scope)
	if err != nil {
		return nil, fmt.Errorf("creating ABM token source: %w", err)
	}

	client, err := abm.NewClientWithBaseURL(nil, ts, baseURL)
	if err != nil {
		return nil, fmt.Errorf("creating ABM client: %w", err)
	}

	return &Client{abm: client}, nil
}

// Device wraps an abm.OrgDevice with the resolved MDM server name.
type Device struct {
	abm.OrgDevice
	AssignedServer string `json:"assignedServerName,omitempty"`
}

// AppleCareCoverage holds flattened AppleCare coverage data for easy access in sync.
type AppleCareCoverage struct {
	AgreementNumber        string    `json:"agreementNumber,omitempty"`
	ContractCancelDateTime time.Time `json:"contractCancelDateTime,omitempty"`
	Description            string    `json:"description,omitempty"`
	EndDateTime            time.Time `json:"endDateTime,omitempty"`
	IsCanceled             bool      `json:"isCanceled,omitempty"`
	IsRenewable            bool      `json:"isRenewable,omitempty"`
	PaymentType            string    `json:"paymentType,omitempty"`
	StartDateTime          time.Time `json:"startDateTime,omitempty"`
	Status                 string    `json:"status,omitempty"`
}

// ConnectionTest fetches a single device to verify the ABM connection.
// Returns the total device count.
func (c *Client) ConnectionTest(ctx context.Context) (int, error) {
	resp, err := c.abm.GetOrgDevices(ctx, &abm.GetOrgDevicesOptions{Limit: 1})
	if err != nil {
		return 0, err
	}
	total := 0
	if resp.Meta != nil {
		total = resp.Meta.Paging.Total
	}
	return total, nil
}

// GetMDMServers fetches all MDM servers and returns a map of server ID to name.
func (c *Client) GetMDMServers(ctx context.Context) (map[string]string, error) {
	resp, err := c.abm.GetMDMServers(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching MDM servers: %w", err)
	}
	servers := make(map[string]string, len(resp.Data))
	for _, s := range resp.Data {
		if s.Attributes != nil {
			servers[s.ID] = s.Attributes.ServerName
		}
	}
	return servers, nil
}

// BuildDeviceServerMap fetches all MDM servers and their device linkages
// to build a map from device ID to server name.
func (c *Client) BuildDeviceServerMap(ctx context.Context) (map[string]string, error) {
	serverNames, err := c.GetMDMServers(ctx)
	if err != nil {
		return nil, err
	}
	log.Infof("Found %d MDM servers for device linkage resolution", len(serverNames))

	deviceToServer := make(map[string]string)
	for serverID, serverName := range serverNames {
		resp, err := c.abm.GetMDMServerDeviceLinkages(ctx, serverID, &abm.GetMDMServerDeviceLinkagesOptions{Limit: 1000})
		if err != nil {
			log.WithError(err).WithField("server", serverName).Warn("Could not fetch device linkages for MDM server")
			continue
		}
		for _, linkage := range resp.Data {
			deviceToServer[linkage.ID] = serverName
		}
		log.Debugf("MDM server %q: %d device linkages", serverName, len(resp.Data))

		if resp.Links.Next != "" {
			log.Warnf("MDM server %q has more than 1000 device linkages; pagination not yet supported, some devices may not have server names resolved", serverName)
		}
	}
	log.Infof("Built device-to-server map with %d entries", len(deviceToServer))
	return deviceToServer, nil
}

// GetAllDevices fetches all devices and resolves assigned MDM server names.
// The device list and MDM server map are fetched concurrently.
func (c *Client) GetAllDevices(ctx context.Context) ([]Device, int, error) {
	type serverMapResult struct {
		m   map[string]string
		err error
	}
	type devicesResult struct {
		devices []abm.OrgDevice
		total   int
		err     error
	}

	serverMapCh := make(chan serverMapResult, 1)
	devicesCh := make(chan devicesResult, 1)

	go func() {
		m, err := c.BuildDeviceServerMap(ctx)
		serverMapCh <- serverMapResult{m, err}
	}()
	go func() {
		devices, total, err := c.abm.FetchAllOrgDevices(ctx)
		devicesCh <- devicesResult{devices, total, err}
	}()

	smr := <-serverMapCh
	dr := <-devicesCh

	deviceToServer := smr.m
	if smr.err != nil {
		// Non-fatal: continue without server names, but warn since mdm_only filtering
		// will treat all devices as unassigned if this fails.
		log.WithError(smr.err).Warn("Could not build device-server map; AssignedServer will be empty for all devices (mdm_only filtering may incorrectly skip managed devices)")
		deviceToServer = make(map[string]string)
	}
	if dr.err != nil {
		return nil, 0, dr.err
	}

	// Wrap with resolved server names
	devices := make([]Device, len(dr.devices))
	for i, od := range dr.devices {
		devices[i] = Device{OrgDevice: od}
		if name, ok := deviceToServer[od.ID]; ok {
			devices[i].AssignedServer = name
		}
	}

	return devices, dr.total, nil
}

// GetDevice fetches a single device by serial number and resolves its assigned MDM server name.
func (c *Client) GetDevice(ctx context.Context, serial string) (*Device, error) {
	resp, err := c.abm.GetOrgDevice(ctx, serial, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching device %s: %w", serial, err)
	}

	device := &Device{OrgDevice: resp.Data}

	// Resolve assigned MDM server name
	serverResp, err := c.abm.GetOrgDeviceAssignedServer(ctx, serial, nil)
	if err != nil {
		log.WithError(err).WithField("serial", serial).Debug("Could not fetch assigned MDM server")
	} else if serverResp.Data.Attributes != nil {
		device.AssignedServer = serverResp.Data.Attributes.ServerName
	}

	return device, nil
}

// PurchaseSource represents a unique ABM purchase source.
type PurchaseSource struct {
	Type string // e.g. "RESELLER", "APPLE", "MANUALLY_ADDED"
	ID   string // purchaseSourceId (may be empty)
}

// collectPurchaseSources extracts unique purchase sources from a slice of ABM devices.
func collectPurchaseSources(devices []abm.OrgDevice) []PurchaseSource {
	seen := make(map[PurchaseSource]bool)
	var sources []PurchaseSource
	for _, d := range devices {
		if d.Attributes == nil {
			continue
		}
		srcType := string(d.Attributes.PurchaseSourceType)
		if srcType == "" {
			continue
		}
		ps := PurchaseSource{Type: srcType, ID: d.Attributes.PurchaseSourceID}
		if !seen[ps] {
			seen[ps] = true
			sources = append(sources, ps)
		}
	}
	return sources
}

// GetAllPurchaseSources fetches all devices and returns the unique purchase sources.
func (c *Client) GetAllPurchaseSources(ctx context.Context) ([]PurchaseSource, error) {
	devices, _, err := c.abm.FetchAllOrgDevices(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching devices: %w", err)
	}
	return collectPurchaseSources(devices), nil
}

// CoverageResult holds both the "winning" AppleCare record and the full list
// of all coverage records for a device.
type CoverageResult struct {
	Best *AppleCareCoverage   // selected by priority rules (may be nil)
	All  []AppleCareCoverage  // all records, including Limited Warranty
}

// GetAppleCareCoverage fetches AppleCare coverage for a device.
// It returns a CoverageResult with all records and the best one selected by
// priority: ACTIVE > PAID_UP_FRONT payment type > latest end date.
func (c *Client) GetAppleCareCoverage(ctx context.Context, deviceID string) (*CoverageResult, error) {
	resp, err := c.abm.GetOrgDeviceAppleCareCoverage(ctx, deviceID, nil)
	if err != nil {
		return nil, err
	}

	var all []AppleCareCoverage
	var bestIdx int = -1
	var best *abm.AppleCareCoverageAttributes

	for i := range resp.Data {
		a := resp.Data[i].Attributes
		if a == nil {
			continue
		}
		all = append(all, AppleCareCoverage{
			AgreementNumber:        a.AgreementNumber,
			ContractCancelDateTime: a.ContractCancelDateTime,
			Description:            a.Description,
			EndDateTime:            a.EndDateTime,
			IsCanceled:             a.IsCanceled,
			IsRenewable:            a.IsRenewable,
			PaymentType:            string(a.PaymentType),
			StartDateTime:          a.StartDateTime,
			Status:                 string(a.Status),
		})
		if best == nil {
			best = a
			bestIdx = len(all) - 1
			continue
		}
		// Prefer ACTIVE over non-ACTIVE
		bestActive := string(best.Status) == "ACTIVE"
		aActive := string(a.Status) == "ACTIVE"
		if aActive && !bestActive {
			best = a
			bestIdx = len(all) - 1
			continue
		}
		if bestActive && !aActive {
			continue
		}
		// Both same activity status: prefer paid coverage over free/none
		bestPaid := string(best.PaymentType) == "PAID_UP_FRONT"
		aPaid := string(a.PaymentType) == "PAID_UP_FRONT"
		if aPaid && !bestPaid {
			best = a
			bestIdx = len(all) - 1
			continue
		}
		if bestPaid && !aPaid {
			continue
		}
		// Final tiebreaker: prefer later end date
		if a.EndDateTime.After(best.EndDateTime) {
			best = a
			bestIdx = len(all) - 1
		}
	}

	if len(all) == 0 {
		return nil, nil
	}
	result := &CoverageResult{All: all}
	if bestIdx >= 0 {
		result.Best = &all[bestIdx]
	}
	return result, nil
}
