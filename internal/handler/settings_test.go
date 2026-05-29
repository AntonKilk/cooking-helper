package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsRendersProfileLinkAndLanguageSwitcher(t *testing.T) {
	srv := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `href="/settings/profile"`) {
		t.Errorf("settings missing profile link:\n%s", body)
	}
	if !strings.Contains(body, `href="/settings/disliked"`) {
		t.Errorf("settings missing disliked link:\n%s", body)
	}
	for _, lang := range []string{`value="ru"`, `value="fi"`, `value="en"`} {
		if !strings.Contains(body, lang) {
			t.Errorf("settings missing language button %s", lang)
		}
	}
	if !strings.Contains(body, `action="/settings/language"`) {
		t.Error("settings missing language switcher form")
	}
}
