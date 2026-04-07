package config

import (
	"fmt"
	"os"
	"strconv"
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
		Password: GetEnv(prefix+"_PASSWORD", "banya_secret"),
		DBName:   GetEnv(prefix+"_DB", "banya"),
		SSLMode:  GetEnv(prefix+"_SSLMODE", "disable"),
	}
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

func NewRedisConfig() RedisConfig {
	return RedisConfig{
		Addr:     GetEnv("REDIS_ADDR", "localhost:6379"),
		Password: GetEnv("REDIS_PASSWORD", ""),
		DB:       GetEnvInt("REDIS_DB", 0),
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
