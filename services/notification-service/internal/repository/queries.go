package repository

// All SQL for the notification repository lives here as named constants; the
// methods in postgres.go reference them by name. Keep the query text out of the
// method bodies so the SQL is reviewable in one place.

const (
	// qEnsureSchema reports whether both required tables exist (advisory check).
	qEnsureSchema = `
SELECT
  to_regclass('public.notifications') IS NOT NULL,
  to_regclass('public.device_tokens')  IS NOT NULL`

	// qCreate inserts a notification and returns the full row.
	qCreate = `
INSERT INTO notifications (user_id, type, title, body, data)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, user_id, type, title, body, data, read_at, created_at`

	// qCountAll / qCountUnread: total matching a List filter (for pagination).
	qCountAll    = `SELECT COUNT(*) FROM notifications WHERE user_id = $1`
	qCountUnread = qCountAll + ` AND read_at IS NULL`

	// qListAll / qListUnread: newest-first page. $2 = limit, $3 = offset.
	qListAll = `
SELECT id, user_id, type, title, body, data, read_at, created_at
FROM   notifications
WHERE  user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`
	qListUnread = `
SELECT id, user_id, type, title, body, data, read_at, created_at
FROM   notifications
WHERE  user_id = $1 AND read_at IS NULL
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`

	// qUnreadCount: badge count for a user.
	qUnreadCount = `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`

	// qMarkRead: mark one owned, unread notification read (idempotent).
	qMarkRead = `
UPDATE notifications SET read_at = now()
WHERE id = $1 AND user_id = $2 AND read_at IS NULL`

	// qMarkAllRead: mark every unread notification of a user read.
	qMarkAllRead = `
UPDATE notifications SET read_at = now()
WHERE user_id = $1 AND read_at IS NULL`

	// qSaveDeviceToken: upsert a push token, reassigning it if it already exists.
	qSaveDeviceToken = `
INSERT INTO device_tokens (token, user_id, platform)
VALUES ($1, $2, $3)
ON CONFLICT (token) DO UPDATE
  SET user_id = EXCLUDED.user_id, platform = EXCLUDED.platform, updated_at = now()`

	// qDeleteDeviceToken: remove one token for a user (logout / opt-out).
	qDeleteDeviceToken = `DELETE FROM device_tokens WHERE token = $1 AND user_id = $2`

	// qListDeviceTokens: every push token registered for a user, with platform.
	qListDeviceTokens = `SELECT token, platform FROM device_tokens WHERE user_id = $1`

	// qDeleteDeviceTokens: prune a batch of tokens FCM reported as invalid.
	qDeleteDeviceTokens = `DELETE FROM device_tokens WHERE token = ANY($1)`
)
