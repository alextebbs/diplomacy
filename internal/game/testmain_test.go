package game

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
)

func TestMain(m *testing.M) {
	if os.Getenv("TEST_DATABASE_URL") != "" {
		os.Exit(m.Run())
	}

	runtimeDir := filepath.Join(os.TempDir(), "ep-game-test")

	pg := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Port(15432).
			Database("diplomacy_test").
			RuntimePath(runtimeDir),
	)
	if err := pg.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start embedded postgres: %v\n", err)
		os.Exit(1)
	}

	os.Setenv("TEST_DATABASE_URL", fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/diplomacy_test?sslmode=disable", 15432,
	))

	code := m.Run()
	pg.Stop()
	os.RemoveAll(runtimeDir)
	os.Exit(code)
}
