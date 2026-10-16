package projecttest_test

import (
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/project"
	"github.com/JustinSornay/go_skeleton/internal/project/projecttest"
)

func TestMemoryRepositoryContract(t *testing.T) {
	projecttest.RepositoryContract(t, func(*testing.T) project.Repository { return projecttest.NewRepository() })
}
