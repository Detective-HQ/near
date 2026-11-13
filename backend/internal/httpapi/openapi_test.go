package httpapi

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// TestOpenAPIMatchesRoutes fails when docs/openapi.yaml and the router's
// route table drift apart in either direction.
func TestOpenAPIMatchesRoutes(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read openapi.yaml: %v", err)
	}
	var doc struct {
		OpenAPI string                            `yaml:"openapi"`
		Paths   map[string]map[string]interface{} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("openapi.yaml is not valid YAML: %v", err)
	}
	if !strings.HasPrefix(doc.OpenAPI, "3.") {
		t.Fatalf("unexpected openapi version %q", doc.OpenAPI)
	}

	documented := map[string]bool{}
	for path, ops := range doc.Paths {
		for method := range ops {
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}
	h, _ := NewHandler(HandlerOptions{Swaps: mustDemoService(t)})
	registered := map[string]bool{}
	for _, route := range h.Routes() {
		registered[route.Method+" "+route.Path] = true
	}

	var missing, extra []string
	for key := range registered {
		if !documented[key] {
			missing = append(missing, key)
		}
	}
	for key := range documented {
		if !registered[key] {
			extra = append(extra, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("openapi drift:\n  undocumented routes: %v\n  documented but not served: %v", missing, extra)
	}
}
