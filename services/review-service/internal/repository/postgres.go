package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	pkgerr "github.com/tienlao/agregator/pkg/errors"
	"github.com/tienlao/agregator/services/review-service/internal/domain"
)

type ReviewRepo struct {
	pool *pgxpool.Pool
}

func NewReviewRepo(pool *pgxpool.Pool) *ReviewRepo {
	return &ReviewRepo{pool: pool}
}

// BeginTx starts a transaction so the usecase can wrap review + outbox
// inserts in a single atomic commit.
func (r *ReviewRepo) BeginTx(ctx context.Context) (pgx.Tx, error) {
	return r.pool.Begin(ctx)
}

const insertReviewSQL = `
	INSERT INTO reviews (user_id, user_name, venue_id, master_id, rating, text, is_verified, is_anonymous)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	RETURNING id, created_at`

// CreateTx inserts a review within the provided transaction.
func (r *ReviewRepo) CreateTx(ctx context.Context, tx pgx.Tx, review *domain.Review) error {
	err := tx.QueryRow(ctx, insertReviewSQL,
		review.UserID, review.UserName, nilIfEmpty(review.VenueID), nilIfEmpty(review.MasterID),
		review.Rating, review.Text, review.IsVerified, review.IsAnonymous,
	).Scan(&review.ID, &review.CreatedAt)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return pkgerr.AlreadyExists("review already exists for this target")
	}
	return err
}

func (r *ReviewRepo) GetByID(ctx context.Context, id string) (*domain.Review, error) {
	const q = `SELECT ` + reviewSelectCols + ` FROM ` + reviewFromJoin + ` WHERE r.id = $1`
	rev := &domain.Review{}
	if err := scanReview(r.pool.QueryRow(ctx, q, id), rev); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pkgerr.NotFound("review not found")
		}
		return nil, err
	}
	return rev, nil
}

func (r *ReviewRepo) ListByVenue(ctx context.Context, venueID string, onlyUnanswered bool, page, pageSize int32) ([]*domain.Review, int32, error) {
	offset := (page - 1) * pageSize
	// COUNT(*) OVER () gives the total matching rows in the same snapshot as the
	// page data — no separate COUNT query, no phantom-read window between two
	// round-trips.
	filter := ""
	if onlyUnanswered {
		// Anti-join scan of the venue's reviews; bounded here by LIMIT/OFFSET.
		// See docs/TECH_DEBT.md [REVIEW-UNANSWERED-ANTIJOIN].
		filter = ` AND rr.review_id IS NULL`
	}
	rows, err := r.pool.Query(ctx,
		`SELECT `+reviewSelectColsWithTotal+
			` FROM `+reviewFromJoin+
			` WHERE r.venue_id = $1`+filter+
			` ORDER BY r.created_at DESC LIMIT $2 OFFSET $3`,
		venueID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	return collectReviewsWithTotal(rows)
}

func (r *ReviewRepo) ListByMaster(ctx context.Context, masterID string, page, pageSize int32) ([]*domain.Review, int32, error) {
	offset := (page - 1) * pageSize
	rows, err := r.pool.Query(ctx,
		`SELECT `+reviewSelectColsWithTotal+
			` FROM `+reviewFromJoin+
			` WHERE r.master_id = $1 ORDER BY r.created_at DESC LIMIT $2 OFFSET $3`,
		masterID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	return collectReviewsWithTotal(rows)
}

// GetVenueReviewSummary returns avg/count from the O(1) ratings cache plus a
// live count of reviews still lacking an owner reply, in a single round trip.
// The unanswered count is an O(reviews_per_venue) anti-join, not a cached
// counter — see docs/TECH_DEBT.md [REVIEW-UNANSWERED-ANTIJOIN] for the O(1)
// upgrade path when large venues appear.
func (r *ReviewRepo) GetVenueReviewSummary(ctx context.Context, venueID string) (*domain.VenueReviewSummary, error) {
	const q = `
		SELECT
			COALESCE(rc.avg_rating, 0),
			COALESCE(rc.review_count, 0),
			(SELECT COUNT(*) FROM reviews r
			   LEFT JOIN review_replies rr ON rr.review_id = r.id
			  WHERE r.venue_id = $1 AND rr.review_id IS NULL)
		FROM (SELECT $1::uuid AS vid) x
		LEFT JOIN ratings_cache rc ON rc.venue_id = x.vid`
	s := &domain.VenueReviewSummary{VenueID: venueID}
	if err := r.pool.QueryRow(ctx, q, venueID).Scan(
		&s.AvgRating, &s.ReviewCount, &s.UnansweredCount,
	); err != nil {
		return nil, err
	}
	return s, nil
}

const upsertReplySQL = `
	INSERT INTO review_replies (review_id, author_user_id, body)
	SELECT $1, $3, $4
	FROM reviews WHERE id = $1 AND venue_id = $2
	ON CONFLICT (review_id) DO UPDATE SET
		body           = EXCLUDED.body,
		author_user_id = EXCLUDED.author_user_id,
		updated_at     = now()
	RETURNING body, author_user_id::text, created_at, updated_at`

// UpsertReply inserts or replaces the owner reply. The SELECT gate ensures the
// review exists AND belongs to venueID; when it doesn't, no row is inserted and
// RETURNING yields no row, which we surface as NotFound.
func (r *ReviewRepo) UpsertReply(ctx context.Context, reviewID, venueID, authorUserID, body string) (*domain.ReviewReply, error) {
	reply := &domain.ReviewReply{}
	err := r.pool.QueryRow(ctx, upsertReplySQL, reviewID, venueID, authorUserID, body).Scan(
		&reply.Body, &reply.AuthorUserID, &reply.CreatedAt, &reply.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("review not found for this venue")
	}
	if err != nil {
		return nil, err
	}
	return reply, nil
}

const deleteReplySQL = `
	DELETE FROM review_replies
	WHERE review_id = $1
	  AND EXISTS (SELECT 1 FROM reviews WHERE id = $1 AND venue_id = $2)`

func (r *ReviewRepo) DeleteReply(ctx context.Context, reviewID, venueID string) error {
	tag, err := r.pool.Exec(ctx, deleteReplySQL, reviewID, venueID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pkgerr.NotFound("reply not found for this venue")
	}
	return nil
}

func (r *ReviewRepo) GetVenueRating(ctx context.Context, venueID string) (*domain.VenueRating, error) {
	const q = `SELECT venue_id, avg_rating, review_count, updated_at FROM ratings_cache WHERE venue_id = $1`
	vr := &domain.VenueRating{}
	err := r.pool.QueryRow(ctx, q, venueID).Scan(
		&vr.VenueID, &vr.AvgRating, &vr.ReviewCount, &vr.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return &domain.VenueRating{VenueID: venueID}, nil
	}
	if err != nil {
		return nil, err
	}
	return vr, nil
}

// UpdateVenueRatingTx increments the ratings_cache row for venueID by rating
// in O(1) within the provided transaction — no full-table aggregate scan.
// Executing inside the review transaction makes the cache update atomic with
// the review insert: either both commit or both roll back.
func (r *ReviewRepo) UpdateVenueRatingTx(ctx context.Context, tx pgx.Tx, venueID string, rating int32) error {
	const q = `
		INSERT INTO ratings_cache (venue_id, sum_rating, avg_rating, review_count, updated_at)
		VALUES ($1, $2, $2::float, 1, now())
		ON CONFLICT (venue_id) DO UPDATE SET
			sum_rating   = ratings_cache.sum_rating   + EXCLUDED.sum_rating,
			review_count = ratings_cache.review_count + 1,
			avg_rating   = (ratings_cache.sum_rating  + EXCLUDED.sum_rating)::float
			               / (ratings_cache.review_count + 1),
			updated_at   = now()`
	_, err := tx.Exec(ctx, q, venueID, rating)
	return err
}

func (r *ReviewRepo) GetMasterRating(ctx context.Context, masterID string) (*domain.MasterRating, error) {
	const q = `SELECT master_id, avg_rating, review_count, updated_at FROM master_ratings_cache WHERE master_id = $1`
	mr := &domain.MasterRating{}
	err := r.pool.QueryRow(ctx, q, masterID).Scan(
		&mr.MasterID, &mr.AvgRating, &mr.ReviewCount, &mr.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return &domain.MasterRating{MasterID: masterID}, nil
	}
	if err != nil {
		return nil, err
	}
	return mr, nil
}

// UpdateMasterRatingTx increments the master_ratings_cache row for masterID by rating
// in O(1) within the provided transaction — no full-table aggregate scan.
// Executing inside the review transaction makes the cache update atomic with
// the review insert: either both commit or both roll back.
func (r *ReviewRepo) UpdateMasterRatingTx(ctx context.Context, tx pgx.Tx, masterID string, rating int32) error {
	const q = `
		INSERT INTO master_ratings_cache (master_id, sum_rating, avg_rating, review_count, updated_at)
		VALUES ($1, $2, $2::float, 1, now())
		ON CONFLICT (master_id) DO UPDATE SET
			sum_rating   = master_ratings_cache.sum_rating   + EXCLUDED.sum_rating,
			review_count = master_ratings_cache.review_count + 1,
			avg_rating   = (master_ratings_cache.sum_rating  + EXCLUDED.sum_rating)::float
			               / (master_ratings_cache.review_count + 1),
			updated_at   = now()`
	_, err := tx.Exec(ctx, q, masterID, rating)
	return err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// reviewFromJoin is the FROM clause shared by every review read: reviews left
// joined to their owner reply. All columns are qualified (r./rr.) to match.
const reviewFromJoin = `reviews r LEFT JOIN review_replies rr ON rr.review_id = r.id`

// reviewSelectCols is the canonical column list (14 columns): 10 review fields
// followed by the 4 reply columns, which are NULL when no reply exists.
// COALESCE maps NULL UUID columns to empty string to match domain.Review fields.
const reviewSelectCols = `r.id, r.user_id, r.user_name,` +
	` COALESCE(r.venue_id::text, ''), COALESCE(r.master_id::text, ''),` +
	` r.rating, r.text, r.is_verified, r.is_anonymous, r.created_at,` +
	` rr.body, rr.author_user_id::text, rr.created_at, rr.updated_at`

// reviewSelectColsWithTotal appends a window-function total so that list queries
// return the filtered row count in the same snapshot as the page data.
const reviewSelectColsWithTotal = reviewSelectCols + `, COUNT(*) OVER () AS total`

// replyHolder collects the nullable reply columns so a review row can be scanned
// whether or not the owner has replied.
type replyHolder struct {
	body      sql.NullString
	author    sql.NullString
	createdAt sql.NullTime
	updatedAt sql.NullTime
}

// toReply returns the domain reply, or nil when the LEFT JOIN produced no reply.
func (h replyHolder) toReply() *domain.ReviewReply {
	if !h.body.Valid {
		return nil
	}
	return &domain.ReviewReply{
		Body:         h.body.String,
		AuthorUserID: h.author.String,
		CreatedAt:    h.createdAt.Time,
		UpdatedAt:    h.updatedAt.Time,
	}
}

// scanReview scans a single review row (14 columns) into rev, including any reply.
func scanReview(row pgx.Row, rev *domain.Review) error {
	var h replyHolder
	if err := row.Scan(
		&rev.ID, &rev.UserID, &rev.UserName, &rev.VenueID, &rev.MasterID,
		&rev.Rating, &rev.Text, &rev.IsVerified, &rev.IsAnonymous, &rev.CreatedAt,
		&h.body, &h.author, &h.createdAt, &h.updatedAt,
	); err != nil {
		return err
	}
	rev.Reply = h.toReply()
	return nil
}

// collectReviewsWithTotal drains pgx.Rows produced by a query that selects
// reviewSelectColsWithTotal (15 columns). It returns the page slice and the
// total filtered count from the window function. If no rows match, total is 0.
func collectReviewsWithTotal(rows pgx.Rows) ([]*domain.Review, int32, error) {
	defer rows.Close()
	var (
		reviews []*domain.Review
		total   int32
	)
	for rows.Next() {
		rev := &domain.Review{}
		var h replyHolder
		if err := rows.Scan(
			&rev.ID, &rev.UserID, &rev.UserName, &rev.VenueID, &rev.MasterID,
			&rev.Rating, &rev.Text, &rev.IsVerified, &rev.IsAnonymous, &rev.CreatedAt,
			&h.body, &h.author, &h.createdAt, &h.updatedAt,
			&total,
		); err != nil {
			return nil, 0, err
		}
		rev.Reply = h.toReply()
		reviews = append(reviews, rev)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return reviews, total, nil
}

// nilIfEmpty returns nil for an empty string and the string itself otherwise,
// so that optional UUID text columns (venue_id, master_id) are stored as SQL NULL
// rather than an empty string.
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
