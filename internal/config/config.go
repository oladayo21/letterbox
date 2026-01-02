package config

import (
	"encoding/hex"
	"fmt"

	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Port          string `envconfig:"PORT" default:"8080"`
	DatabaseURL   string `envconfig:"DATABASE_URL" required:"true"`
	EncryptionKey string `envconfig:"ENCRYPTION_KEY" required:"true"`
	APIKey        string `envconfig:"API_KEY" required:"true"`
	S3Endpoint    string `envconfig:"S3_ENDPOINT" required:"true"`
	S3Bucket      string `envconfig:"S3_BUCKET" required:"true"`
	S3AccessKey   string `envconfig:"S3_ACCESS_KEY" required:"true"`
	S3SecretKey   string `envconfig:"S3_SECRET_KEY" required:"true"`
	S3Region      string `envconfig:"S3_REGION" default:"auto"`
	LogLevel      string `envconfig:"LOG_LEVEL" default:"info"`
	LogFormat     string `envconfig:"LOG_FORMAT" default:"json"`

	encryptionKeyBytes []byte
}

func Load() (*Config, error) {
	var cfg Config

	if err := envconfig.Process("letterbox", &cfg); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	keyBytes, err := hex.DecodeString(cfg.EncryptionKey)

	if err != nil {
		return nil, fmt.Errorf("LETTERBOX_ENCRYPTION_KEY must be valid hex: %w", err)
	}

	if len(keyBytes) != 32 {
		return nil, fmt.Errorf("LETTERBOX_ENCRYPTION_KEY must be 64 hex chars (32 bytes), got %d bytes", len(keyBytes))
	}

	cfg.encryptionKeyBytes = keyBytes

	return &cfg, nil
}

func (c *Config) EncryptionKeyBytes() []byte {
	return c.encryptionKeyBytes
}
