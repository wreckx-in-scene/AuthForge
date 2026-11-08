package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesDashboardAndAssets(t *testing.T) {
	handler := Handler()
	for _, test := range []struct {
		path        string
		contentType string
		contains    string
	}{
		{path: "/", contentType: "text/html", contains: "AuthForge Console"},
		{path: "/reset-password?token=example", contentType: "text/html", contains: "Choose a new password"},
		{path: "/verify-email?environment_id=example&token=example", contentType: "text/html", contains: "Verify your email"},
		{path: "/demo", contentType: "text/html", contains: "Try AuthForge in your browser."},
		{path: "/authorize", contentType: "text/html", contains: "Sign in to your application"},
		{path: "/", contentType: "text/html", contains: "Allowed callback URLs"},
		{path: "/assets/app.js", contentType: "text/javascript", contains: "/v1/console/signup"},
		{path: "/assets/app.js", contentType: "text/javascript", contains: "/v1/environments/${encodeURIComponent(environmentID)}/api-keys"},
		{path: "/assets/styles.css", contentType: "text/css", contains: ".auth-layout"},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, test.path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: got status %d, want %d", test.path, rec.Code, http.StatusOK)
		}
		if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, test.contentType) {
			t.Errorf("%s: got content type %q, want prefix %q", test.path, contentType, test.contentType)
		}
		if !strings.Contains(rec.Body.String(), test.contains) {
			t.Errorf("%s: response does not contain %q", test.path, test.contains)
		}
		if rec.Header().Get("Content-Security-Policy") == "" ||
			rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: security headers are missing", test.path)
		}
	}
}

func TestHandlerDoesNotServeUnknownAssetsOrPaths(t *testing.T) {
	handler := Handler()
	for _, path := range []string{"/missing", "/assets/missing.js", "/assets/nested/app.js"} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: got status %d, want %d", path, rec.Code, http.StatusNotFound)
		}
	}
}
