package repositorytest_test

import (
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/model"
	"github.com/JustinSornay/go_skeleton/internal/repository/repositorytest"
)

func TestMemoryProjectRepositoryContract(t *testing.T) {
	repositorytest.ProjectRepositoryContract(t, func(*testing.T) model.ProjectRepository {
		return repositorytest.NewProjectRepository()
	})
}
