package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	App     AppConfig
	DB      DBConfig
	Session SessionConfig
	SMTP    SMTPConfig
}

type AppConfig struct {
	Env  string `env:"APP_ENV" envDefault:"development"`
	Port int    `env:"APP_PORT" envDefault:"8080"`
}

type DBConfig struct {
	Host     string `env:"DB_HOST" envDefault:"localhost"`
	Port     int    `env:"DB_PORT" envDefault:"5432"`
	User     string `env:"DB_USER,required"`
	Password string `env:"DB_PASSWORD,required"`
	Name     string `env:"DB_NAME,required"`
	SSLMode  string `env:"DB_SSLMODE" envDefault:"disable"`
}

type SessionConfig struct {
	TTL time.Duration `env:"SESSION_TTL" envDefault:"24h"`
}

// SMTPConfig describes the outgoing mail relay. Nothing here is marked required
// because the app must still boot without email; email.NewSMTPSender validates it.
type SMTPConfig struct {
	Host        string        `env:"SMTP_HOST" envDefault:"localhost"`
	Port        int           `env:"SMTP_PORT" envDefault:"587"`
	Username    string        `env:"SMTP_USERNAME"`
	Password    string        `env:"SMTP_PASSWORD"`
	FromAddress string        `env:"SMTP_FROM_ADDRESS"`
	FromName    string        `env:"SMTP_FROM_NAME" envDefault:"Muni"`
	TLSMode     string        `env:"SMTP_TLS_MODE" envDefault:"starttls"` // starttls, tls or none
	Timeout     time.Duration `env:"SMTP_TIMEOUT" envDefault:"10s"`
}

// DSN returns the PostgreSQL connection string.
func (c DBConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Name, c.SSLMode)
}

// Load reads configuration from the environment, loading .env first if present.
func Load() (*Config, error) {
	_ = godotenv.Load()

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}
