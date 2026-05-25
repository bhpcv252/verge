package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bhpcv252/verge/internal/domain"
	"github.com/bhpcv252/verge/internal/storage/interfaces"
)

type GraphStore struct {
	db *pgxpool.Pool
}

func NewGraphStore(db *pgxpool.Pool) *GraphStore {
	return &GraphStore{db: db}
}

func (s *GraphStore) TraverseDAG(
	ctx context.Context,
	params interfaces.TraversalParams,
) ([]*domain.Commit, string, error) {
	if params.Head == "" {
		return nil, "", fmt.Errorf("postgres graph: TraverseDAG requires a Head commit ID")
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	fetchLimit := limit + 1

	args := []interface{}{params.RepoID, params.Head}
	argIdx := 3

	query := `
		WITH RECURSIVE ancestors AS (
			SELECT c.id
			FROM commits c
			WHERE c.repo_id = $1 AND c.id = $2

			UNION

			SELECT cp.parent_id
			FROM commit_parents cp
			INNER JOIN ancestors a ON cp.commit_id = a.id
			WHERE cp.repo_id = $1
		)
		SELECT c.id, c.repo_id, c.message, c.author, c.timestamp, c.data_pointer
		FROM commits c
		INNER JOIN ancestors a ON c.id = a.id
		WHERE c.repo_id = $1`

	if params.Author != "" {
		query += fmt.Sprintf(" AND c.author = $%d", argIdx)
		args = append(args, params.Author)
		argIdx++
	}
	if params.Since != nil {
		query += fmt.Sprintf(" AND c.timestamp >= $%d", argIdx)
		args = append(args, *params.Since)
		argIdx++
	}
	if params.Until != nil {
		query += fmt.Sprintf(" AND c.timestamp <= $%d", argIdx)
		args = append(args, *params.Until)
		argIdx++
	}
	if params.Cursor != "" {
		cur, err := decodeCommitCursor(params.Cursor)
		if err != nil {
			return nil, "", fmt.Errorf("postgres graph: invalid cursor: %w", err)
		}
		query += fmt.Sprintf(" AND (c.timestamp, c.id) < ($%d, $%d)", argIdx, argIdx+1)
		args = append(args, cur.Timestamp, cur.ID)
		argIdx += 2
	}

	query += " ORDER BY c.timestamp DESC, c.id DESC"
	query += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, fetchLimit)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("postgres graph: traverse dag: %w", err)
	}
	defer rows.Close()

	commits, err := pgScanCommitRows(ctx, s.db, rows)
	if err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(commits) > limit {
		nextCursor = encodeCommitCursor(commits[limit-1])
		commits = commits[:limit]
	}

	return commits, nextCursor, nil
}

func (s *GraphStore) GetAncestors(
	ctx context.Context,
	repoID, commitID string,
	limit int,
) ([]*domain.Commit, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.Query(ctx, `
		WITH RECURSIVE ancestors AS (
			SELECT c.id
			FROM commits c
			WHERE c.repo_id = $1 AND c.id = $2

			UNION

			SELECT cp.parent_id
			FROM commit_parents cp
			INNER JOIN ancestors a ON cp.commit_id = a.id
			WHERE cp.repo_id = $1
		)
		SELECT c.id, c.repo_id, c.message, c.author, c.timestamp, c.data_pointer
		FROM commits c
		INNER JOIN ancestors a ON c.id = a.id
		WHERE c.repo_id = $1 AND c.id != $2
		ORDER BY c.timestamp DESC, c.id DESC
		LIMIT $3`,
		repoID, commitID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres graph: get ancestors: %w", err)
	}
	defer rows.Close()

	return pgScanCommitRows(ctx, s.db, rows)
}

func (s *GraphStore) FindMergeBase(
	ctx context.Context,
	repoID, commitA, commitB string,
) (*domain.Commit, error) {
	row := s.db.QueryRow(ctx, `
		WITH RECURSIVE
		ancestors_a AS (
			SELECT id FROM commits WHERE repo_id = $1 AND id = $2
			UNION
			SELECT cp.parent_id
			FROM commit_parents cp
			INNER JOIN ancestors_a a ON cp.commit_id = a.id
			WHERE cp.repo_id = $1
		),
		ancestors_b AS (
			SELECT id FROM commits WHERE repo_id = $1 AND id = $3
			UNION
			SELECT cp.parent_id
			FROM commit_parents cp
			INNER JOIN ancestors_b a ON cp.commit_id = a.id
			WHERE cp.repo_id = $1
		)
		SELECT c.id, c.repo_id, c.message, c.author, c.timestamp, c.data_pointer
		FROM commits c
		WHERE c.repo_id = $1
		  AND c.id IN (SELECT id FROM ancestors_a)
		  AND c.id IN (SELECT id FROM ancestors_b)
		ORDER BY c.timestamp DESC
		LIMIT 1`,
		repoID, commitA, commitB,
	)

	commit := &domain.Commit{}
	var dataPointerJSON []byte
	err := row.Scan(
		&commit.ID, &commit.RepoID, &commit.Message,
		&commit.Author, &commit.Timestamp, &dataPointerJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres graph: find merge base: %w", err)
	}
	if err := json.Unmarshal(dataPointerJSON, &commit.DataPointer); err != nil {
		return nil, fmt.Errorf("postgres graph: unmarshal data_pointer: %w", err)
	}
	return commit, nil
}

func pgScanCommitRows(
	ctx context.Context,
	db *pgxpool.Pool,
	rows pgx.Rows,
) ([]*domain.Commit, error) {
	commits := make([]*domain.Commit, 0)
	for rows.Next() {
		commit := &domain.Commit{}
		var dataPointerJSON []byte
		if err := rows.Scan(
			&commit.ID, &commit.RepoID, &commit.Message,
			&commit.Author, &commit.Timestamp, &dataPointerJSON,
		); err != nil {
			return nil, fmt.Errorf("postgres graph: scan commit: %w", err)
		}
		if err := json.Unmarshal(dataPointerJSON, &commit.DataPointer); err != nil {
			return nil, fmt.Errorf("postgres graph: unmarshal data_pointer: %w", err)
		}

		parentRows, err := db.Query(ctx,
			`SELECT parent_id FROM commit_parents WHERE commit_id = $1 ORDER BY parent_id`,
			commit.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("postgres graph: query parents for %s: %w", commit.ID, err)
		}
		commit.ParentIDs = []string{}
		for parentRows.Next() {
			var pid string
			if err := parentRows.Scan(&pid); err != nil {
				parentRows.Close()
				return nil, fmt.Errorf("postgres graph: scan parent_id: %w", err)
			}
			commit.ParentIDs = append(commit.ParentIDs, pid)
		}
		parentRows.Close()
		if err := parentRows.Err(); err != nil {
			return nil, fmt.Errorf("postgres graph: parent rows error: %w", err)
		}

		commits = append(commits, commit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres graph: rows error: %w", err)
	}
	return commits, nil
}
