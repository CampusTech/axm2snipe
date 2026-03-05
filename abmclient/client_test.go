package abmclient

import (
	"testing"
	"time"

	"github.com/zchee/abm"
)

func TestDevice_EmbeddedOrgDevice(t *testing.T) {
	d := Device{
		OrgDevice: abm.OrgDevice{
			ID: "DEV001",
			Attributes: &abm.OrgDeviceAttributes{
				SerialNumber:  "TESTSERIAL1",
				DeviceModel:   "MacBook Pro (16-inch, 2024)",
				ProductType:   "Mac16,1",
				Color:         "SILVER",
				ProductFamily: abm.ProductFamilyMac,
			},
		},
		AssignedServer: "TestMDM",
	}

	if d.ID != "DEV001" {
		t.Errorf("ID = %q, want DEV001", d.ID)
	}
	if d.Attributes.SerialNumber != "TESTSERIAL1" {
		t.Errorf("SerialNumber = %q", d.Attributes.SerialNumber)
	}
	if d.AssignedServer != "TestMDM" {
		t.Errorf("AssignedServer = %q", d.AssignedServer)
	}
}

func TestDevice_NilAttributes(t *testing.T) {
	d := Device{OrgDevice: abm.OrgDevice{ID: "DEV002"}}
	if d.Attributes != nil {
		t.Error("expected nil attributes")
	}
	if d.ID != "DEV002" {
		t.Errorf("ID = %q, want DEV002", d.ID)
	}
}

func TestAppleCareCoverage_Fields(t *testing.T) {
	start := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	end := time.Date(2027, 6, 15, 0, 0, 0, 0, time.UTC)

	ac := AppleCareCoverage{
		AgreementNumber: "AGR-TEST-001",
		Description:     "AppleCare+ for Mac",
		StartDateTime:   start,
		EndDateTime:     end,
		Status:          "ACTIVE",
		PaymentType:     "Paid_up_front",
		IsRenewable:     true,
		IsCanceled:      false,
	}

	if ac.AgreementNumber != "AGR-TEST-001" {
		t.Errorf("AgreementNumber = %q", ac.AgreementNumber)
	}
	if ac.Status != "ACTIVE" {
		t.Errorf("Status = %q", ac.Status)
	}
	if !ac.IsRenewable {
		t.Error("IsRenewable should be true")
	}
	if ac.IsCanceled {
		t.Error("IsCanceled should be false")
	}
	if ac.EndDateTime != end {
		t.Errorf("EndDateTime = %v, want %v", ac.EndDateTime, end)
	}
}

func TestSetLogLevel(t *testing.T) {
	// Just verify it doesn't panic
	SetLogLevel(0) // PanicLevel
	SetLogLevel(6) // TraceLevel
}
