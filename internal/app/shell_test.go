package app

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"slices"
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

// shellBudgetCompressed is the plan's slow-4G budget for a cold launch: Home
// within 1.5 s on 150 ms round trips and 1.6 Mbit/s. With every module
// preloaded, a cold launch takes three round trips (the page, its modules and
// stylesheet, then Home's data), 0.45 s, which leaves 1.05 s to carry the
// shell: 210 KB compressed. Production sends Brotli, smaller than the gzip
// measured here.
const shellBudgetCompressed = 210 * 1024

func TestShellFitsTheSlow4GBudget(t *testing.T) {
	staticFS := staticAssets(t)
	files := append(appModulePaths(staticFS), "css/app.css")
	type size struct {
		file  string
		bytes int
	}
	sizes := make([]size, 0, len(files))
	total := 0
	for _, file := range files {
		raw, err := fs.ReadFile(staticFS, file)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := zw.Write(raw); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, size{file, buf.Len()})
		total += buf.Len()
	}
	t.Logf("the shell is %d files, %d KB compressed; the budget is %d KB", len(files), total/1024, shellBudgetCompressed/1024)
	if total > shellBudgetCompressed {
		slices.SortFunc(sizes, func(a, b size) int { return b.bytes - a.bytes })
		var largest []string
		for _, s := range sizes[:min(5, len(sizes))] {
			largest = append(largest, fmt.Sprintf("%s %d KB", s.file, s.bytes/1024))
		}
		t.Fatalf("the shell is %d KB compressed, over the %d KB a cold Home can load in 1.5 s on slow 4G; largest: %s",
			total/1024, shellBudgetCompressed/1024, strings.Join(largest, ", "))
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

// A Home Screen app added from the shell gets an opaque status bar: from iOS
// 26 the system blurs about 40pt below a translucent one, over the top bar.
// The status bar takes the theme color, the top bar's surface in each scheme.
func TestAppShellAsksForAnOpaqueStatusBar(t *testing.T) {
	srv := newTestSrv(t)
	body := srvRequest(srv, http.MethodGet, "/today", "", "", "").Body.String()
	if strings.Contains(body, "apple-mobile-web-app-status-bar-style") {
		t.Error("the shell sets a status bar style; under a translucent one iOS blurs the top bar")
	}
	for _, want := range []string{
		`<meta name="theme-color" content="#FFFFFF" media="(prefers-color-scheme: light)">`,
		`<meta name="theme-color" content="#181C27" media="(prefers-color-scheme: dark)">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("shell is missing %s", want)
		}
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
