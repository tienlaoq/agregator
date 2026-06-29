// migrator applies every service's SQL migrations against an external Postgres
// (e.g. Yandex Managed PostgreSQL) and exits. It is the k8s-native replacement
// for deploy/migrate.sh, which only works against the docker-compose Postgres
// via `docker exec`.
//
// It runs as a Helm pre-install/pre-upgrade hook Job (deploy/helm/agregator),
// so the schema is up to date before the new service Deployments roll out. The
// migrations themselves are bundled into the image under MIGRATIONS_ROOT
// (/migrations/<service>/…); see services/migrator/Dockerfile.
//
// The databases must already exist — on managed Postgres they are created via
// the cloud console/API, not by this Job (the app role rarely has CREATEDB).
// Required databases and extensions are listed in deploy/helm/MIGRATIONS.md.
//
// Connection parameters come from the standard PG_* env (PG_HOST, PG_PORT,
// PG_USER, PG_PASSWORD, PG_SSLMODE) shared by every service; only the database
// name varies per entry below. The role must have access to all databases.
package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/tienlao/agregator/pkg/config"
	"github.com/tienlao/agregator/pkg/dbmigrate"
	"github.com/tienlao/agregator/pkg/logger"
)

// plan maps each database to the service migration directories that own its
// schema, applied in slice order. It mirrors deploy/migrate.sh. venue_db is the
// only shared database: venue-service owns 001–011 and crm-service owns 012+
// (strangler-fig phase B), so venue migrations must run before crm migrations.
var plan = []struct {
	db   string
	dirs []string
}{
	{db: "auth_db", dirs: []string{"auth-service"}},
	{db: "user_db", dirs: []string{"user-service"}},
	{db: "venue_db", dirs: []string{"venue-service", "crm-service"}},
	{db: "booking_db", dirs: []string{"booking-service"}},
	{db: "review_db", dirs: []string{"review-service"}},
	{db: "payment_db", dirs: []string{"payment-service"}},
	{db: "master_db", dirs: []string{"master-service"}},
	{db: "chat_db", dirs: []string{"chat-service"}},
	{db: "notification_db", dirs: []string{"notification-service"}},
	{db: "support_db", dirs: []string{"api-gateway"}},
	{db: "analytics_db", dirs: []string{"analytics-service"}},
}

func main() {
	log := logger.New("migrator")

	root := config.GetEnv("MIGRATIONS_ROOT", "/migrations")

	// One PostgresConfig template; DBName is overridden per plan entry. Validate
	// once up front so a missing password fails fast rather than per database.
	base := config.NewPostgresConfig("PG")
	if err := base.Validate(); err != nil {
		log.Fatal().Err(err).Msg("invalid postgres config")
	}

	ctx := context.Background()
	for _, p := range plan {
		cfg := base
		cfg.DBName = p.db
		for _, dir := range p.dirs {
			path := filepath.Join(root, dir, "migrations")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				// Fall back to flat layout (/migrations/<service>) in case the
				// image bundles the SQL without the nested migrations/ dir.
				path = filepath.Join(root, dir)
			}
			log.Info().Str("db", p.db).Str("dir", path).Msg("applying migrations")
			if err := dbmigrate.Up(ctx, cfg.DSN(), path, log); err != nil {
				log.Fatal().Err(err).Str("db", p.db).Str("dir", path).Msg("migration failed")
			}
		}
	}
	log.Info().Msg("all migrations applied")
}
