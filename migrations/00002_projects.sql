-- +goose Up
-- Example table (model/project.go, repository/project_repository.go). Replace or delete it together
-- with the project_* files when starting a real project.
CREATE TABLE projects (
    id          uuid        PRIMARY KEY,
    owner_id    text        NOT NULL,
    name        text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    description text        NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- A name is unique per owner, case-insensitively.
CREATE UNIQUE INDEX projects_owner_name_key ON projects (owner_id, lower(name));
-- Serves the default listing order.
CREATE INDEX projects_owner_created_idx ON projects (owner_id, created_at DESC, id DESC);

-- +goose Down
DROP TABLE projects;
