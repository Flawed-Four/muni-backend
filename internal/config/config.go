package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	App     AppConfig
	Log     LogConfig
	DB      DBConfig
	Session SessionConfig
	SMTP    SMTPConfig
	Redis   RedisConfig
	Chroma  ChromaConfig
	Gemini  GeminiConfig
}

type AppConfig struct {
	Env  string `env:"APP_ENV" envDefault:"development"`
	Port int    `env:"APP_PORT" envDefault:"8080"`
	// TrustedProxies lists the proxy IPs/CIDRs whose X-Forwarded-For is believed.
	// Empty = trust no proxy, so the client IP is the TCP peer address.
	TrustedProxies []string `env:"TRUSTED_PROXIES"`
	// TrustedPlatform is a header that holds the client IP, e.g. CF-Connecting-IP.
	// It is believed from any peer, so only set it when the platform is the only way in.
	TrustedPlatform string `env:"TRUSTED_PLATFORM"`
}

// LogConfig is validated by logging.New.
type LogConfig struct {
	Level  string `env:"LOG_LEVEL" envDefault:"info"` // debug, info, warn or error
	Format string `env:"LOG_FORMAT"`                  // json or text; empty = json in production, text otherwise
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

// RedisConfig describes the Redis server behind domain.KeyValueStore.
type RedisConfig struct {
	Host     string        `env:"REDIS_HOST" envDefault:"localhost"`
	Port     int           `env:"REDIS_PORT" envDefault:"6379"`
	Username string        `env:"REDIS_USERNAME"`
	Password string        `env:"REDIS_PASSWORD"`
	DB       int           `env:"REDIS_DB" envDefault:"0"`
	TLS      bool          `env:"REDIS_TLS" envDefault:"false"`
	Timeout  time.Duration `env:"REDIS_TIMEOUT" envDefault:"3s"` // per dial, read and write
}

// ChromaConfig describes the Chroma server behind domain.VectorStore.
type ChromaConfig struct {
	Host     string        `env:"CHROMA_HOST" envDefault:"localhost"`
	Port     int           `env:"CHROMA_PORT" envDefault:"8000"`
	TLS      bool          `env:"CHROMA_TLS" envDefault:"false"`
	APIKey   string        `env:"CHROMA_API_KEY"` // Chroma Cloud, sent as x-chroma-token
	Tenant   string        `env:"CHROMA_TENANT" envDefault:"default_tenant"`
	Database string        `env:"CHROMA_DATABASE" envDefault:"default_database"`
	Timeout  time.Duration `env:"CHROMA_TIMEOUT" envDefault:"10s"` // per request
}

// GeminiConfig describes the Gemini API behind domain.LanguageModel.
// Nothing is required so the app boots without a key; llm.NewGemini validates it.
type GeminiConfig struct {
	APIKey        string        `env:"GEMINI_API_KEY"`
	Model         string        `env:"GEMINI_MODEL" envDefault:"gemini-3.5-flash-lite"`
	ThinkingLevel string        `env:"GEMINI_THINKING_LEVEL" envDefault:"low"`
	Timeout       time.Duration `env:"GEMINI_TIMEOUT" envDefault:"120s"` // per HTTP attempt
	MaxRetries    int           `env:"GEMINI_MAX_RETRIES" envDefault:"2"`
	BaseURL       string        `env:"GEMINI_BASE_URL"` // empty = Google's endpoint
}

// DSN returns the PostgreSQL connection string. Every part is escaped, so a
// password like "p@ss/w#rd" stays one password instead of breaking the URL.
func (c DBConfig) DSN() string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(c.User, c.Password),
		Host:     net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		Path:     "/" + c.Name,
		RawQuery: url.Values{"sslmode": {c.SSLMode}}.Encode(),
	}
	return u.String()
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
