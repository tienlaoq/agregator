package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

func GetEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func GetEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

// GetEnvDuration reads key as a time.Duration string (e.g. "15m", "1h").
// Returns fallback on missing or unparseable values.
func GetEnvDuration(key string, fallback time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}

func GetEnvBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return fallback
}

type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

func (c PostgresConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.DBName, c.SSLMode)
}

func NewPostgresConfig(prefix string) PostgresConfig {
	return PostgresConfig{
		Host:     GetEnv(prefix+"_HOST", "localhost"),
		Port:     GetEnvInt(prefix+"_PORT", 5432),
		User:     GetEnv(prefix+"_USER", "banya"),
		Password: GetEnv(prefix+"_PASSWORD", ""),
		DBName:   GetEnv(prefix+"_DB", "banya"),
		SSLMode:  GetEnv(prefix+"_SSLMODE", "disable"),
	}
}

// Validate returns an error if the DSN would use an empty password (never safe outside controlled tests).
func (c PostgresConfig) Validate() error {
	if c.Password == "" {
		return errors.New("postgres password is empty: set PG_PASSWORD (or the matching *_PASSWORD for your service prefix)")
	}
	return nil
}

type RedisConfig struct {
	Addr         string
	Password     string
	DB           int
	PoolSize     int
	MinIdleConns int
	DialTimeout  int // seconds
	ReadTimeout  int // seconds
	WriteTimeout int // seconds
}

func NewRedisConfig() RedisConfig {
	return RedisConfig{
		Addr:         GetEnv("REDIS_ADDR", "localhost:6379"),
		Password:     GetEnv("REDIS_PASSWORD", ""),
		DB:           GetEnvInt("REDIS_DB", 0),
		PoolSize:     GetEnvInt("REDIS_POOL_SIZE", 32),
		MinIdleConns: GetEnvInt("REDIS_MIN_IDLE_CONNS", 8),
		DialTimeout:  GetEnvInt("REDIS_DIAL_TIMEOUT_SEC", 5),
		ReadTimeout:  GetEnvInt("REDIS_READ_TIMEOUT_SEC", 5),
		WriteTimeout: GetEnvInt("REDIS_WRITE_TIMEOUT_SEC", 5),
	}
}

type NATSConfig struct {
	URL string
}

func NewNATSConfig() NATSConfig {
	return NATSConfig{
		URL: GetEnv("NATS_URL", "nats://localhost:4222"),
	}
}
