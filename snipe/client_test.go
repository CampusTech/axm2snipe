package snipe

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	snipeit "github.com/michellepellon/go-snipeit"
)

// newTestClient creates a Client backed by a test HTTP server.
func newTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := NewClient(srv.URL, "test-api-key")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestNewClient_TrimTrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	c, err := NewClient(srv.URL+"/", "test-key")
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

// --- Dry-run enforcement tests ---

func TestCreateModel_DryRun(t *testing.T) {
	c := &Client{DryRun: true}
	_, err := c.CreateModel(context.Background(), snipeit.Model{})
	if !errors.Is(err, ErrDryRun) {
		t.Errorf("expected ErrDryRun, got %v", err)
	}
}

func TestCreateSupplier_DryRun(t *testing.T) {
	c := &Client{DryRun: true}
	_, err := c.CreateSupplier(context.Background(), "Test Supplier")
	if !errors.Is(err, ErrDryRun) {
		t.Errorf("expected ErrDryRun, got %v", err)
	}
}

func TestCreateAsset_DryRun(t *testing.T) {
	c := &Client{DryRun: true}
	_, err := c.CreateAsset(context.Background(), snipeit.Asset{})
	if !errors.Is(err, ErrDryRun) {
		t.Errorf("expected ErrDryRun, got %v", err)
	}
}

func TestPatchAsset_DryRun(t *testing.T) {
	c := &Client{DryRun: true}
	_, err := c.PatchAsset(context.Background(), 1, snipeit.Asset{})
	if !errors.Is(err, ErrDryRun) {
		t.Errorf("expected ErrDryRun, got %v", err)
	}
}

// --- API integration tests (with mock server) ---

func TestGetAssetBySerial(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/hardware/byserial/TESTSERIAL1" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]any{
			"total": 1,
			"rows": []map[string]any{
				{"id": 42, "name": "Test Asset"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	c := newTestClient(t, handler)
	resp, err := c.GetAssetBySerial(context.Background(), "TESTSERIAL1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 {
		t.Fatalf("expected 1 result, got %d", resp.Total)
	}
	if len(resp.Rows) == 0 {
		t.Fatal("expected at least 1 row")
	}
	if resp.Rows[0].ID != 42 {
		t.Errorf("expected asset ID 42, got %d", resp.Rows[0].ID)
	}
}

func TestCreateAsset_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "expected POST", http.StatusMethodNotAllowed)
			return
		}
		resp := map[string]any{
			"status":   "success",
			"messages": "Asset created",
			"payload":  map[string]any{"id": 100, "name": "New Asset"},
		}
		json.NewEncoder(w).Encode(resp)
	})

	c := newTestClient(t, handler)
	asset, err := c.CreateAsset(context.Background(), snipeit.Asset{})
	if err != nil {
		t.Fatal(err)
	}
	if asset.ID != 100 {
		t.Errorf("expected asset ID 100, got %d", asset.ID)
	}
}

func TestCreateAsset_ValidationError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"status":   "error",
			"messages": "Validation failed",
		}
		json.NewEncoder(w).Encode(resp)
	})

	c := newTestClient(t, handler)
	_, err := c.CreateAsset(context.Background(), snipeit.Asset{})
	if err == nil {
		t.Error("expected error for validation failure")
	}
}

func TestListAllModels_Pagination(t *testing.T) {
	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var resp map[string]any
		if callCount == 1 {
			resp = map[string]any{
				"total": 3,
				"rows": []map[string]any{
					{"id": 1, "name": "Model 1"},
					{"id": 2, "name": "Model 2"},
				},
			}
		} else {
			resp = map[string]any{
				"total": 3,
				"rows": []map[string]any{
					{"id": 3, "name": "Model 3"},
				},
			}
		}
		json.NewEncoder(w).Encode(resp)
	})

	c := newTestClient(t, handler)
	models, err := c.ListAllModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 {
		t.Errorf("expected 3 models, got %d", len(models))
	}
	if callCount != 2 {
		t.Errorf("expected 2 API calls for pagination, got %d", callCount)
	}
}

func TestListAllSuppliers_SinglePage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"total": 2,
			"rows": []map[string]any{
				{"id": 1, "name": "Supplier 1"},
				{"id": 2, "name": "Supplier 2"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	c := newTestClient(t, handler)
	suppliers, err := c.ListAllSuppliers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(suppliers) != 2 {
		t.Errorf("expected 2 suppliers, got %d", len(suppliers))
	}
}
