package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Dashboard HTML integration tests
// ---------------------------------------------------------------------------

func TestDashboardHTML_ManifestLink(t *testing.T) {
	html := string(dashboardHTML)
	if !strings.Contains(html, `rel="manifest"`) {
		t.Error("dashboard.html missing manifest link")
	}
	if !strings.Contains(html, `/dashboard/manifest.json`) {
		t.Error("dashboard.html manifest link has wrong href")
	}
}

func TestDashboardHTML_SWRegistration(t *testing.T) {
	html := string(dashboardHTML)
	if !strings.Contains(html, "serviceWorker") {
		t.Error("dashboard.html missing service worker registration")
	}
	if !strings.Contains(html, "/dashboard/sw.js") {
		t.Error("dashboard.html SW registration has wrong path")
	}
}

func TestDashboardHTML_ThemeColor(t *testing.T) {
	html := string(dashboardHTML)
	if !strings.Contains(html, `name="theme-color"`) {
		t.Error("dashboard.html missing theme-color meta tag")
	}
}

func TestDashboardHTML_InstallButton(t *testing.T) {
	html := string(dashboardHTML)
	if !strings.Contains(html, "pwa-install-btn") {
		t.Error("dashboard.html missing PWA install button")
	}
	if !strings.Contains(html, "pwaInstall") {
		t.Error("dashboard.html missing pwaInstall function")
	}
}

// ---------------------------------------------------------------------------
// Auth middleware bypass test
// ---------------------------------------------------------------------------

func TestDashboardAuthMiddleware_AllowsPWAAssets(t *testing.T) {
	cfg := &Config{
		DashboardAuth: DashboardAuthConfig{
			Enabled:  true,
			Password: "secret",
		},
	}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := dashboardAuthMiddleware(cfg, inner)

	paths := []string{"/dashboard/manifest.json", "/dashboard/sw.js", "/dashboard/icon.svg"}
	for _, p := range paths {
		req := httptest.NewRequest("GET", p, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("path %s returned %d with auth enabled, expected 200 (bypass)", p, rr.Code)
		}
	}
}
