package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// phpunitVarRe matches <env name="..." value="..."/> and <server name="..." value="..."/> lines.
var phpunitVarRe = regexp.MustCompile(`^(\s*<(?:env|server)\s+name=")([^"]+)("\s+value=")([^"]*)(".*)$`)

// PatchPHPUnitXML rewrites phpunit.xml so the testing database uses the given
// connection (e.g. "mysql", "pgsql") with database name "testing".
// For sqlite (or empty string) no patching is needed — returns nil.
// If phpunit.xml does not exist the call is a no-op.
//
// Sets both <env> and <server> entries because Docker injects DB_* into
// $_SERVER, and Laravel's immutable Env repository reads $_SERVER first.
// Without <server>, phpunit.xml <env> values are silently ignored.
func PatchPHPUnitXML(dir string, dbConnection string) error {
	if dbConnection == "" || dbConnection == "sqlite" {
		return nil
	}
	return patchPHPUnit(dir, dbConnection, "testing")
}

// RestorePHPUnitXML resets phpunit.xml to Laravel defaults:
// DB_CONNECTION → sqlite, DB_DATABASE → :memory:.
// If phpunit.xml does not exist the call is a no-op.
func RestorePHPUnitXML(dir string) error {
	return patchPHPUnit(dir, "sqlite", ":memory:")
}

// patchPHPUnit is the shared implementation for PatchPHPUnitXML and
// RestorePHPUnitXML. It patches both <env> and <server> entries for
// DB_CONNECTION and DB_DATABASE, inserting missing lines before </php>.
func patchPHPUnit(dir string, dbConnection string, dbDatabase string) error {
	path := filepath.Join(dir, "phpunit.xml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read phpunit.xml: %w", err)
	}

	lines := strings.Split(string(data), "\n")

	// Track which combinations we've found: env+server × conn+db.
	foundEnvConn := false
	foundEnvDB := false
	foundServerConn := false
	foundServerDB := false
	phpCloseIdx := -1

	for i, line := range lines {
		if m := phpunitVarRe.FindStringSubmatch(line); m != nil {
			tag := extractTag(line)
			switch {
			case m[2] == "DB_CONNECTION" && tag == "env":
				lines[i] = m[1] + m[2] + m[3] + dbConnection + ensureForce(m[5])
				foundEnvConn = true
			case m[2] == "DB_DATABASE" && tag == "env":
				lines[i] = m[1] + m[2] + m[3] + dbDatabase + ensureForce(m[5])
				foundEnvDB = true
			case m[2] == "DB_CONNECTION" && tag == "server":
				lines[i] = m[1] + m[2] + m[3] + dbConnection + m[5]
				foundServerConn = true
			case m[2] == "DB_DATABASE" && tag == "server":
				lines[i] = m[1] + m[2] + m[3] + dbDatabase + m[5]
				foundServerDB = true
			}
		}
		if strings.TrimSpace(line) == "</php>" {
			phpCloseIdx = i
		}
	}

	if phpCloseIdx < 0 {
		return nil
	}

	// Build missing lines.
	indent := "        "
	if idx := strings.Index(lines[phpCloseIdx], "</php>"); idx > 0 {
		indent = lines[phpCloseIdx][:idx] + "    "
	}

	var inserts []string
	if !foundEnvConn {
		inserts = append(inserts, fmt.Sprintf(`%s<env name="DB_CONNECTION" value="%s" force="true"/>`, indent, dbConnection))
	}
	if !foundEnvDB {
		inserts = append(inserts, fmt.Sprintf(`%s<env name="DB_DATABASE" value="%s" force="true"/>`, indent, dbDatabase))
	}
	if !foundServerConn {
		inserts = append(inserts, fmt.Sprintf(`%s<server name="DB_CONNECTION" value="%s"/>`, indent, dbConnection))
	}
	if !foundServerDB {
		inserts = append(inserts, fmt.Sprintf(`%s<server name="DB_DATABASE" value="%s"/>`, indent, dbDatabase))
	}

	if len(inserts) > 0 {
		tail := make([]string, len(lines[phpCloseIdx:]))
		copy(tail, lines[phpCloseIdx:])
		lines = append(lines[:phpCloseIdx], inserts...)
		lines = append(lines, tail...)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// extractTag returns "env" or "server" from a line like `<env name=...` or `<server name=...`.
func extractTag(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "<server") {
		return "server"
	}
	return "env"
}

// ensureForce guarantees the trailing portion of an <env/> line contains
// force="true" so phpunit.xml values override .env.
func ensureForce(suffix string) string {
	if strings.Contains(suffix, `force="true"`) {
		return suffix
	}
	return strings.Replace(suffix, `"/>`, `" force="true"/>`, 1)
}
