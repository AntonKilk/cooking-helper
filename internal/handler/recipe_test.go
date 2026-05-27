package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecipeRendersID(t *testing.T) {
	srv := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/recipe/abc123", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "abc123") {
		t.Errorf("body missing recipe id:\n%s", rec.Body.String())
	}
}

func TestRecipeBlankIDNotFound(t *testing.T) {
	rd := &renderer{tmpl: testTemplates(t), bundle: testBundle(t)}
	// No mux match → PathValue("id") is empty, exercising the not-found branch.
	req := httptest.NewRequest(http.MethodGet, "/recipe/", nil)
	rec := httptest.NewRecorder()

	rd.Recipe(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Recipe not found") {
		t.Errorf("body missing not-found message:\n%s", rec.Body.String())
	}
}

func TestRecipeHTMXReturnsFragment(t *testing.T) {
	srv := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/recipe/abc123", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "abc123") {
		t.Errorf("fragment missing recipe id:\n%s", body)
	}
	if strings.Contains(strings.ToLower(body), "<!doctype") {
		t.Errorf("HTMX request returned a full page, want fragment only:\n%s", body)
	}
}
