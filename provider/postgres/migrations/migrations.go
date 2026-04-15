package migrations

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"
)

type Migration struct {
	Version int
	Name    string
	SQL     string
}

//go:embed *.sql
var migrationFS embed.FS

var (
	migrationsOnce sync.Once
	migrations     []Migration
	migrationsErr  error
)

func All() ([]Migration, error) {
	migrationsOnce.Do(func() {
		entries, err := fs.Glob(migrationFS, "*.sql")
		if err != nil {
			migrationsErr = fmt.Errorf("postgres migrations: failed to list embedded migrations: %w", err)
			return
		}

		loaded := make([]Migration, 0, len(entries))
		seenVersions := make(map[int]string, len(entries))
		for _, name := range entries {
			body, err := migrationFS.ReadFile(name)
			if err != nil {
				migrationsErr = fmt.Errorf("postgres migrations: failed to read embedded migration %s: %w", name, err)
				return
			}

			version, err := parseVersion(filepath.Base(name))
			if err != nil {
				migrationsErr = err
				return
			}
			if prevName, exists := seenVersions[version]; exists {
				migrationsErr = fmt.Errorf("postgres migrations: duplicated migration version %d in %s and %s", version, prevName, name)
				return
			}
			seenVersions[version] = name

			loaded = append(loaded, Migration{
				Version: version,
				Name:    filepath.Base(name),
				SQL:     string(body),
			})
		}

		sort.Slice(loaded, func(i, j int) bool {
			return loaded[i].Version < loaded[j].Version
		})
		migrations = loaded
	})

	if migrationsErr != nil {
		return nil, migrationsErr
	}

	result := make([]Migration, len(migrations))
	copy(result, migrations)
	return result, nil
}

func LatestVersion() (int, error) {
	migrations, err := All()
	if err != nil {
		return 0, err
	}
	if len(migrations) == 0 {
		return 0, nil
	}
	return migrations[len(migrations)-1].Version, nil
}

func parseVersion(name string) (int, error) {
	re := regexp.MustCompile(`^(\d+)`)
	matches := re.FindStringSubmatch(name)
	if len(matches) != 2 {
		return 0, fmt.Errorf("postgres migrations: migration file %s does not start with a numeric version", name)
	}

	version, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("postgres migrations: invalid migration version in %s: %w", name, err)
	}
	return version, nil
}
