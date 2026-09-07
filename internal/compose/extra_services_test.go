package compose

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/phlisg/frank/internal/config"
)

// extraCfg returns a minimal config carrying the given extra_services blocks.
func extraCfg(extra map[string]map[string]any) *config.Config {
	return &config.Config{
		PHP:           config.PHP{Version: "8.5", Runtime: "frankenphp"},
		Laravel:       config.Laravel{Version: "latest"},
		Services:      []string{"pgsql"},
		ExtraServices: extra,
	}
}

// parseCompose unmarshals generated compose.yaml back into a map so tests can
// assert on structure rather than on YAML formatting.
func parseCompose(t *testing.T, out string) map[string]interface{} {
	t.Helper()

	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("parse generated compose.yaml: %v\n%s", err, out)
	}

	return parsed
}

func composeService(t *testing.T, out, name string) map[string]interface{} {
	t.Helper()

	parsed := parseCompose(t, out)

	services, ok := parsed["services"].(map[string]interface{})
	if !ok {
		t.Fatalf("no services map in output:\n%s", out)
	}

	svc, ok := services[name].(map[string]interface{})
	if !ok {
		t.Fatalf("service %q missing from output:\n%s", name, out)
	}

	return svc
}

func TestExtraServices_PortsRewrittenInWorktree(t *testing.T) {
	g := newTestGenerator(t)
	cfg := extraCfg(map[string]map[string]any{
		"gotenberg": {
			"image": "gotenberg/gotenberg:8",
			"ports": []interface{}{
				"3000:3000",
				"8080:3001",
				"127.0.0.1:8081:3002",
				"3003",
				"9000:3004/udp",
				map[string]interface{}{"target": 3005, "published": 8085},
			},
		},
	})

	out, err := g.Generate(cfg, "wt", "main", true, 5187)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	ports, ok := composeService(t, out, "gotenberg")["ports"].([]interface{})
	if !ok {
		t.Fatalf("gotenberg has no ports:\n%s", out)
	}

	want := []string{"3000", "3001", "3002", "3003", "3004/udp"}
	for i, w := range want {
		if got, _ := ports[i].(string); got != w {
			t.Errorf("ports[%d] = %v, want %q", i, ports[i], w)
		}
	}

	long, ok := ports[5].(map[string]interface{})
	if !ok {
		t.Fatalf("ports[5] is not a long-form map: %#v", ports[5])
	}

	if _, present := long["published"]; present {
		t.Errorf("long-form published not stripped in worktree mode: %#v", long)
	}

	if long["target"] != 3005 {
		t.Errorf("long-form target changed: %#v", long)
	}
}

func TestExtraServices_PortsKeptOutsideWorktree(t *testing.T) {
	g := newTestGenerator(t)
	cfg := extraCfg(map[string]map[string]any{
		"gotenberg": {
			"image": "gotenberg/gotenberg:8",
			"ports": []interface{}{"3000:3000"},
		},
	})

	out, err := g.Generate(cfg, "myapp", "myapp", false, 5173)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	ports, _ := composeService(t, out, "gotenberg")["ports"].([]interface{})
	if len(ports) != 1 || ports[0] != "3000:3000" {
		t.Errorf("host mapping must survive outside worktree mode: %#v", ports)
	}
}

func TestExtraServices_NamedVolumesDeclared(t *testing.T) {
	g := newTestGenerator(t)
	cfg := extraCfg(map[string]map[string]any{
		"minio": {
			"image": "minio/minio",
			"volumes": []interface{}{
				"minio_data:/data",
				"./local:/app",          // bind mount — not a volume
				"/abs/path:/mnt",        // bind mount
				"~/home/path:/home",     // bind mount
				"${HOST_DIR}:/var/host", // env expansion, not a volume
				map[string]interface{}{"type": "volume", "source": "minio_conf", "target": "/conf"},
				map[string]interface{}{"type": "bind", "source": "./cfg", "target": "/cfg"},
			},
		},
	})

	out, err := g.Generate(cfg, "myapp", "myapp", false, 5173)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	vols, ok := parseCompose(t, out)["volumes"].(map[string]interface{})
	if !ok {
		t.Fatalf("no top-level volumes map:\n%s", out)
	}

	for _, want := range []string{"minio_data", "minio_conf"} {
		decl, ok := vols[want].(map[string]interface{})
		if !ok {
			t.Fatalf("named volume %q not declared: %#v", want, vols)
		}

		if decl["driver"] != "local" {
			t.Errorf("volume %q driver = %v, want local", want, decl["driver"])
		}
	}

	for name := range vols {
		switch name {
		case "minio_data", "minio_conf", "pgsql_data":
		default:
			t.Errorf("unexpected volume declared: %q", name)
		}
	}
}

func TestExtraServices_DotEnvStripped(t *testing.T) {
	g := newTestGenerator(t)
	cfg := extraCfg(map[string]map[string]any{
		"gotenberg": {
			"image":   "gotenberg/gotenberg:8",
			"dot_env": map[string]interface{}{"GOTENBERG_URL": "http://gotenberg:3000"},
		},
	})

	out, err := g.Generate(cfg, "myapp", "myapp", false, 5173)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	if _, present := composeService(t, out, "gotenberg")["dot_env"]; present {
		t.Errorf("dot_env must not reach compose.yaml:\n%s", out)
	}

	if strings.Contains(out, "dot_env") {
		t.Errorf("dot_env leaked into output:\n%s", out)
	}
}

func TestExtraServices_NetworksDefaultedAndPreserved(t *testing.T) {
	g := newTestGenerator(t)
	cfg := extraCfg(map[string]map[string]any{
		"plain":    {"image": "alpine"},
		"explicit": {"image": "alpine", "networks": []interface{}{"other"}},
	})

	out, err := g.Generate(cfg, "myapp", "myapp", false, 5173)
	if err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	plain, _ := composeService(t, out, "plain")["networks"].([]interface{})
	if len(plain) != 1 || plain[0] != "frank" {
		t.Errorf("networks not defaulted to [frank]: %#v", plain)
	}

	explicit, _ := composeService(t, out, "explicit")["networks"].([]interface{})
	if len(explicit) != 1 || explicit[0] != "other" {
		t.Errorf("explicit networks must pass through untouched: %#v", explicit)
	}
}

// Generate() runs twice per invocation against the same *config.Config, so the
// block must be deep-copied before any mutation — otherwise the second pass
// sees a block already stripped of dot_env and rewritten ports.
func TestExtraServices_GenerateTwiceIsStable(t *testing.T) {
	g := newTestGenerator(t)
	cfg := extraCfg(map[string]map[string]any{
		"gotenberg": {
			"image":   "gotenberg/gotenberg:8",
			"ports":   []interface{}{"3000:3000"},
			"dot_env": map[string]interface{}{"GOTENBERG_URL": "http://gotenberg:3000"},
			"volumes": []interface{}{"gotenberg_data:/data"},
		},
	})

	first, err := g.Generate(cfg, "wt", "main", true, 5187)
	if err != nil {
		t.Fatalf("first Generate error: %v", err)
	}

	second, err := g.Generate(cfg, "wt", "main", true, 5187)
	if err != nil {
		t.Fatalf("second Generate error: %v", err)
	}

	if first != second {
		t.Errorf("output differs between passes — cfg was mutated:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}

	block := cfg.ExtraServices["gotenberg"]
	if _, present := block["dot_env"]; !present {
		t.Error("dot_env deleted from the caller's config")
	}

	ports, _ := block["ports"].([]interface{})
	if len(ports) != 1 || ports[0] != "3000:3000" {
		t.Errorf("caller's ports were rewritten in place: %#v", ports)
	}
}
