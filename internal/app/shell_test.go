package app

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
	"testing"

	"github.com/HammerMeetNail/nabu/internal/version"
	webassets "github.com/HammerMeetNail/nabu/web"
)

func staticAssets(t *testing.T) fs.FS {
	t.Helper()
	staticFS, err := fs.Sub(webassets.Assets, "static")
	if err != nil {
		t.Fatal(err)
	}
	return staticFS
}

// Every module the app imports must be preloaded and precached, or an offline
// launch fails on the first one that was never fetched.
func TestAppModulePathsCoversImportGraph(t *testing.T) {
	staticFS := staticAssets(t)
	modules := appModulePaths(staticFS)
	if len(modules) < 10 || modules[0] != "js/app.js" {
		t.Fatalf("modules = %v, want js/app.js first and the whole graph", modules)
	}
	listed := map[string]bool{}
	for _, module := range modules {
		if listed[module] {
			t.Errorf("%s listed twice", module)
		}
		listed[module] = true
	}
	for _, module := range modules {
		raw, err := fs.ReadFile(staticFS, module)
		if err != nil {
			t.Fatalf("%s: %v", module, err)
		}
		for _, match := range relativeJSImport.FindAllSubmatch(raw, -1) {
			if dep := path.Join(path.Dir(module), string(match[2])); !listed[dep] {
				t.Errorf("%s imports %s, which is missing from the list", module, dep)
			}
		}
	}
	for _, want := range []string{"js/state.js", "js/browser-context.js", "js/snapshot.js", "js/offline-queue.js"} {
		if !listed[want] {
			t.Errorf("%s missing", want)
		}
	}
}

func TestAppShellPreloadsModulesAndIsMarkedForOffline(t *testing.T) {
	srv := newTestSrv(t)
	rec := srvRequest(srv, http.MethodGet, "/today", "", "", "")
	if rec.Code != http.StatusOK || rec.Header().Get("X-Nabu-Shell") != "1" {
		t.Fatalf("/today: status %d, X-Nabu-Shell %q", rec.Code, rec.Header().Get("X-Nabu-Shell"))
	}
	body := rec.Body.String()
	want := `<link rel="modulepreload" href="/static/js/state.js?v=` + version.Version + `">`
	if !strings.Contains(body, want) {
		t.Errorf("shell is missing %s", want)
	}
	if strings.Contains(body, `rel="modulepreload" href="/static/js/app.js`) {
		t.Error("app.js is the page's module script; it must not also be preloaded")
	}
	// The anonymous marketing page is not the app shell and must not replace it.
	if home := srvRequest(srv, http.MethodGet, "/", "", "", ""); home.Header().Get("X-Nabu-Shell") != "" {
		t.Error("marketing page is marked as the app shell")
	}
}

func TestServiceWorkerPrecachesVersionedShellFiles(t *testing.T) {
	sw := string(buildVersionedSW(staticAssets(t), "test-ver"))
	for _, want := range []string{
		`"nabu-static-test-ver"`,
		`"/static/css/app.css?v=test-ver"`,
		`"/static/js/head-init.js?v=test-ver"`,
		`"/static/js/app.js?v=test-ver"`,
		`"/static/js/snapshot.js?v=test-ver"`,
	} {
		if !strings.Contains(sw, want) {
			t.Errorf("service worker is missing %s", want)
		}
	}
	if strings.Contains(sw, swPrecachePlaceholder) {
		t.Error("precache placeholder was not replaced")
	}
}
