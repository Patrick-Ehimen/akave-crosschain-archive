package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the top-level application configuration.
type Config struct {
	Database Database         `mapstructure:"database"`
	Chains   map[uint64]Chain `mapstructure:"chains"`
	Akave    Akave            `mapstructure:"akave"`
	Indexer  Indexer          `mapstructure:"indexer"`
	Logging  Logging          `mapstructure:"logging"`
}

// Database holds PostgreSQL connection parameters.
type Database struct {
	Host         string `mapstructure:"host"`
	Port         int    `mapstructure:"port"`
	User         string `mapstructure:"user"`
	Password     string `mapstructure:"password"`
	DBName       string `mapstructure:"dbname"`
	SSLMode      string `mapstructure:"sslmode"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}

// DSN returns the PostgreSQL connection string.
func (d Database) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, d.SSLMode,
	)
}

// Chain holds configuration for a single EVM chain.
type Chain struct {
	Name              string `mapstructure:"name"`
	RPCURL            string `mapstructure:"rpc_url"`
	ConfirmationDepth uint64 `mapstructure:"confirmation_depth"`
	MaxBlockRange     uint64 `mapstructure:"max_block_range"`
	RateLimit         int    `mapstructure:"rate_limit"`
}

// Akave holds Akave O3 storage configuration.
type Akave struct {
	Endpoint   string `mapstructure:"endpoint"`
	AccessKey  string `mapstructure:"access_key"`
	SecretKey  string `mapstructure:"secret_key"`
	BucketName string `mapstructure:"bucket_name"`
	UseSSL     bool   `mapstructure:"use_ssl"`
	Region     string `mapstructure:"region"`
}

// Indexer holds indexer service settings.
type Indexer struct {
	BatchSize       int           `mapstructure:"batch_size"`
	PollInterval    time.Duration `mapstructure:"poll_interval"`
	ArchiveInterval time.Duration `mapstructure:"archive_interval"`
}

// Logging holds logging configuration.
type Logging struct {
	Level  string `mapstructure:"level"`
	Pretty bool   `mapstructure:"pretty"`
}

// Load reads configuration from a YAML file and environment variable overrides.
// Environment variables use the CROSSCHAIN_ prefix with underscores replacing dots.
// For example, CROSSCHAIN_DATABASE_HOST overrides database.host.
func Load(path string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(path)
	v.SetEnvPrefix("CROSSCHAIN")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) validate() error {
	if c.Database.Host == "" {
		return fmt.Errorf("database.host is required")
	}
	if c.Database.DBName == "" {
		return fmt.Errorf("database.dbname is required")
	}
	if len(c.Chains) == 0 {
		return fmt.Errorf("at least one chain must be configured")
	}
	for id, chain := range c.Chains {
		if chain.RPCURL == "" {
			return fmt.Errorf("chains.%d.rpc_url is required", id)
		}
	}
	return nil
}
