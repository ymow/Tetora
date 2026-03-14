package pwa

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Manifest tests
// ---------------------------------------------------------------------------

func TestPWAManifest_ValidJSON(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal([]byte(ManifestJSON), &m); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	required := []string{"name", "short_name", "start_url", "display", "icons", "theme_color", "background_color"}
	for _, key := range required {
		if _, ok := m[key]; !ok {
			t.Errorf("manifest missing required field %q", key)
		}
	}
}

func TestPWAManifest_DisplayStandalone(t *testing.T) {
	var m map[string]any
	json.Unmarshal([]byte(ManifestJSON), &m)
	if m["display"] != "standalone" {
		t.Errorf("expected display=standalone, got %v", m["display"])
	}
}

func TestPWAManifest_StartURL(t *testing.T) {
	var m map[string]any
	json.Unmarshal([]byte(ManifestJSON), &m)
	if m["start_url"] != "/dashboard" {
		t.Errorf("expected start_url=/dashboard, got %v", m["start_url"])
	}
}

func TestPWAManifest_ThemeColor(t *testing.T) {
	var m map[string]any
	json.Unmarshal([]byte(ManifestJSON), &m)
	if m["theme_color"] != "#a78bfa" {
		t.Errorf("expected theme_color=#a78bfa, got %v", m["theme_color"])
	}
}

// ---------------------------------------------------------------------------
// Icon tests
// ---------------------------------------------------------------------------

func TestPWAIcon_ValidSVG(t *testing.T) {
	if !strings.Contains(IconSVG, "<svg") {
		t.Error("icon does not contain <svg tag")
	}
	if !strings.Contains(IconSVG, "</svg>") {
		t.Error("icon does not contain closing </svg> tag")
	}
	if !strings.Contains(IconSVG, "xmlns") {
		t.Error("icon missing xmlns attribute")
	}
}

func TestPWAIcon_BrandColors(t *testing.T) {
	if !strings.Contains(IconSVG, "#a78bfa") {
		t.Error("icon missing accent color #a78bfa")
	}
	if !strings.Contains(IconSVG, "#60a5fa") {
		t.Error("icon missing accent2 color #60a5fa")
	}
	if !strings.Contains(IconSVG, "#08080d") {
		t.Error("icon missing background color #08080d")
	}
}

// ---------------------------------------------------------------------------
// Service Worker tests
// ---------------------------------------------------------------------------

func TestPWAServiceWorker_EventListeners(t *testing.T) {
	for _, event := range []string{"install", "activate", "fetch"} {
		if !strings.Contains(ServiceWorkerJS, "'"+event+"'") {
			t.Errorf("service worker missing %q event listener", event)
		}
	}
}

func TestPWAServiceWorker_SkipsPOST(t *testing.T) {
	if !strings.Contains(ServiceWorkerJS, "e.request.method !== 'GET'") {
		t.Error("service worker does not skip non-GET requests")
	}
}

func TestPWAServiceWorker_SkipsSSE(t *testing.T) {
	// The SW uses an allow-list (isShell) approach: only app shell assets are
	// cached; all other paths including SSE streams are passed through via
	// "if (!isShell) return;" rather than an explicit /stream check.
	if !strings.Contains(ServiceWorkerJS, "if (!isShell) return;") {
		t.Error("service worker does not pass through non-shell paths (SSE streams must not be intercepted)")
	}
}

// ---------------------------------------------------------------------------
// Handler tests
// ---------------------------------------------------------------------------

func TestHandlePWAManifest_ContentType(t *testing.T) {
	req := httptest.NewRequest("GET", "/dashboard/manifest.json", nil)
	rr := httptest.NewRecorder()
	HandleManifest(rr, req)
	if ct := rr.Header().Get("Content-Type"); ct != "application/manifest+json" {
		t.Errorf("Content-Type = %q, want application/manifest+json", ct)
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Error("response body is empty")
	}
}

func TestHandlePWAServiceWorker_Headers(t *testing.T) {
	req := httptest.NewRequest("GET", "/dashboard/sw.js", nil)
	rr := httptest.NewRecorder()
	HandleServiceWorker("test-version")(rr, req)
	if ct := rr.Header().Get("Content-Type"); ct != "application/javascript" {
		t.Errorf("Content-Type = %q, want application/javascript", ct)
	}
	if swa := rr.Header().Get("Service-Worker-Allowed"); swa != "/" {
		t.Errorf("Service-Worker-Allowed = %q, want /", swa)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	// Version replacement should have happened.
	body := rr.Body.String()
	if strings.Contains(body, "TETORA_VERSION") {
		t.Error("service worker still contains TETORA_VERSION placeholder")
	}
	if !strings.Contains(body, "test-version") {
		t.Error("service worker does not contain substituted version string")
	}
}

func TestHandlePWAIcon_ContentType(t *testing.T) {
	req := httptest.NewRequest("GET", "/dashboard/icon.svg", nil)
	rr := httptest.NewRecorder()
	HandleIcon(rr, req)
	if ct := rr.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Errorf("Content-Type = %q, want image/svg+xml", ct)
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}
