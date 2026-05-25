package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bhpcv252/verge/internal/domain"
)

type BranchStore struct {
	db *pgxpool.Pool
}

func NewBranchStore(db *pgxpool.Pool) *BranchStore {
	return &BranchStore{db: db}
}

type ListBranchesPage struct {
	Branches   []*domain.Branch
	NextCursor string
}

func (s *BranchStore) Create(ctx context.Context, branch *domain.Branch) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO branches (name, repo_id, commit_id, created_at)
		 VALUES ($1, $2, $3, $4)`,
		branch.Name, branch.RepoID, branch.CommitID, branch.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.ErrBranchAlreadyExists
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			if pgErr.ConstraintName == "fk_branches_repo" {
				return domain.ErrRepoNotFound
			}
			if pgErr.ConstraintName == "fk_branches_commit_same_repo" {
				return domain.ErrCommitNotFound
			}
			return fmt.Errorf("postgres: create branch: foreign key violation on %s: %w",
				pgErr.ConstraintName, err)
		}
		return fmt.Errorf("postgres: create branch: %w", err)
	}
	return nil
}

func (s *BranchStore) GetByName(ctx context.Context, repoID, name string) (*domain.Branch, error) {
	row := s.db.QueryRow(ctx,
		`SELECT name, repo_id, commit_id, created_at
		 FROM branches
		 WHERE repo_id = $1 AND name = $2`,
		repoID, name,
	)
	branch := &domain.Branch{}
	err := row.Scan(&branch.Name, &branch.RepoID, &branch.CommitID, &branch.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrBranchNotFound
		}
		return nil, fmt.Errorf("postgres: get branch by name: %w", err)
	}
	return branch, nil
}

func (s *BranchStore) List(
	ctx context.Context,
	repoID string,
	limit int,
	cursor string,
) (*ListBranchesPage, error) {
	fetchLimit := limit + 1

	var (
		rows pgx.Rows
		err  error
	)

	if cursor == "" {
		rows, err = s.db.Query(ctx,
			`SELECT name, repo_id, commit_id, created_at
			 FROM branches
			 WHERE repo_id = $1
			 ORDER BY created_at DESC, name ASC
			 LIMIT $2`,
			repoID, fetchLimit,
		)
	} else {
		c, decErr := decodeBranchCursor(cursor)
		if decErr != nil {
			return nil, fmt.Errorf("postgres: invalid cursor: %w", decErr)
		}
		rows, err = s.db.Query(ctx,
			`SELECT name, repo_id, commit_id, created_at
			 FROM branches
			 WHERE repo_id = $1
			   AND (created_at, name) < ($2, $3)
			 ORDER BY created_at DESC, name ASC
			 LIMIT $4`,
			repoID, c.CreatedAt, c.Name, fetchLimit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: list branches: %w", err)
	}
	defer rows.Close()

	branches := make([]*domain.Branch, 0, fetchLimit)
	for rows.Next() {
		b := &domain.Branch{}
		if err := rows.Scan(&b.Name, &b.RepoID, &b.CommitID, &b.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: list branches scan: %w", err)
		}
		branches = append(branches, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list branches rows: %w", err)
	}

	page := &ListBranchesPage{}
	if len(branches) > limit {
		page.Branches = branches[:limit]
		page.NextCursor = encodeBranchCursor(branches[limit-1])
	} else {
		page.Branches = branches
	}
	return page, nil
}

func (s *BranchStore) Advance(
	ctx context.Context,
	repoID, name, commitID, expectedCommitID string,
) (*domain.Branch, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: advance branch: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	branch := &domain.Branch{}
	err = tx.QueryRow(ctx,
		`UPDATE branches
		 SET commit_id = $1
		 WHERE repo_id = $2 AND name = $3 AND commit_id = $4
		 RETURNING name, repo_id, commit_id, created_at`,
		commitID, repoID, name, expectedCommitID,
	).Scan(&branch.Name, &branch.RepoID, &branch.CommitID, &branch.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// fetch current head for the conflict error
			var currentHead string
			checkErr := tx.QueryRow(ctx,
				`SELECT commit_id FROM branches WHERE repo_id = $1 AND name = $2`,
				repoID, name,
			).Scan(&currentHead)
			if checkErr != nil {
				if errors.Is(checkErr, pgx.ErrNoRows) {
					return nil, domain.ErrBranchNotFound
				}
				return nil, fmt.Errorf("postgres: advance branch: check current head: %w", checkErr)
			}
			return nil, &BranchConflictError{CurrentHead: currentHead}
		}
		return nil, fmt.Errorf("postgres: advance branch: %w", err)
	}

	// version = unix millis of this event's created_at
	now := time.Now().UTC()
	version := now.UnixMilli()

	payload := map[string]interface{}{
		"repo_id":   repoID,
		"branch":    name,
		"commit_id": commitID,
		"version":   version,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("postgres: advance branch: marshal outbox payload: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO outbox_events (id, event_type, payload, created_at, processed)
		 VALUES ($1, $2, $3, $4, $5)`,
		"evt_"+uuid.New().String(), "BranchHeadMoved", payloadJSON, now, false,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: advance branch: insert outbox event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("postgres: advance branch: commit tx: %w", err)
	}

	return branch, nil
}

type BranchConflictError struct {
	CurrentHead string
}

func (e *BranchConflictError) Error() string { return "branch conflict" }
func (e *BranchConflictError) Is(target error) bool {
	return target == domain.ErrBranchConflict
}

func (s *BranchStore) Delete(ctx context.Context, repoID, name string) error {
	result, err := s.db.Exec(ctx,
		`DELETE FROM branches WHERE repo_id = $1 AND name = $2`,
		repoID, name,
	)
	if err != nil {
		return fmt.Errorf("postgres: delete branch: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrBranchNotFound
	}
	return nil
}

// cursor encoding
type branchCursor struct {
	CreatedAt time.Time `json:"ca"`
	Name      string    `json:"n"`
}

func encodeBranchCursor(b *domain.Branch) string {
	data, _ := json.Marshal(branchCursor{CreatedAt: b.CreatedAt, Name: b.Name})
	return base64.StdEncoding.EncodeToString(data)
}

func decodeBranchCursor(s string) (*branchCursor, error) {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	var c branchCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	return &c, nil
}
