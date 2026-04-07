package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
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

func (r *ReviewRepo) Create(ctx context.Context, review *domain.Review) error {
	const q = `
		INSERT INTO reviews (user_id, venue_id, rating, text, is_verified)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`
	return r.pool.QueryRow(ctx, q,
		review.UserID, review.VenueID, review.Rating, review.Text, review.IsVerified,
	).Scan(&review.ID, &review.CreatedAt)
}

func (r *ReviewRepo) GetByID(ctx context.Context, id string) (*domain.Review, error) {
	const q = `
		SELECT id, user_id, venue_id, rating, text, is_verified, created_at
		FROM reviews WHERE id = $1`
	rev := &domain.Review{}
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&rev.ID, &rev.UserID, &rev.VenueID,
		&rev.Rating, &rev.Text, &rev.IsVerified, &rev.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, pkgerr.NotFound("review not found")
	}
	if err != nil {
		return nil, err
	}
	return rev, nil
}

func (r *ReviewRepo) ListByVenue(ctx context.Context, venueID string, page, pageSize int32) ([]*domain.Review, int32, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int32
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM reviews WHERE venue_id = $1`, venueID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, venue_id, rating, text, is_verified, created_at
		FROM reviews
		WHERE venue_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, venueID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reviews []*domain.Review
	for rows.Next() {
		rev := &domain.Review{}
		if err := rows.Scan(
			&rev.ID, &rev.UserID, &rev.VenueID,
			&rev.Rating, &rev.Text, &rev.IsVerified, &rev.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		reviews = append(reviews, rev)
	}
	return reviews, total, rows.Err()
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

func (r *ReviewRepo) UpdateVenueRating(ctx context.Context, venueID string) error {
	const q = `
		INSERT INTO ratings_cache (venue_id, avg_rating, review_count, updated_at)
		SELECT
			$1,
			COALESCE(AVG(rating)::FLOAT, 0),
			COUNT(*)::INT,
			now()
		FROM reviews
		WHERE venue_id = $1
		ON CONFLICT (venue_id) DO UPDATE SET
			avg_rating   = EXCLUDED.avg_rating,
			review_count = EXCLUDED.review_count,
			updated_at   = EXCLUDED.updated_at`
	_, err := r.pool.Exec(ctx, q, venueID)
	return err
}
