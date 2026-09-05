package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoadImpressumData_UsesEnvVars(t *testing.T) {
	t.Setenv("IMPRESSUM_NAME", "Jane Doe")
	t.Setenv("IMPRESSUM_ADDRESS", "Musterstraße 1, 12345 Berlin, Germany")
	t.Setenv("IMPRESSUM_EMAIL", "legal@example.com")
	t.Setenv("IMPRESSUM_PHONE", "+49 30 1234567")

	d := loadImpressumData()

	if d.Name != "Jane Doe" {
		t.Errorf("Name = %q, want %q", d.Name, "Jane Doe")
	}
	if d.Address != "Musterstraße 1, 12345 Berlin, Germany" {
		t.Errorf("Address = %q, want the configured address", d.Address)
	}
	if d.Email != "legal@example.com" {
		t.Errorf("Email = %q, want %q", d.Email, "legal@example.com")
	}
	if d.Phone != "+49 30 1234567" {
		t.Errorf("Phone = %q, want %q", d.Phone, "+49 30 1234567")
	}
}

func TestLoadImpressumData_PlaceholdersWhenUnset(t *testing.T) {
	t.Setenv("IMPRESSUM_NAME", "")
	t.Setenv("IMPRESSUM_ADDRESS", "")
	t.Setenv("IMPRESSUM_EMAIL", "")
	t.Setenv("IMPRESSUM_PHONE", "")

	d := loadImpressumData()

	for field, v := range map[string]string{
		"Name": d.Name, "Address": d.Address, "Email": d.Email,
	} {
		if !strings.Contains(v, "IMPRESSUM_") {
			t.Errorf("%s placeholder %q should name the env var to set", field, v)
		}
	}
	// Phone is optional — an unset phone number should render as an empty
	// field, not a placeholder demanding one.
	if d.Phone != "" {
		t.Errorf("Phone = %q, want empty (optional field)", d.Phone)
	}
}

func TestImpressumHandler_RendersConfiguredValues(t *testing.T) {
	t.Setenv("IMPRESSUM_NAME", "Jane Doe")
	t.Setenv("IMPRESSUM_ADDRESS", "Musterstraße 1, 12345 Berlin")
	t.Setenv("IMPRESSUM_EMAIL", "legal@example.com")
	t.Setenv("IMPRESSUM_PHONE", "")
	setupTemplates(t)

	req := httptest.NewRequest(http.MethodGet, "/impressum", nil)
	rec := httptest.NewRecorder()
	impressumHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Jane Doe", "Musterstraße 1, 12345 Berlin", "legal@example.com"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestImpressumHandler_RendersPlaceholdersWhenUnconfigured(t *testing.T) {
	t.Setenv("IMPRESSUM_NAME", "")
	t.Setenv("IMPRESSUM_ADDRESS", "")
	t.Setenv("IMPRESSUM_EMAIL", "")
	t.Setenv("IMPRESSUM_PHONE", "")
	setupTemplates(t)

	req := httptest.NewRequest(http.MethodGet, "/impressum", nil)
	rec := httptest.NewRecorder()
	impressumHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "IMPRESSUM_NAME") {
		t.Error("body should show which env var to set when unconfigured")
	}
}
