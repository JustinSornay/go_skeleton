// Package repository implements the persistence interfaces declared in
// internal/model with handwritten SQL on pgx. SQL stays visible and
// reviewable; no ORM. Driver errors are translated into model errors here.
package repository

import (
	"context"
	"fmt"
	"strings"
	"time"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/JustinSornay/go_skeleton/internal/infrastructure/database"
	"github.com/JustinSornay/go_skeleton/internal/model"
)

const uniqueOwnerName = "projects_owner_name_key"

// ProjectRepository implements model.ProjectRepository on PostgreSQL.
type ProjectRepository struct {
	db *database.DB
}

// NewProjectRepository returns a Postgres-backed project repository.
func NewProjectRepository(db *database.DB) *ProjectRepository { return &ProjectRepository{db: db} }

var _ model.ProjectRepository = (*ProjectRepository)(nil)

const columns = `id, owner_id, name, description, created_at, updated_at`

// row mirrors the table; pgx maps columns to fields by name.
type row struct {
	ID          uuid.UUID `db:"id"`
	OwnerID     string    `db:"owner_id"`
	Name        string    `db:"name"`
	Description string    `db:"description"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

func (r row) toDomain() model.Project {
	return model.Project{
		ID: r.ID, OwnerID: r.OwnerID, Name: r.Name, Description: r.Description,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}
}

func (r *ProjectRepository) Create(ctx context.Context, p model.Project) error {
	_, err := r.db.Q(ctx).Exec(ctx, `
		INSERT INTO projects (`+columns+`)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		p.ID, p.OwnerID, p.Name, p.Description, p.CreatedAt, p.UpdatedAt)
	return mapErr(err, "insert project")
}

func (r *ProjectRepository) Get(ctx context.Context, ownerID string, id uuid.UUID) (model.Project, error) {
	rows, _ := r.db.Q(ctx).Query(ctx, `
		SELECT `+columns+` FROM projects WHERE id = $1 AND owner_id = $2`, id, ownerID)
	got, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[row])
	if err != nil {
		return model.Project{}, mapErr(err, "get project")
	}
	return got.toDomain(), nil
}

func (r *ProjectRepository) List(ctx context.Context, q model.ProjectListQuery) ([]model.Project, int, error) {
	// count(*) OVER () returns the total of the filtered set with the page,
	// in a single round-trip.
	rows, _ := r.db.Q(ctx).Query(ctx, `
		SELECT `+columns+`, count(*) OVER () AS total
		FROM projects
		WHERE owner_id = $1
		  AND ($2 = '' OR name ILIKE '%' || $2 || '%' ESCAPE '\')
		ORDER BY created_at DESC, id DESC
		LIMIT $3 OFFSET $4`,
		q.OwnerID, escapeLike(q.Search), q.Limit, q.Offset)

	type listRow struct {
		row
		Total int `db:"total"`
	}
	got, err := pgx.CollectRows(rows, pgx.RowToStructByName[listRow])
	if err != nil {
		return nil, 0, mapErr(err, "list projects")
	}
	if len(got) == 0 {
		if q.Offset == 0 {
			return []model.Project{}, 0, nil
		}
		// Past the last page: the window count is unavailable, ask directly.
		var total int
		err := r.db.Q(ctx).QueryRow(ctx, `
			SELECT count(*) FROM projects
			WHERE owner_id = $1 AND ($2 = '' OR name ILIKE '%' || $2 || '%' ESCAPE '\')`,
			q.OwnerID, escapeLike(q.Search)).Scan(&total)
		return []model.Project{}, total, mapErr(err, "count projects")
	}
	out := make([]model.Project, len(got))
	for i, g := range got {
		out[i] = g.toDomain()
	}
	return out, got[0].Total, nil
}

func (r *ProjectRepository) Update(ctx context.Context, p model.Project) error {
	tag, err := r.db.Q(ctx).Exec(ctx, `
		UPDATE projects SET name = $3, description = $4, updated_at = $5
		WHERE id = $1 AND owner_id = $2`,
		p.ID, p.OwnerID, p.Name, p.Description, p.UpdatedAt)
	if err != nil {
		return mapErr(err, "update project")
	}
	if tag.RowsAffected() == 0 {
		return model.ErrProjectNotFound
	}
	return nil
}

func (r *ProjectRepository) Delete(ctx context.Context, ownerID string, id uuid.UUID) error {
	tag, err := r.db.Q(ctx).Exec(ctx, `DELETE FROM projects WHERE id = $1 AND owner_id = $2`, id, ownerID)
	if err != nil {
		return mapErr(err, "delete project")
	}
	if tag.RowsAffected() == 0 {
		return model.ErrProjectNotFound
	}
	return nil
}

// mapErr translates driver errors into model errors; anything else
// is wrapped with context and becomes a 500 upstream.
func mapErr(err error, op string) error {
	switch {
	case err == nil:
		return nil
	case database.IsNoRows(err):
		return model.ErrProjectNotFound
	}
	if c, ok := database.UniqueViolation(err); ok && c == uniqueOwnerName {
		return model.ErrProjectNameTaken
	}
	return fmt.Errorf("%s: %w", op, err)
}

// escapeLike neutralises LIKE wildcards in user input.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
