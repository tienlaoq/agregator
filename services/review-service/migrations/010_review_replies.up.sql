-- Owner replies to reviews. One reply per review (review_id is the primary key),
-- so a venue owner can respond publicly once and edit that response in place.
-- ON DELETE CASCADE keeps replies from outliving their review.
CREATE TABLE IF NOT EXISTS review_replies (
    review_id      UUID        PRIMARY KEY REFERENCES reviews(id) ON DELETE CASCADE,
    author_user_id UUID        NOT NULL,
    body           TEXT        NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
