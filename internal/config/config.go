package config

import (
	"log"
	"os"
)

type Config struct {
	Port        string
	DatabaseURL string
	JWTSecret   string
	DevMode     bool
}

func Load() *Config {
	c := &Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://localhost:5432/vostros?sslmode=disable"),
		JWTSecret:   getEnv("JWT_SECRET", ""),
		DevMode:     getEnv("DEV_MODE", "") == "true",
	}
	if c.JWTSecret == "" {
		if c.DevMode {
			c.JWTSecret = "dev-secret-local-only"
			log.Println("WARNING: using dev JWT secret — set JWT_SECRET for production")
		} else {
			log.Fatal("FATAL: JWT_SECRET environment variable is required (set DEV_MODE=true for local dev)")
		}
	}
	return c
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
