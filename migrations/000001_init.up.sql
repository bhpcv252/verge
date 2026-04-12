CREATE TABLE repos (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    default_branch TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE commits (
    id TEXT NOT NULL,
    repo_id TEXT NOT NULL,
    message TEXT NOT NULL,
    author TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT NOW(),
    data_pointer JSONB NOT NULL,
    idempotency_key TEXT,

    PRIMARY KEY (id),

    CONSTRAINT fk_commits_repo
        FOREIGN KEY (repo_id) REFERENCES repos(id)
        ON DELETE CASCADE,

    CONSTRAINT commits_id_repo_unique
        UNIQUE (id, repo_id)
);

CREATE INDEX idx_commits_repo_id ON commits(repo_id);

CREATE INDEX idx_commits_idempotency 
    ON commits(repo_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE TABLE commit_parents (
    commit_id TEXT NOT NULL,
    parent_id TEXT NOT NULL,
    repo_id TEXT NOT NULL,

    PRIMARY KEY (commit_id, parent_id),

    CONSTRAINT fk_cp_commit_same_repo
        FOREIGN KEY (commit_id, repo_id)
        REFERENCES commits(id, repo_id)
        ON DELETE CASCADE,

    CONSTRAINT fk_cp_parent_same_repo
        FOREIGN KEY (parent_id, repo_id)
        REFERENCES commits(id, repo_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_cp_parent_id ON commit_parents(parent_id);
CREATE INDEX idx_cp_repo_id ON commit_parents(repo_id);

CREATE TABLE branches (
    name TEXT NOT NULL,
    repo_id TEXT NOT NULL,
    commit_id TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),

    PRIMARY KEY (name, repo_id),

    CONSTRAINT fk_branches_repo
        FOREIGN KEY (repo_id) REFERENCES repos(id)
        ON DELETE CASCADE,

    -- Enforces branch commit belongs to same repo
    CONSTRAINT fk_branches_commit_same_repo
        FOREIGN KEY (commit_id, repo_id)
        REFERENCES commits(id, repo_id)
);

CREATE INDEX idx_branches_repo_id ON branches(repo_id);

CREATE TABLE outbox_events (
    id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    processed BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_outbox_unprocessed 
    ON outbox_events(processed, created_at);
