package front

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedModuleGraph(t *testing.T) {
	index, err := FS.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), `<script type="module" src="/app.js"></script>`) {
		t.Fatal("console entry must load as an ES module")
	}
	imports := regexp.MustCompile(`(?m)^import\s*\{([^}]+)\}\s*from\s*['"]([^'"]+)['"]`)
	exported := regexp.MustCompile(`(?m)^export (?:async )?function\s+([\w$]+)\(`)
	visited, visiting := map[string]bool{}, map[string]bool{}
	var visit func(string)
	visit = func(name string) {
		if visiting[name] {
			t.Fatalf("cyclic module dependency at %s", name)
		}
		if visited[name] {
			return
		}
		visiting[name] = true
		data, err := FS.ReadFile(name)
		if err != nil {
			t.Fatalf("module %s is not embedded: %v", name, err)
		}
		for _, imp := range imports.FindAllStringSubmatch(string(data), -1) {
			if !strings.HasPrefix(imp[2], "./") {
				t.Fatalf("%s imports non-local module %s", name, imp[2])
			}
			dependency := path.Join(path.Dir(name), imp[2])
			visit(dependency)
			dependencyData, _ := FS.ReadFile(dependency)
			names := map[string]bool{}
			for _, match := range exported.FindAllStringSubmatch(string(dependencyData), -1) {
				names[match[1]] = true
			}
			for _, symbol := range strings.Split(imp[1], ",") {
				if symbol = strings.TrimSpace(symbol); symbol != "" && !names[symbol] {
					t.Errorf("%s imports missing export %s from %s", name, symbol, dependency)
				}
			}
		}
		visiting[name] = false
		visited[name] = true
	}
	visit("app.js")
	modules, err := fs.Glob(FS, "modules/*.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range modules {
		if !visited[module] {
			t.Errorf("embedded module is not reachable: %s", module)
		}
	}
}

func TestEmbeddedModulesAreServedAsJavaScript(t *testing.T) {
	modules, err := fs.Glob(FS, "modules/*.js")
	if err != nil || len(modules) == 0 {
		t.Fatalf("no modules embedded: %v", err)
	}
	handler := http.FileServer(http.FS(FS))
	for _, name := range append(modules, "app.js") {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/"+name, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Type"), "javascript") {
			t.Errorf("%s: status %d, content type %q", name, response.Code, response.Header().Get("Content-Type"))
		}
	}
}
