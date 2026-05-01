package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// envNameRe matches <env name="..." value="..."/> lines in phpunit.xml.
var envNameRe = regexp.MustCompile(`^(\s*<env\s+name=")([^"]+)("\s+value=")([^"]*)(".*)$`)

// PatchPHPUnitXML rewrites phpunit.xml so the testing database uses the given
// connection (e.g. "mysql", "pgsql") with database name "testing".
// For sqlite (or empty string) no patching is needed — returns nil.
// If phpunit.xml does not exist the call is a no-op.
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
// RestorePHPUnitXML. It sets DB_CONNECTION to dbConnection and DB_DATABASE
// to dbDatabase, inserting env lines before </php> if they are missing.
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

	foundConn := false
	foundDB := false
	phpCloseIdx := -1 // index of the </php> line

	for i, line := range lines {
		if m := envNameRe.FindStringSubmatch(line); m != nil {
			switch m[2] {
			case "DB_CONNECTION":
				lines[i] = m[1] + m[2] + m[3] + dbConnection + ensureForce(m[5])
				foundConn = true
			case "DB_DATABASE":
				lines[i] = m[1] + m[2] + m[3] + dbDatabase + ensureForce(m[5])
				foundDB = true
			}
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "</php>" {
			phpCloseIdx = i
		}
	}

	// Insert missing env lines before </php>.
	if phpCloseIdx >= 0 && (!foundConn || !foundDB) {
		// Detect indentation from the </php> line or use 8 spaces.
		indent := "        "
		if idx := strings.Index(lines[phpCloseIdx], "</php>"); idx > 0 {
			indent = lines[phpCloseIdx][:idx] + "    "
		}

		var inserts []string
		if !foundConn {
			inserts = append(inserts, fmt.Sprintf(`%s<env name="DB_CONNECTION" value="%s" force="true"/>`, indent, dbConnection))
		}
		if !foundDB {
			inserts = append(inserts, fmt.Sprintf(`%s<env name="DB_DATABASE" value="%s" force="true"/>`, indent, dbDatabase))
		}

		// Splice inserts before phpCloseIdx.
		tail := make([]string, len(lines[phpCloseIdx:]))
		copy(tail, lines[phpCloseIdx:])
		lines = append(lines[:phpCloseIdx], inserts...)
		lines = append(lines, tail...)
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0644)
}

// ensureForce guarantees the trailing portion of an <env/> line contains
// force="true" so phpunit.xml values override .env.
func ensureForce(suffix string) string {
	if strings.Contains(suffix, `force="true"`) {
		return suffix
	}
	return strings.Replace(suffix, `"/>`, `" force="true"/>`, 1)
}
