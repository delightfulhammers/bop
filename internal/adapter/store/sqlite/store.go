package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/delightfulhammers/bop/internal/store"
	_ "modernc.org/sqlite" // Pure-Go SQLite driver (no cgo required)
)

// Store implements the store.Store interface using SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a new SQLite store at the given path.
// Use ":memory:" for in-memory database (useful for testing).
func NewStore(dbPath string) (*Store, error) {
	// Add busy timeout to handle concurrent access gracefully.
	// The modernc.org/sqlite driver returns errors immediately on lock contention
	// without a timeout configured.
	dsn := dbPath
	if dbPath != ":memory:" && !strings.Contains(dbPath, "?") {
		dsn = dbPath + "?_busy_timeout=5000"
	} else if dbPath != ":memory:" && !strings.Contains(dbPath, "_busy_timeout") {
		dsn = dbPath + "&_busy_timeout=5000"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Limit concurrent connections to avoid lock contention.
	// SQLite doesn't handle concurrent writes well without WAL mode.
	db.SetMaxOpenConns(1)

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	s := &Store{db: db}

	if err := s.createSchema(); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	return s, nil
}

// createSchema creates all tables and indexes if they don't exist.
func (s *Store) createSchema() error {
	schema := `
	-- Stores metadata about each review run
	CREATE TABLE IF NOT EXISTS runs (
		run_id TEXT PRIMARY KEY,
		timestamp INTEGER NOT NULL,
		scope TEXT NOT NULL,
		config_hash TEXT NOT NULL,
		total_cost REAL DEFAULT 0.0,
		base_ref TEXT NOT NULL,
		target_ref TEXT NOT NULL,
		repository TEXT NOT NULL
	);

	-- Individual reviews from each provider
	CREATE TABLE IF NOT EXISTS reviews (
		review_id TEXT PRIMARY KEY,
		run_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		summary TEXT,
		created_at INTEGER NOT NULL,
		FOREIGN KEY (run_id) REFERENCES runs(run_id) ON DELETE CASCADE
	);

	-- Individual findings from reviews
	CREATE TABLE IF NOT EXISTS findings (
		finding_id TEXT PRIMARY KEY,
		review_id TEXT NOT NULL,
		finding_hash TEXT NOT NULL,
		file TEXT NOT NULL,
		line_start INTEGER NOT NULL,
		line_end INTEGER NOT NULL,
		category TEXT NOT NULL,
		severity TEXT NOT NULL,
		description TEXT NOT NULL,
		suggestion TEXT,
		evidence INTEGER DEFAULT 0,
		FOREIGN KEY (review_id) REFERENCES reviews(review_id) ON DELETE CASCADE
	);

	-- User feedback on findings
	CREATE TABLE IF NOT EXISTS feedback (
		feedback_id INTEGER PRIMARY KEY AUTOINCREMENT,
		finding_id TEXT NOT NULL,
		status TEXT NOT NULL CHECK(status IN ('accepted', 'rejected')),
		timestamp INTEGER NOT NULL,
		FOREIGN KEY (finding_id) REFERENCES findings(finding_id) ON DELETE CASCADE
	);

	-- Precision priors using Beta distribution parameters
	CREATE TABLE IF NOT EXISTS precision_priors (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider TEXT NOT NULL,
		category TEXT NOT NULL,
		alpha REAL NOT NULL DEFAULT 1.0,
		beta REAL NOT NULL DEFAULT 1.0,
		UNIQUE(provider, category)
	);

	-- Indexes for performance
	CREATE INDEX IF NOT EXISTS idx_findings_hash ON findings(finding_hash);
	CREATE INDEX IF NOT EXISTS idx_findings_review ON findings(review_id);
	CREATE INDEX IF NOT EXISTS idx_feedback_finding ON feedback(finding_id);
	CREATE INDEX IF NOT EXISTS idx_precision_provider_category ON precision_priors(provider, category);
	CREATE INDEX IF NOT EXISTS idx_runs_timestamp ON runs(timestamp DESC);
	`

	_, err := s.db.Exec(schema)
	return err
}

// CreateRun stores a new review run.
func (s *Store) CreateRun(ctx context.Context, run store.Run) error {
	query := `
		INSERT INTO runs (run_id, timestamp, scope, config_hash, total_cost, base_ref, target_ref, repository)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		run.RunID,
		run.Timestamp.Unix(),
		run.Scope,
		run.ConfigHash,
		run.TotalCost,
		run.BaseRef,
		run.TargetRef,
		run.Repository,
	)

	if err != nil {
		return fmt.Errorf("failed to create run: %w", err)
	}

	return nil
}

// UpdateRunCost updates the total cost for a run.
func (s *Store) UpdateRunCost(ctx context.Context, runID string, totalCost float64) error {
	query := `UPDATE runs SET total_cost = ? WHERE run_id = ?`

	result, err := s.db.ExecContext(ctx, query, totalCost, runID)
	if err != nil {
		return fmt.Errorf("failed to update run cost: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("run not found: %s", runID)
	}

	return nil
}

// GetRun retrieves a run by ID.
func (s *Store) GetRun(ctx context.Context, runID string) (store.Run, error) {
	query := `
		SELECT run_id, timestamp, scope, config_hash, total_cost, base_ref, target_ref, repository
		FROM runs
		WHERE run_id = ?
	`

	var run store.Run
	var timestamp int64

	err := s.db.QueryRowContext(ctx, query, runID).Scan(
		&run.RunID,
		&timestamp,
		&run.Scope,
		&run.ConfigHash,
		&run.TotalCost,
		&run.BaseRef,
		&run.TargetRef,
		&run.Repository,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return store.Run{}, fmt.Errorf("run not found: %s", runID)
		}
		return store.Run{}, fmt.Errorf("failed to get run: %w", err)
	}

	run.Timestamp = time.Unix(timestamp, 0)
	return run, nil
}

// ListRuns retrieves the most recent runs, limited by the given count.
func (s *Store) ListRuns(ctx context.Context, limit int) ([]store.Run, error) {
	query := `
		SELECT run_id, timestamp, scope, config_hash, total_cost, base_ref, target_ref, repository
		FROM runs
		ORDER BY timestamp DESC
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var runs []store.Run
	for rows.Next() {
		var run store.Run
		var timestamp int64

		if err := rows.Scan(
			&run.RunID,
			&timestamp,
			&run.Scope,
			&run.ConfigHash,
			&run.TotalCost,
			&run.BaseRef,
			&run.TargetRef,
			&run.Repository,
		); err != nil {
			return nil, fmt.Errorf("failed to scan run: %w", err)
		}

		run.Timestamp = time.Unix(timestamp, 0)
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating runs: %w", err)
	}

	return runs, nil
}

// SaveReview stores a review record.
func (s *Store) SaveReview(ctx context.Context, review store.ReviewRecord) error {
	query := `
		INSERT INTO reviews (review_id, run_id, provider, model, summary, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		review.ReviewID,
		review.RunID,
		review.Provider,
		review.Model,
		review.Summary,
		review.CreatedAt.Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to save review: %w", err)
	}

	return nil
}

// GetReview retrieves a review by ID.
func (s *Store) GetReview(ctx context.Context, reviewID string) (store.ReviewRecord, error) {
	query := `
		SELECT review_id, run_id, provider, model, summary, created_at
		FROM reviews
		WHERE review_id = ?
	`

	var review store.ReviewRecord
	var createdAt int64

	err := s.db.QueryRowContext(ctx, query, reviewID).Scan(
		&review.ReviewID,
		&review.RunID,
		&review.Provider,
		&review.Model,
		&review.Summary,
		&createdAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return store.ReviewRecord{}, fmt.Errorf("review not found: %s", reviewID)
		}
		return store.ReviewRecord{}, fmt.Errorf("failed to get review: %w", err)
	}

	review.CreatedAt = time.Unix(createdAt, 0)
	return review, nil
}

// GetReviewsByRun retrieves all reviews for a given run.
func (s *Store) GetReviewsByRun(ctx context.Context, runID string) ([]store.ReviewRecord, error) {
	query := `
		SELECT review_id, run_id, provider, model, summary, created_at
		FROM reviews
		WHERE run_id = ?
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get reviews by run: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var reviews []store.ReviewRecord
	for rows.Next() {
		var review store.ReviewRecord
		var createdAt int64

		if err := rows.Scan(
			&review.ReviewID,
			&review.RunID,
			&review.Provider,
			&review.Model,
			&review.Summary,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan review: %w", err)
		}

		review.CreatedAt = time.Unix(createdAt, 0)
		reviews = append(reviews, review)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating reviews: %w", err)
	}

	return reviews, nil
}

// SaveFindings stores multiple findings in a single transaction.
func (s *Store) SaveFindings(ctx context.Context, findings []store.FindingRecord) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO findings (finding_id, review_id, finding_hash, file, line_start, line_end, category, severity, description, suggestion, evidence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, finding := range findings {
		evidence := 0
		if finding.Evidence {
			evidence = 1
		}

		if _, err := stmt.ExecContext(ctx,
			finding.FindingID,
			finding.ReviewID,
			finding.FindingHash,
			finding.File,
			finding.LineStart,
			finding.LineEnd,
			finding.Category,
			finding.Severity,
			finding.Description,
			finding.Suggestion,
			evidence,
		); err != nil {
			return fmt.Errorf("failed to insert finding: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetFinding retrieves a single finding by ID.
func (s *Store) GetFinding(ctx context.Context, findingID string) (store.FindingRecord, error) {
	query := `
		SELECT finding_id, review_id, finding_hash, file, line_start, line_end, category, severity, description, suggestion, evidence
		FROM findings
		WHERE finding_id = ?
	`

	var finding store.FindingRecord
	var evidence int

	err := s.db.QueryRowContext(ctx, query, findingID).Scan(
		&finding.FindingID,
		&finding.ReviewID,
		&finding.FindingHash,
		&finding.File,
		&finding.LineStart,
		&finding.LineEnd,
		&finding.Category,
		&finding.Severity,
		&finding.Description,
		&finding.Suggestion,
		&evidence,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return store.FindingRecord{}, fmt.Errorf("finding not found: %s", findingID)
		}
		return store.FindingRecord{}, fmt.Errorf("failed to get finding: %w", err)
	}

	finding.Evidence = evidence == 1
	return finding, nil
}

// GetFindingsByReview retrieves all findings for a given review.
func (s *Store) GetFindingsByReview(ctx context.Context, reviewID string) ([]store.FindingRecord, error) {
	query := `
		SELECT finding_id, review_id, finding_hash, file, line_start, line_end, category, severity, description, suggestion, evidence
		FROM findings
		WHERE review_id = ?
		ORDER BY line_start ASC
	`

	rows, err := s.db.QueryContext(ctx, query, reviewID)
	if err != nil {
		return nil, fmt.Errorf("failed to get findings by review: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var findings []store.FindingRecord
	for rows.Next() {
		var finding store.FindingRecord
		var evidence int

		if err := rows.Scan(
			&finding.FindingID,
			&finding.ReviewID,
			&finding.FindingHash,
			&finding.File,
			&finding.LineStart,
			&finding.LineEnd,
			&finding.Category,
			&finding.Severity,
			&finding.Description,
			&finding.Suggestion,
			&evidence,
		); err != nil {
			return nil, fmt.Errorf("failed to scan finding: %w", err)
		}

		finding.Evidence = evidence == 1
		findings = append(findings, finding)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating findings: %w", err)
	}

	return findings, nil
}

// RecordFeedback stores user feedback for a finding.
func (s *Store) RecordFeedback(ctx context.Context, feedback store.Feedback) error {
	query := `
		INSERT INTO feedback (finding_id, status, timestamp)
		VALUES (?, ?, ?)
	`

	result, err := s.db.ExecContext(ctx, query,
		feedback.FindingID,
		feedback.Status,
		feedback.Timestamp.Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to record feedback: %w", err)
	}

	// Get the generated feedback_id
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get feedback ID: %w", err)
	}

	feedback.FeedbackID = int(id)
	return nil
}

// GetFeedbackForFinding retrieves all feedback for a given finding.
func (s *Store) GetFeedbackForFinding(ctx context.Context, findingID string) ([]store.Feedback, error) {
	query := `
		SELECT feedback_id, finding_id, status, timestamp
		FROM feedback
		WHERE finding_id = ?
		ORDER BY timestamp DESC
	`

	rows, err := s.db.QueryContext(ctx, query, findingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get feedback for finding: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var feedbacks []store.Feedback
	for rows.Next() {
		var feedback store.Feedback
		var timestamp int64

		if err := rows.Scan(
			&feedback.FeedbackID,
			&feedback.FindingID,
			&feedback.Status,
			&timestamp,
		); err != nil {
			return nil, fmt.Errorf("failed to scan feedback: %w", err)
		}

		feedback.Timestamp = time.Unix(timestamp, 0)
		feedbacks = append(feedbacks, feedback)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating feedback: %w", err)
	}

	return feedbacks, nil
}

// GetPrecisionPriors retrieves all precision priors, organized by provider and category.
func (s *Store) GetPrecisionPriors(ctx context.Context) (map[string]map[string]store.PrecisionPrior, error) {
	query := `
		SELECT provider, category, alpha, beta
		FROM precision_priors
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get precision priors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	priors := make(map[string]map[string]store.PrecisionPrior)

	for rows.Next() {
		var prior store.PrecisionPrior

		if err := rows.Scan(
			&prior.Provider,
			&prior.Category,
			&prior.Alpha,
			&prior.Beta,
		); err != nil {
			return nil, fmt.Errorf("failed to scan precision prior: %w", err)
		}

		if priors[prior.Provider] == nil {
			priors[prior.Provider] = make(map[string]store.PrecisionPrior)
		}
		priors[prior.Provider][prior.Category] = prior
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating precision priors: %w", err)
	}

	return priors, nil
}

// UpdatePrecisionPrior updates the Beta distribution parameters for a provider/category pair.
func (s *Store) UpdatePrecisionPrior(ctx context.Context, provider, category string, accepted, rejected int) error {
	query := `
		INSERT INTO precision_priors (provider, category, alpha, beta)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(provider, category) DO UPDATE SET
			alpha = alpha + excluded.alpha - 1.0,
			beta = beta + excluded.beta - 1.0
	`

	// Start with uniform prior (1.0, 1.0) and add the counts
	alpha := 1.0 + float64(accepted)
	beta := 1.0 + float64(rejected)

	_, err := s.db.ExecContext(ctx, query, provider, category, alpha, beta)
	if err != nil {
		return fmt.Errorf("failed to update precision prior: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}
