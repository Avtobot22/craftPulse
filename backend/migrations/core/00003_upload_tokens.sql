-- +goose Up
CREATE INDEX upload_token_page ON auth.upload_token (organization_id, project_id, created_at DESC, id DESC);
-- +goose Down
DROP INDEX auth.upload_token_page;
