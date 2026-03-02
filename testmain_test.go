package diplomacy_test

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

	runtimeDir := filepath.Join(os.TempDir(), "ep-integ-test")

	pg := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Port(15434).
			Database("diplomacy_integ_test").
			RuntimePath(runtimeDir),
	)
	if err := pg.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start embedded postgres: %v\n", err)
		os.Exit(1)
	}

	os.Setenv("TEST_DATABASE_URL", fmt.Sprintf(
		"postgres://postgres:postgres@localhost:%d/diplomacy_integ_test?sslmode=disable", 15434,
	))

	code := m.Run()
	pg.Stop()
	os.RemoveAll(runtimeDir)
	os.Exit(code)
}
