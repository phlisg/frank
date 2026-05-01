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
	// No file should be created.
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
	if !strings.Contains(s, `name="DB_CONNECTION" value="mysql" force="true"`) {
		t.Errorf("DB_CONNECTION not patched with force:\n%s", s)
	}
	if !strings.Contains(s, `name="DB_DATABASE" value="testing" force="true"`) {
		t.Errorf("DB_DATABASE not patched with force:\n%s", s)
	}
	// APP_ENV should be untouched (no force added).
	if !strings.Contains(s, `name="APP_ENV" value="testing"/>`) {
		t.Errorf("APP_ENV should not be modified:\n%s", s)
	}
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
	if !strings.Contains(s, `name="DB_CONNECTION" value="pgsql" force="true"`) {
		t.Errorf("DB_CONNECTION not inserted with force:\n%s", s)
	}
	if !strings.Contains(s, `name="DB_DATABASE" value="testing" force="true"`) {
		t.Errorf("DB_DATABASE not inserted with force:\n%s", s)
	}
	// Inserted before </php>.
	phpIdx := strings.Index(s, "</php>")
	connIdx := strings.Index(s, `name="DB_CONNECTION"`)
	dbIdx := strings.Index(s, `name="DB_DATABASE"`)
	if connIdx > phpIdx || dbIdx > phpIdx {
		t.Errorf("env lines should appear before </php>:\n%s", s)
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
	if !strings.Contains(s, `value="mariadb" force="true"`) {
		t.Errorf("DB_CONNECTION not patched or force lost:\n%s", s)
	}
	if !strings.Contains(s, `value="testing" force="true"`) {
		t.Errorf("DB_DATABASE not patched or force lost:\n%s", s)
	}
}

func TestPatchPHPUnitXML_FullLaravelDefault(t *testing.T) {
	dir := t.TempDir()
	// Realistic Laravel 11+ default phpunit.xml.
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
	if !strings.Contains(s, `name="DB_CONNECTION" value="pgsql" force="true"`) {
		t.Errorf("DB_CONNECTION not patched with force:\n%s", s)
	}
	if !strings.Contains(s, `name="DB_DATABASE" value="testing" force="true"`) {
		t.Errorf("DB_DATABASE not patched with force:\n%s", s)
	}
	// Other env vars untouched (no force added).
	if !strings.Contains(s, `name="CACHE_STORE" value="array"/>`) {
		t.Errorf("CACHE_STORE should be untouched:\n%s", s)
	}
	// XML structure preserved.
	if !strings.Contains(s, "<testsuites>") {
		t.Errorf("testsuites section missing:\n%s", s)
	}
}

func TestPatchPHPUnitXML_CommentedOutLines(t *testing.T) {
	dir := t.TempDir()
	// Laravel 11+ default: DB lines are commented out.
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
	if !strings.Contains(s, `name="DB_CONNECTION" value="mariadb" force="true"`) {
		t.Errorf("DB_CONNECTION not inserted with force:\n%s", s)
	}
	if !strings.Contains(s, `name="DB_DATABASE" value="testing" force="true"`) {
		t.Errorf("DB_DATABASE not inserted with force:\n%s", s)
	}
	// Commented lines should still be present (untouched).
	if !strings.Contains(s, `<!-- <env name="DB_CONNECTION"`) {
		t.Errorf("original commented line should remain:\n%s", s)
	}
}

func TestRestorePHPUnitXML(t *testing.T) {
	dir := t.TempDir()
	// phpunit.xml patched for pgsql/testing — restore should revert to sqlite/:memory:.
	content := `<?xml version="1.0" encoding="UTF-8"?>
<phpunit>
    <php>
        <env name="APP_ENV" value="testing"/>
        <env name="DB_CONNECTION" value="pgsql"/>
        <env name="DB_DATABASE" value="testing"/>
    </php>
</phpunit>`
	os.WriteFile(filepath.Join(dir, "phpunit.xml"), []byte(content), 0644)

	if err := RestorePHPUnitXML(dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "phpunit.xml"))
	s := string(got)
	if !strings.Contains(s, `name="DB_CONNECTION" value="sqlite" force="true"`) {
		t.Errorf("DB_CONNECTION not restored to sqlite with force:\n%s", s)
	}
	if !strings.Contains(s, `name="DB_DATABASE" value=":memory:" force="true"`) {
		t.Errorf("DB_DATABASE not restored to :memory: with force:\n%s", s)
	}
	// APP_ENV should be untouched.
	if !strings.Contains(s, `name="APP_ENV" value="testing"`) {
		t.Errorf("APP_ENV should not be modified:\n%s", s)
	}
}

func TestRestorePHPUnitXML_FileNotExist(t *testing.T) {
	dir := t.TempDir()
	if err := RestorePHPUnitXML(dir); err != nil {
		t.Fatalf("expected nil for missing file, got %v", err)
	}
}
