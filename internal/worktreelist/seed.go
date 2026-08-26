package worktreelist

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"

	"github.com/phlisg/frank/internal/config"
	"github.com/phlisg/frank/internal/docker"
)

// seedMarker lives under .frank/ (already gitignored) and holds the absolute path
// of the main checkout to clone the database from. Written at worktree creation,
// consumed and deleted by the first successful `up`.
const seedMarker = ".frank/.seed-db"

func markerPath(wtPath string) string {
	return filepath.Join(wtPath, filepath.FromSlash(seedMarker))
}

// markSeedDB records that this worktree wants mainDir's database cloned on first up.
func markSeedDB(wtPath, mainDir string) error {
	if err := os.MkdirAll(filepath.Join(wtPath, ".frank"), 0755); err != nil {
		return err
	}

	return os.WriteFile(markerPath(wtPath), []byte(mainDir+"\n"), 0644)
}

// SeedPending reports whether wtPath is still waiting for its database clone.
func SeedPending(wtPath string) bool {
	_, err := os.Stat(markerPath(wtPath))
	return err == nil
}

// CloneDB clones mainDir's database into an existing worktree on demand, for
// worktrees created before the seed-on-up flow existed. It goes through the same
// marker as the automatic path, so a clone that fails half-way is retried by the
// next `up` rather than leaving the worktree silently half-populated.
func CloneDB(wtPath, mainDir string, w io.Writer) error {
	if err := markSeedDB(wtPath, mainDir); err != nil {
		return err
	}

	return seedDB(wtPath, w)
}

// seedDB clones the main checkout's database into the worktree, then runs
// migrations so the branch's own migrations land on top. The marker is cleared
// only on success, so a failed clone is retried by the next `up`.
func seedDB(wtPath string, w io.Writer) error {
	raw, err := os.ReadFile(markerPath(wtPath))
	if err != nil {
		return nil // nothing pending
	}

	mainDir := strings.TrimSpace(string(raw))
	if mainDir == "" || mainDir == wtPath {
		return os.Remove(markerPath(wtPath))
	}

	cfg, err := config.Load(wtPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	db := cfg.Database()
	if db == "" {
		return os.Remove(markerPath(wtPath))
	}

	fmt.Fprintf(w, "cloning %s database from %s\n", db, filepath.Base(mainDir))

	warnAppKeyMismatch(mainDir, wtPath, w)

	if db == "sqlite" {
		if err := copySQLite(mainDir, wtPath); err != nil {
			return err
		}
	} else if err := pipeDump(mainDir, wtPath, db, w); err != nil {
		return err
	}

	if cfg.HasService("meilisearch") {
		// The index lives in meilisearch's own volume, not in the database, so a
		// cloned database leaves it empty. Rebuilding it is the app's job — Scout
		// has no "import everything" command and the searchable model classes are
		// not knowable from frank.yaml.
		fmt.Fprintln(w, "note: meilisearch index is empty — run `php artisan scout:import <Model>`")
	}

	fmt.Fprintln(w, "applying migrations")

	if err := runFrank(w, "exec", "--dir", wtPath, "php", "artisan", "migrate", "--force"); err != nil {
		return err
	}

	return os.Remove(markerPath(wtPath))
}

// warnAppKeyMismatch flags a differing APP_KEY: Laravel's encrypted casts and
// signed URLs are keyed off it, so cloned rows decrypt to garbage without it.
// A warning rather than a fix — overwriting APP_KEY invalidates whatever the
// worktree already encrypted under its own key.
func warnAppKeyMismatch(mainDir, wtPath string, w io.Writer) {
	src, err1 := godotenv.Read(filepath.Join(mainDir, ".env"))
	dst, err2 := godotenv.Read(filepath.Join(wtPath, ".env"))

	if err1 != nil || err2 != nil || src["APP_KEY"] == dst["APP_KEY"] {
		return
	}

	fmt.Fprintf(w, "warning: APP_KEY differs from the main project — encrypted columns in the cloned data will not decrypt. Copy APP_KEY from %s/.env to fix.\n", filepath.Base(mainDir))
}

func copySQLite(mainDir, wtPath string) error {
	src := filepath.Join(mainDir, "database", "database.sqlite")

	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read main database.sqlite: %w", err)
	}

	dst := filepath.Join(wtPath, "database", "database.sqlite")
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	return os.WriteFile(dst, data, 0644)
}

// pipeDump streams a dump out of the main project's database container straight
// into the worktree's. Streaming rather than buffering keeps a multi-GB database
// off the heap.
func pipeDump(mainDir, wtPath, db string, w io.Writer) error {
	srcEnv, err := godotenv.Read(filepath.Join(mainDir, ".env"))
	if err != nil {
		return fmt.Errorf("read main .env: %w", err)
	}

	dstEnv, err := godotenv.Read(filepath.Join(wtPath, ".env"))
	if err != nil {
		return fmt.Errorf("read worktree .env: %w", err)
	}

	dump, restore := dumpCommands(db, srcEnv, dstEnv)

	pr, pw := io.Pipe()

	errCh := make(chan error, 1)

	go func() {
		err := docker.New(mainDir).ExecPipe(nil, pw, w, db, dump...)
		if err != nil {
			err = fmt.Errorf("dump from main project (is it running? `frank up -d`): %w", err)
		}

		_ = pw.CloseWithError(err)
		errCh <- err
	}()

	if err := docker.New(wtPath).ExecPipe(pr, io.Discard, w, db, restore...); err != nil {
		<-errCh
		return fmt.Errorf("restore into worktree: %w", err)
	}

	return <-errCh
}

// dumpCommands returns the argv for dumping the source database and for loading
// it into the destination. Credentials go through env vars rather than flags so
// they never show up in the container's process list.
//
// MySQL/MariaDB go through root, not DB_USERNAME: the app user cannot read the
// mysql.* system tables mysqldump touches, and lacks the SUPER/SYSTEM_VARIABLES_ADMIN
// the dump's session-variable statements need on restore. Both images set root's
// password to DB_PASSWORD, so no extra config is needed.
func dumpCommands(db string, src, dst map[string]string) (dump, restore []string) {
	switch db {
	case "pgsql":
		// The app user owns the database, so no superuser escalation is needed here.
		return []string{
			"env", "PGPASSWORD=" + src["DB_PASSWORD"],
			"pg_dump", "--clean", "--if-exists",
			"-U", src["DB_USERNAME"], "-d", src["DB_DATABASE"],
		}, []string{
			"env", "PGPASSWORD=" + dst["DB_PASSWORD"],
			"psql", "-q", "-U", dst["DB_USERNAME"], "-d", dst["DB_DATABASE"],
		}

	case "mariadb":
		// MariaDB dropped the mysql* client names; mariadb-dump has shipped since 10.11.
		return []string{
			"env", "MYSQL_PWD=" + src["DB_PASSWORD"],
			"mariadb-dump", "--no-tablespaces", "--single-transaction",
			"-u", "root", src["DB_DATABASE"],
		}, []string{
			"env", "MYSQL_PWD=" + dst["DB_PASSWORD"],
			"mariadb", "-u", "root", dst["DB_DATABASE"],
		}

	default:
		// --set-gtid-purged=OFF drops the GTID_PURGED statement, which is meaningless
		// on a different server and is what triggers ERROR 1227 on restore.
		return []string{
			"env", "MYSQL_PWD=" + src["DB_PASSWORD"],
			"mysqldump", "--no-tablespaces", "--single-transaction",
			"--set-gtid-purged=OFF",
			"-u", "root", src["DB_DATABASE"],
		}, []string{
			"env", "MYSQL_PWD=" + dst["DB_PASSWORD"],
			"mysql", "-u", "root", dst["DB_DATABASE"],
		}
	}
}
