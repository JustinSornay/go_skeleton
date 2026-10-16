//go:build integration

package postgres_test

import (
	"os"
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/platform/database/dbtest"
	"github.com/JustinSornay/go_skeleton/internal/project"
	"github.com/JustinSornay/go_skeleton/internal/project/postgres"
	"github.com/JustinSornay/go_skeleton/internal/project/projecttest"
)

func TestMain(m *testing.M) { os.Exit(dbtest.Main(m)) }

func TestPostgresRepositoryContract(t *testing.T) {
	projecttest.RepositoryContract(t, func(t *testing.T) project.Repository {
		return postgres.NewRepository(dbtest.New(t))
	})
}
