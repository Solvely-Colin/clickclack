CREATE TABLE github_app_installations (
  installation_id BIGINT NOT NULL,
  workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  account_login TEXT NOT NULL,
  account_type TEXT NOT NULL,
  repository_selection TEXT NOT NULL CHECK (repository_selection IN ('all', 'selected')),
  installed_by TEXT NOT NULL REFERENCES users(id),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (installation_id, workspace_id)
);

CREATE INDEX idx_github_app_installations_workspace
  ON github_app_installations(workspace_id, installation_id);

ALTER TABLE project_repositories
  ADD COLUMN github_installation_id BIGINT;

CREATE INDEX idx_project_repositories_github_installation
  ON project_repositories(github_installation_id, provider, full_name);
