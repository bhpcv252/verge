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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/bhpcv252/verge/internal/domain"
)

type CommitStore struct {
	db *pgxpool.Pool
}

func NewCommitStore(db *pgxpool.Pool) *CommitStore {
	return &CommitStore{db: db}
}

type ListCommitsFilter struct {
	RepoID    string
	Branch    string
	Author    string
	Since     *time.Time
	Until     *time.Time
	Traversal string // "flat" | "dag"
	Limit     int
	Cursor    string
}

type ListCommitsPage struct {
	Commits    []*domain.Commit
	NextCursor string
}

func (s *CommitStore) Create(
	ctx context.Context,
	commit *domain.Commit,
	parentIDs []string,
) (*domain.Commit, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	dataPointerJSON, err := json.Marshal(commit.DataPointer)
	if err != nil {
		return nil, fmt.Errorf("postgres: marshal data_pointer: %w", err)
	}

	var idempotencyKeyPtr *string
	if commit.IdempotencyKey != "" {
		idempotencyKeyPtr = &commit.IdempotencyKey
	}

	_, err = tx.Exec(
		ctx,
		`INSERT INTO commits (id, repo_id, message, author, timestamp, data_pointer, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		commit.ID,
		commit.RepoID,
		commit.Message,
		commit.Author,
		commit.Timestamp,
		dataPointerJSON,
		idempotencyKeyPtr,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: insert commit: %w", err)
	}

	for _, parentID := range parentIDs {
		_, err = tx.Exec(ctx,
			`INSERT INTO commit_parents (commit_id, parent_id, repo_id) VALUES ($1, $2, $3)`,
			commit.ID, parentID, commit.RepoID,
		)
		if err != nil {
			return nil, fmt.Errorf("postgres: insert commit_parent: %w", err)
		}
	}

	if err := txInsertOutboxEvent(ctx, tx, "CommitCreated", map[string]interface{}{
		"commit_id":  commit.ID,
		"repo_id":    commit.RepoID,
		"parent_ids": parentIDs,
		"author":     commit.Author,
		"message":    commit.Message,
		"timestamp":  commit.Timestamp.Format(time.RFC3339),
	}); err != nil {
		return nil, fmt.Errorf("postgres: insert CommitCreated event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("postgres: commit transaction: %w", err)
	}

	return commit, nil
}

func (s *CommitStore) GetByID(
	ctx context.Context,
	repoID, commitID string,
) (*domain.Commit, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, repo_id, message, author, timestamp, data_pointer
		 FROM commits
		 WHERE repo_id = $1 AND id = $2`,
		repoID, commitID,
	)

	commit := &domain.Commit{}
	var dataPointerJSON []byte
	err := row.Scan(
		&commit.ID, &commit.RepoID, &commit.Message,
		&commit.Author, &commit.Timestamp, &dataPointerJSON,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrCommitNotFound
		}
		return nil, fmt.Errorf("postgres: get commit by id: %w", err)
	}
	if err := json.Unmarshal(dataPointerJSON, &commit.DataPointer); err != nil {
		return nil, fmt.Errorf("postgres: unmarshal data_pointer: %w", err)
	}

	commit.ParentIDs = []string{}
	parentRows, err := s.db.Query(ctx,
		`SELECT parent_id FROM commit_parents WHERE commit_id = $1 ORDER BY parent_id`,
		commitID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: query commit parents: %w", err)
	}
	defer parentRows.Close()
	for parentRows.Next() {
		var pid string
		if err := parentRows.Scan(&pid); err != nil {
			return nil, fmt.Errorf("postgres: scan parent_id: %w", err)
		}
		commit.ParentIDs = append(commit.ParentIDs, pid)
	}
	if err := parentRows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: parent rows error: %w", err)
	}

	return commit, nil
}

func (s *CommitStore) GetByIdempotencyKey(
	ctx context.Context,
	repoID, idempotencyKey string,
) (*domain.Commit, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, repo_id, message, author, timestamp, data_pointer, idempotency_key
		 FROM commits
		 WHERE repo_id = $1 AND idempotency_key = $2`,
		repoID, idempotencyKey,
	)

	commit := &domain.Commit{}
	var dataPointerJSON []byte
	var ikPtr *string
	err := row.Scan(
		&commit.ID, &commit.RepoID, &commit.Message,
		&commit.Author, &commit.Timestamp, &dataPointerJSON, &ikPtr,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrCommitNotFound
		}
		return nil, fmt.Errorf("postgres: get commit by idempotency key: %w", err)
	}
	if ikPtr != nil {
		commit.IdempotencyKey = *ikPtr
	}
	if err := json.Unmarshal(dataPointerJSON, &commit.DataPointer); err != nil {
		return nil, fmt.Errorf("postgres: unmarshal data_pointer: %w", err)
	}

	commit.ParentIDs = []string{}
	parentRows, err := s.db.Query(ctx,
		`SELECT parent_id FROM commit_parents WHERE commit_id = $1 ORDER BY parent_id`,
		commit.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: query commit parents: %w", err)
	}
	defer parentRows.Close()
	for parentRows.Next() {
		var pid string
		if err := parentRows.Scan(&pid); err != nil {
			return nil, fmt.Errorf("postgres: scan parent_id: %w", err)
		}
		commit.ParentIDs = append(commit.ParentIDs, pid)
	}

	return commit, nil
}

func (s *CommitStore) ValidateParentsExist(
	ctx context.Context,
	repoID string,
	parentIDs []string,
) error {
	if len(parentIDs) == 0 {
		return nil
	}
	var count int
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM commits WHERE repo_id = $1 AND id = ANY($2)`,
		repoID, parentIDs,
	).Scan(&count)
	if err != nil {
		return fmt.Errorf("postgres: validate parents exist: %w", err)
	}
	if count != len(parentIDs) {
		return domain.ErrInvalidParent
	}
	return nil
}

func (s *CommitStore) List(
	ctx context.Context,
	filter ListCommitsFilter,
) (*ListCommitsPage, error) {
	fetchLimit := filter.Limit + 1

	var query string
	args := []interface{}{filter.RepoID}
	argIdx := 2

	if filter.Branch != "" {
		query = `
			WITH RECURSIVE ancestors AS (
				SELECT c.id
				FROM commits c
				INNER JOIN branches b ON c.id = b.commit_id
				WHERE b.repo_id = $1 AND b.name = $` + fmt.Sprintf("%d", argIdx)

		args = append(args, filter.Branch)
		argIdx++

		query += `
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
	} else {
		query = `SELECT c.id, c.repo_id, c.message, c.author, c.timestamp, c.data_pointer
		          FROM commits c WHERE c.repo_id = $1`
	}

	if filter.Author != "" {
		query += fmt.Sprintf(" AND c.author = $%d", argIdx)
		args = append(args, filter.Author)
		argIdx++
	}
	if filter.Since != nil {
		query += fmt.Sprintf(" AND c.timestamp >= $%d", argIdx)
		args = append(args, *filter.Since)
		argIdx++
	}
	if filter.Until != nil {
		query += fmt.Sprintf(" AND c.timestamp <= $%d", argIdx)
		args = append(args, *filter.Until)
		argIdx++
	}
	if filter.Cursor != "" {
		c, err := decodeCommitCursor(filter.Cursor)
		if err != nil {
			return nil, fmt.Errorf("postgres: invalid cursor: %w", err)
		}
		query += fmt.Sprintf(" AND (c.timestamp, c.id) < ($%d, $%d)", argIdx, argIdx+1)
		args = append(args, c.Timestamp, c.ID)
		argIdx += 2
	}

	query += " ORDER BY c.timestamp DESC, c.id DESC"
	query += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, fetchLimit)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list commits: %w", err)
	}
	defer rows.Close()

	commits := make([]*domain.Commit, 0, fetchLimit)
	for rows.Next() {
		commit := &domain.Commit{}
		var dataPointerJSON []byte
		if err := rows.Scan(&commit.ID, &commit.RepoID, &commit.Message,
			&commit.Author, &commit.Timestamp, &dataPointerJSON); err != nil {
			return nil, fmt.Errorf("postgres: list commits scan: %w", err)
		}
		if err := json.Unmarshal(dataPointerJSON, &commit.DataPointer); err != nil {
			return nil, fmt.Errorf("postgres: unmarshal data_pointer: %w", err)
		}

		parentRows, err := s.db.Query(ctx,
			`SELECT parent_id FROM commit_parents WHERE commit_id = $1 ORDER BY parent_id`,
			commit.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("postgres: query commit parents: %w", err)
		}
		commit.ParentIDs = []string{}
		for parentRows.Next() {
			var pid string
			if err := parentRows.Scan(&pid); err != nil {
				parentRows.Close()
				return nil, fmt.Errorf("postgres: scan parent_id: %w", err)
			}
			commit.ParentIDs = append(commit.ParentIDs, pid)
		}
		parentRows.Close()
		commits = append(commits, commit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list commits rows: %w", err)
	}

	page := &ListCommitsPage{}
	if len(commits) > filter.Limit {
		page.Commits = commits[:filter.Limit]
		page.NextCursor = encodeCommitCursor(commits[filter.Limit-1])
	} else {
		page.Commits = commits
	}
	return page, nil
}

func (s *CommitStore) GetParents(
	ctx context.Context,
	repoID, commitID string,
) ([]*domain.Commit, error) {
	var exists bool
	if err := s.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM commits WHERE id = $1 AND repo_id = $2)`,
		commitID, repoID,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("postgres: check commit exists: %w", err)
	}
	if !exists {
		return nil, domain.ErrCommitNotFound
	}

	rows, err := s.db.Query(ctx,
		`SELECT c.id, c.repo_id, c.message, c.author, c.timestamp, c.data_pointer
		 FROM commits c
		 INNER JOIN commit_parents cp ON c.id = cp.parent_id
		 WHERE cp.commit_id = $1 AND cp.repo_id = $2
		 ORDER BY c.timestamp DESC`,
		commitID, repoID,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: get parents: %w", err)
	}
	defer rows.Close()

	parents := []*domain.Commit{}
	for rows.Next() {
		commit := &domain.Commit{}
		var dataPointerJSON []byte
		if err := rows.Scan(&commit.ID, &commit.RepoID, &commit.Message,
			&commit.Author, &commit.Timestamp, &dataPointerJSON); err != nil {
			return nil, fmt.Errorf("postgres: get parents scan: %w", err)
		}
		if err := json.Unmarshal(dataPointerJSON, &commit.DataPointer); err != nil {
			return nil, fmt.Errorf("postgres: unmarshal data_pointer: %w", err)
		}

		parentRows, err := s.db.Query(ctx,
			`SELECT parent_id FROM commit_parents WHERE commit_id = $1 ORDER BY parent_id`,
			commit.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("postgres: query commit parents: %w", err)
		}
		commit.ParentIDs = []string{}
		for parentRows.Next() {
			var pid string
			if err := parentRows.Scan(&pid); err != nil {
				parentRows.Close()
				return nil, fmt.Errorf("postgres: scan parent_id: %w", err)
			}
			commit.ParentIDs = append(commit.ParentIDs, pid)
		}
		parentRows.Close()
		parents = append(parents, commit)
	}

	return parents, nil
}

func (s *CommitStore) CreateMerge(
	ctx context.Context,
	commit *domain.Commit,
	parentIDs []string,
	targetBranch, expectedTargetHead string,
) (*domain.Commit, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	dataPointerJSON, err := json.Marshal(commit.DataPointer)
	if err != nil {
		return nil, fmt.Errorf("postgres: marshal data_pointer: %w", err)
	}

	var idempotencyKeyPtr *string
	if commit.IdempotencyKey != "" {
		idempotencyKeyPtr = &commit.IdempotencyKey
	}

	_, err = tx.Exec(
		ctx,
		`INSERT INTO commits (id, repo_id, message, author, timestamp, data_pointer, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		commit.ID,
		commit.RepoID,
		commit.Message,
		commit.Author,
		commit.Timestamp,
		dataPointerJSON,
		idempotencyKeyPtr,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: insert merge commit: %w", err)
	}

	for _, parentID := range parentIDs {
		_, err = tx.Exec(ctx,
			`INSERT INTO commit_parents (commit_id, parent_id, repo_id) VALUES ($1, $2, $3)`,
			commit.ID, parentID, commit.RepoID,
		)
		if err != nil {
			return nil, fmt.Errorf("postgres: insert commit_parent: %w", err)
		}
	}

	// advance target branch with optimistic lock
	var updatedName string
	err = tx.QueryRow(ctx,
		`UPDATE branches
		 SET commit_id = $1
		 WHERE repo_id = $2 AND name = $3 AND commit_id = $4
		 RETURNING name`,
		commit.ID, commit.RepoID, targetBranch, expectedTargetHead,
	).Scan(&updatedName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var currentHead string
			checkErr := tx.QueryRow(ctx,
				`SELECT commit_id FROM branches WHERE repo_id = $1 AND name = $2`,
				commit.RepoID, targetBranch,
			).Scan(&currentHead)
			if checkErr != nil {
				return nil, fmt.Errorf("postgres: check branch head: %w", checkErr)
			}
			return nil, &MergeBranchConflictError{
				BranchName:   targetBranch,
				CurrentHead:  currentHead,
				ExpectedHead: expectedTargetHead,
			}
		}
		return nil, fmt.Errorf("postgres: advance target branch: %w", err)
	}

	now := time.Now().UTC()

	// CommitCreated
	if err := txInsertOutboxEvent(ctx, tx, "CommitCreated", map[string]interface{}{
		"commit_id":  commit.ID,
		"repo_id":    commit.RepoID,
		"parent_ids": parentIDs,
		"author":     commit.Author,
		"message":    commit.Message,
		"timestamp":  commit.Timestamp.Format(time.RFC3339),
	}); err != nil {
		return nil, fmt.Errorf("postgres: insert CommitCreated event: %w", err)
	}

	// BranchHeadMoved
	if err := txInsertOutboxEventAt(ctx, tx, "BranchHeadMoved", map[string]interface{}{
		"repo_id":   commit.RepoID,
		"branch":    targetBranch,
		"commit_id": commit.ID,
		"version":   now.UnixMilli(),
	}, now); err != nil {
		return nil, fmt.Errorf("postgres: insert BranchHeadMoved event: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("postgres: commit merge transaction: %w", err)
	}

	return commit, nil
}

type MergeBranchConflictError struct {
	BranchName   string
	CurrentHead  string
	ExpectedHead string
}

func (e *MergeBranchConflictError) Error() string { return "merge branch conflict" }
func (e *MergeBranchConflictError) Is(target error) bool {
	return target == domain.ErrStaleMergeTarget
}

func txInsertOutboxEvent(
	ctx context.Context,
	tx pgx.Tx,
	eventType string,
	payload map[string]interface{},
) error {
	return txInsertOutboxEventAt(ctx, tx, eventType, payload, time.Now().UTC())
}

func txInsertOutboxEventAt(
	ctx context.Context,
	tx pgx.Tx,
	eventType string,
	payload map[string]interface{},
	createdAt time.Time,
) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO outbox_events (id, event_type, payload, created_at, processed)
		 VALUES ($1, $2, $3, $4, $5)`,
		"evt_"+uuid.New().String(), eventType, payloadJSON, createdAt, false,
	)
	return err
}

type commitCursor struct {
	Timestamp time.Time `json:"ts"`
	ID        string    `json:"id"`
}

func encodeCommitCursor(c *domain.Commit) string {
	data, _ := json.Marshal(commitCursor{Timestamp: c.Timestamp, ID: c.ID})
	return base64.StdEncoding.EncodeToString(data)
}

func decodeCommitCursor(s string) (*commitCursor, error) {
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	var c commitCursor
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	return &c, nil
}
