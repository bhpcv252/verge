package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bhpcv252/verge/internal/domain"
)

type RepoStore struct {
	db *pgxpool.Pool
}

func NewRepoStore(db *pgxpool.Pool) *RepoStore {
	return &RepoStore{db: db}
}

type ListReposPage struct {
	Repos      []*domain.Repo
	NextCursor string // empty string = no more pages
}

func (s *RepoStore) Create(ctx context.Context, repo *domain.Repo) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO repos (id, name, default_branch, created_at)
		 VALUES ($1, $2, $3, $4)`,
		repo.ID, repo.Name, repo.DefaultBranch, repo.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: create repo: %w", err)
	}
	return nil
}

func (s *RepoStore) GetByID(ctx context.Context, id string) (*domain.Repo, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, name, default_branch, created_at
		 FROM repos
		 WHERE id = $1`,
		id,
	)

	repo := &domain.Repo{}
	err := row.Scan(&repo.ID, &repo.Name, &repo.DefaultBranch, &repo.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrRepoNotFound
		}
		return nil, fmt.Errorf("postgres: get repo by id: %w", err)
	}
	return repo, nil
}

// Pass an empty cursor for the first page;
// use ListReposPage.NextCursor for subsequent pages.
func (s *RepoStore) List(ctx context.Context, limit int, cursor string) (*ListReposPage, error) {
	fetchLimit := limit + 1 // Fetch one extra to check if there is another page.

	var (
		rows pgx.Rows
		err  error
	)

	if cursor == "" {
		rows, err = s.db.Query(ctx,
			`SELECT id, name, default_branch, created_at
			 FROM repos
			 ORDER BY created_at DESC
			 LIMIT $1`,
			fetchLimit,
		)
	} else {
		c, decErr := decodeRepoCursor(cursor)
		if decErr != nil {
			return nil, fmt.Errorf("postgres: invalid cursor: %w", decErr)
		}

		rows, err = s.db.Query(ctx,
			`SELECT id, name, default_branch, created_at
			 FROM repos
			 WHERE (created_at, id) < ($2, $3)
			 ORDER BY created_at DESC
			 LIMIT $1`,
			fetchLimit, c.CreatedAt, c.ID,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: list repos: %w", err)
	}
	defer rows.Close()

	repos := make([]*domain.Repo, 0, fetchLimit)
	for rows.Next() {
		r := &domain.Repo{}
		if err := rows.Scan(&r.ID, &r.Name, &r.DefaultBranch, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("postgres: list repos scan: %w", err)
		}
		repos = append(repos, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list repos rows: %w", err)
	}

	page := &ListReposPage{}
	if len(repos) > limit {
		page.Repos = repos[:limit]
		page.NextCursor = encodeRepoCursor(repos[limit-1])
	} else {
		page.Repos = repos
	}

	return page, nil
}

// cursor encoding
type repoCursor struct {
	CreatedAt time.Time `json:"ca"`
	ID        string    `json:"id"`
}

func encodeRepoCursor(r *domain.Repo) string {
	b, _ := json.Marshal(repoCursor{CreatedAt: r.CreatedAt, ID: r.ID})
	return base64.StdEncoding.EncodeToString(b)
}

func decodeRepoCursor(s string) (*repoCursor, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	var c repoCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	return &c, nil
}
