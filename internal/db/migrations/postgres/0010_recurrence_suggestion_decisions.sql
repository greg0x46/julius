-- +goose Up
CREATE TABLE recurrence_suggestion_decisions (
    pattern_key TEXT PRIMARY KEY,
    state TEXT NOT NULL CHECK (state IN ('dismissed', 'accepted')),
    scenario_id TEXT,
    approval_token TEXT
);

-- +goose Down
DROP TABLE recurrence_suggestion_decisions;
