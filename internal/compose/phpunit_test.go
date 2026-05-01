package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPatchPHPUnitXML_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	if err := PatchPHPUnitXML(dir, "mysql"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "phpunit.xml")); !os.IsNotExist(err) {
		t.Fatal("phpunit.xml should not be created")
	}
}

func TestPatchPHPUnitXML_SqliteSkip(t *testing.T) {
	dir := t.TempDir()
	content := `<phpunit>
    <php>
        <env name="DB_CONNECTION" value="sqlite"/>
    </php>
</phpunit>`
	path := filepath.Join(dir, "phpunit.xml")
	os.WriteFile(path, []byte(content), 0644)

	if err := PatchPHPUnitXML(dir, "sqlite"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != content {
		t.Fatal("file should not be modified for sqlite")
	}
}

func TestPatchPHPUnitXML_EmptyStringSkip(t *testing.T) {
	dir := t.TempDir()
	content := `<phpunit>
    <php>
        <env name="DB_CONNECTION" value="sqlite"/>
    </php>
</phpunit>`
	path := filepath.Join(dir, "phpunit.xml")
	os.WriteFile(path, []byte(content), 0644)

	if err := PatchPHPUnitXML(dir, ""); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != content {
		t.Fatal("file should not be modified for empty string")
	}
}

func TestPatchPHPUnitXML_NormalPatch(t *testing.T) {
	dir := t.TempDir()
	content := `<?xml version="1.0" encoding="UTF-8"?>
<phpunit>
    <php>
        <env name="APP_ENV" value="testing"/>
        <env name="DB_CONNECTION" value="sqlite"/>
        <env name="DB_DATABASE" value=":memory:"/>
    </php>
</phpunit>`
	os.WriteFile(filepath.Join(dir, "phpunit.xml"), []byte(content), 0644)

	if err := PatchPHPUnitXML(dir, "mysql"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "phpunit.xml"))
	s := string(got)
	requireContains(t, s, `<env name="DB_CONNECTION" value="mysql" force="true"/>`)
	requireContains(t, s, `<env name="DB_DATABASE" value="testing" force="true"/>`)
	requireContains(t, s, `<server name="DB_CONNECTION" value="mysql"/>`)
	requireContains(t, s, `<server name="DB_DATABASE" value="testing"/>`)
	requireContains(t, s, `name="APP_ENV" value="testing"/>`)
}

func TestPatchPHPUnitXML_MissingEnvLines(t *testing.T) {
	dir := t.TempDir()
	content := `<?xml version="1.0" encoding="UTF-8"?>
<phpunit>
    <php>
        <env name="APP_ENV" value="testing"/>
    </php>
</phpunit>`
	os.WriteFile(filepath.Join(dir, "phpunit.xml"), []byte(content), 0644)

	if err := PatchPHPUnitXML(dir, "pgsql"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "phpunit.xml"))
	s := string(got)
	requireContains(t, s, `<env name="DB_CONNECTION" value="pgsql" force="true"/>`)
	requireContains(t, s, `<env name="DB_DATABASE" value="testing" force="true"/>`)
	requireContains(t, s, `<server name="DB_CONNECTION" value="pgsql"/>`)
	requireContains(t, s, `<server name="DB_DATABASE" value="testing"/>`)

	// All inserted before </php>.
	phpIdx := strings.Index(s, "</php>")
	for _, needle := range []string{`<env name="DB_CONNECTION"`, `<server name="DB_DATABASE"`} {
		if idx := strings.Index(s, needle); idx > phpIdx {
			t.Errorf("%s should appear before </php>:\n%s", needle, s)
		}
	}
}

func TestPatchPHPUnitXML_ForceAttribute(t *testing.T) {
	dir := t.TempDir()
	content := `<phpunit>
    <php>
        <env name="DB_CONNECTION" value="sqlite" force="true"/>
        <env name="DB_DATABASE" value=":memory:" force="true"/>
    </php>
</phpunit>`
	os.WriteFile(filepath.Join(dir, "phpunit.xml"), []byte(content), 0644)

	if err := PatchPHPUnitXML(dir, "mariadb"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "phpunit.xml"))
	s := string(got)
	requireContains(t, s, `<env name="DB_CONNECTION" value="mariadb" force="true"/>`)
	requireContains(t, s, `<env name="DB_DATABASE" value="testing" force="true"/>`)
	requireContains(t, s, `<server name="DB_CONNECTION" value="mariadb"/>`)
	requireContains(t, s, `<server name="DB_DATABASE" value="testing"/>`)
}

func TestPatchPHPUnitXML_ExistingServerEntries(t *testing.T) {
	dir := t.TempDir()
	content := `<phpunit>
    <php>
        <env name="DB_CONNECTION" value="sqlite" force="true"/>
        <env name="DB_DATABASE" value=":memory:" force="true"/>
        <server name="DB_CONNECTION" value="sqlite"/>
        <server name="DB_DATABASE" value=":memory:"/>
    </php>
</phpunit>`
	os.WriteFile(filepath.Join(dir, "phpunit.xml"), []byte(content), 0644)

	if err := PatchPHPUnitXML(dir, "pgsql"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "phpunit.xml"))
	s := string(got)
	requireContains(t, s, `<env name="DB_CONNECTION" value="pgsql" force="true"/>`)
	requireContains(t, s, `<env name="DB_DATABASE" value="testing" force="true"/>`)
	requireContains(t, s, `<server name="DB_CONNECTION" value="pgsql"/>`)
	requireContains(t, s, `<server name="DB_DATABASE" value="testing"/>`)
	// No duplicates — count occurrences.
	if c := strings.Count(s, `name="DB_CONNECTION"`); c != 2 {
		t.Errorf("expected exactly 2 DB_CONNECTION entries (env+server), got %d:\n%s", c, s)
	}
}

func TestPatchPHPUnitXML_FullLaravelDefault(t *testing.T) {
	dir := t.TempDir()
	content := `<?xml version="1.0" encoding="UTF-8"?>
<phpunit xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:noNamespaceSchemaLocation="vendor/phpunit/phpunit/phpunit.xsd"
         bootstrap="vendor/autoload.php"
         colors="true"
>
    <testsuites>
        <testsuite name="Unit">
            <directory>tests/Unit</directory>
        </testsuite>
        <testsuite name="Feature">
            <directory>tests/Feature</directory>
        </testsuite>
    </testsuites>
    <source>
        <include>
            <directory>app</directory>
        </include>
    </source>
    <php>
        <env name="APP_ENV" value="testing"/>
        <env name="APP_MAINTENANCE_DRIVER" value="file"/>
        <env name="BCRYPT_ROUNDS" value="4"/>
        <env name="CACHE_STORE" value="array"/>
        <env name="DB_CONNECTION" value="sqlite"/>
        <env name="DB_DATABASE" value=":memory:"/>
        <env name="MAIL_MAILER" value="array"/>
        <env name="PULSE_ENABLED" value="false"/>
        <env name="QUEUE_CONNECTION" value="sync"/>
        <env name="SESSION_DRIVER" value="array"/>
        <env name="TELESCOPE_ENABLED" value="false"/>
    </php>
</phpunit>
`
	os.WriteFile(filepath.Join(dir, "phpunit.xml"), []byte(content), 0644)

	if err := PatchPHPUnitXML(dir, "pgsql"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "phpunit.xml"))
	s := string(got)
	requireContains(t, s, `<env name="DB_CONNECTION" value="pgsql" force="true"/>`)
	requireContains(t, s, `<env name="DB_DATABASE" value="testing" force="true"/>`)
	requireContains(t, s, `<server name="DB_CONNECTION" value="pgsql"/>`)
	requireContains(t, s, `<server name="DB_DATABASE" value="testing"/>`)
	requireContains(t, s, `name="CACHE_STORE" value="array"/>`)
	requireContains(t, s, "<testsuites>")
}

func TestPatchPHPUnitXML_CommentedOutLines(t *testing.T) {
	dir := t.TempDir()
	content := `<?xml version="1.0" encoding="UTF-8"?>
<phpunit>
    <php>
        <env name="APP_ENV" value="testing"/>
        <env name="BCRYPT_ROUNDS" value="4"/>
        <env name="CACHE_STORE" value="array"/>
        <!-- <env name="DB_CONNECTION" value="sqlite"/> -->
        <!-- <env name="DB_DATABASE" value=":memory:"/> -->
        <env name="MAIL_MAILER" value="array"/>
    </php>
</phpunit>`
	os.WriteFile(filepath.Join(dir, "phpunit.xml"), []byte(content), 0644)

	if err := PatchPHPUnitXML(dir, "mariadb"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "phpunit.xml"))
	s := string(got)
	requireContains(t, s, `<env name="DB_CONNECTION" value="mariadb" force="true"/>`)
	requireContains(t, s, `<env name="DB_DATABASE" value="testing" force="true"/>`)
	requireContains(t, s, `<server name="DB_CONNECTION" value="mariadb"/>`)
	requireContains(t, s, `<server name="DB_DATABASE" value="testing"/>`)
	requireContains(t, s, `<!-- <env name="DB_CONNECTION"`)
}

func TestRestorePHPUnitXML(t *testing.T) {
	dir := t.TempDir()
	content := `<?xml version="1.0" encoding="UTF-8"?>
<phpunit>
    <php>
        <env name="APP_ENV" value="testing"/>
        <env name="DB_CONNECTION" value="pgsql" force="true"/>
        <env name="DB_DATABASE" value="testing" force="true"/>
        <server name="DB_CONNECTION" value="pgsql"/>
        <server name="DB_DATABASE" value="testing"/>
    </php>
</phpunit>`
	os.WriteFile(filepath.Join(dir, "phpunit.xml"), []byte(content), 0644)

	if err := RestorePHPUnitXML(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "phpunit.xml"))
	s := string(got)
	requireContains(t, s, `<env name="DB_CONNECTION" value="sqlite" force="true"/>`)
	requireContains(t, s, `<env name="DB_DATABASE" value=":memory:" force="true"/>`)
	requireContains(t, s, `<server name="DB_CONNECTION" value="sqlite"/>`)
	requireContains(t, s, `<server name="DB_DATABASE" value=":memory:"/>`)
	requireContains(t, s, `name="APP_ENV" value="testing"`)
}

func TestRestorePHPUnitXML_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	if err := RestorePHPUnitXML(dir); err != nil {
		t.Fatalf("expected nil for missing file, got %v", err)
	}
}

func requireContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected to find %q in:\n%s", needle, haystack)
	}
}
