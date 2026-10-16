//go:build integration

package repository_test

import (
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/database/dbtest"
	"github.com/JustinSornay/go_skeleton/internal/model"
	"github.com/JustinSornay/go_skeleton/internal/repository"
	"github.com/JustinSornay/go_skeleton/internal/repository/repositorytest"
)

func TestProjectRepositoryContract(t *testing.T) {
	repositorytest.ProjectRepositoryContract(t, func(t *testing.T) model.ProjectRepository {
		return repository.NewProjectRepository(dbtest.New(t))
	})
}
