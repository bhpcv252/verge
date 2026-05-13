package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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
	NextCursor string // empty string - no more pages
}

func (s *BranchStore) Create(ctx context.Context, branch *domain.Branch) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO branches (name, repo_id, commit_id, created_at)
		 VALUES ($1, $2, $3, $4)`,
		branch.Name, branch.RepoID, branch.CommitID, branch.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return domain.ErrBranchAlreadyExists
		}
		if errors.As(err, &pgErr) && pgErr.Code == "23503" { // foreign_key_violation
			constraintName := pgErr.ConstraintName

			// check which FK constraint failed
			if constraintName == "fk_branches_repo" {
				return domain.ErrRepoNotFound
			}
			if constraintName == "fk_branches_commit_same_repo" {
				return domain.ErrCommitNotFound
			}

			// fallback (shouldn't happen)
			return fmt.Errorf(
				"postgres: create branch: foreign key violation on %s: %w",
				constraintName,
				err,
			)
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

// pass an empty cursor for the first page;
// use ListBranchesPage.NextCursor for subsequent pages
func (s *BranchStore) List(
	ctx context.Context,
	repoID string,
	limit int,
	cursor string,
) (*ListBranchesPage, error) {
	fetchLimit := limit + 1 // fetch one extra to check if there is another page.

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

// advance advances the branch pointer with optimistic locking,
func (s *BranchStore) Advance(
	ctx context.Context,
	repoID, name, commitID, expectedCommitID string,
) (*domain.Branch, error) {
	row := s.db.QueryRow(ctx,
		`UPDATE branches
		 SET commit_id = $1
		 WHERE repo_id = $2 AND name = $3 AND commit_id = $4
		 RETURNING name, repo_id, commit_id, created_at`,
		commitID, repoID, name, expectedCommitID,
	)

	branch := &domain.Branch{}
	err := row.Scan(&branch.Name, &branch.RepoID, &branch.CommitID, &branch.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			existingBranch, checkErr := s.GetByName(ctx, repoID, name)
			if checkErr != nil {
				if errors.Is(checkErr, domain.ErrBranchNotFound) {
					return nil, domain.ErrBranchNotFound
				}
				return nil, fmt.Errorf("postgres: advance branch check: %w", checkErr)
			}
			// branch exists but expected_commit_id doesn't match
			return nil, &BranchConflictError{
				CurrentHead: existingBranch.CommitID,
			}
		}
		return nil, fmt.Errorf("postgres: advance branch: %w", err)
	}

	return branch, nil
}

// BranchConflictError is returned when optimistic
// locking fails during branch advancement
type BranchConflictError struct {
	CurrentHead string
}

func (e *BranchConflictError) Error() string {
	return "branch conflict"
}

func (e *BranchConflictError) Is(target error) bool {
	return target == domain.ErrBranchConflict
}

func (s *BranchStore) Delete(ctx context.Context, repoID, name string) error {
	result, err := s.db.Exec(ctx,
		`DELETE FROM branches
		 WHERE repo_id = $1 AND name = $2`,
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
