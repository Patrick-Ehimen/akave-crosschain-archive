package config

import (
	"os"
	"path/filepath"
	"testing"
)

const testConfig = `
database:
  host: localhost
  port: 5432
  user: testuser
  password: testpass
  dbname: testdb
  sslmode: disable
  max_open_conns: 10
  max_idle_conns: 2

chains:
  1:
    name: ethereum
    rpc_url: https://eth.example.com
    confirmation_depth: 12
    max_block_range: 1000
    rate_limit: 10
  42161:
    name: arbitrum
    rpc_url: https://arb.example.com
    confirmation_depth: 1
    max_block_range: 2000
    rate_limit: 20

akave:
  endpoint: o3.example.com
  access_key: testkey
  secret_key: testsecret
  bucket_name: test-bucket
  use_ssl: true
  region: test-region

indexer:
  batch_size: 500
  poll_interval: 10s
  archive_interval: 30m

logging:
  level: debug
  pretty: true
`

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	path := writeTestConfig(t, testConfig)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Database.Host != "localhost" {
		t.Errorf("expected host localhost, got %s", cfg.Database.Host)
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("expected port 5432, got %d", cfg.Database.Port)
	}
	if cfg.Database.DBName != "testdb" {
		t.Errorf("expected dbname testdb, got %s", cfg.Database.DBName)
	}

	if len(cfg.Chains) != 2 {
		t.Fatalf("expected 2 chains, got %d", len(cfg.Chains))
	}
	eth := cfg.Chains[1]
	if eth.Name != "ethereum" {
		t.Errorf("expected chain name ethereum, got %s", eth.Name)
	}
	if eth.ConfirmationDepth != 12 {
		t.Errorf("expected confirmation depth 12, got %d", eth.ConfirmationDepth)
	}

	arb := cfg.Chains[42161]
	if arb.RPCURL != "https://arb.example.com" {
		t.Errorf("expected arb rpc url, got %s", arb.RPCURL)
	}

	if cfg.Akave.BucketName != "test-bucket" {
		t.Errorf("expected bucket test-bucket, got %s", cfg.Akave.BucketName)
	}
	if cfg.Indexer.BatchSize != 500 {
		t.Errorf("expected batch size 500, got %d", cfg.Indexer.BatchSize)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Logging.Level)
	}
}

func TestLoadDSN(t *testing.T) {
	path := writeTestConfig(t, testConfig)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "postgres://testuser:testpass@localhost:5432/testdb?sslmode=disable"
	if cfg.Database.DSN() != expected {
		t.Errorf("expected DSN %s, got %s", expected, cfg.Database.DSN())
	}
}

func TestLoadEnvOverride(t *testing.T) {
	path := writeTestConfig(t, testConfig)

	t.Setenv("CROSSCHAIN_DATABASE_HOST", "remotehost")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Database.Host != "remotehost" {
		t.Errorf("expected env override to remotehost, got %s", cfg.Database.Host)
	}
}

func TestLoadValidationMissingHost(t *testing.T) {
	config := `
database:
  host: ""
  port: 5432
  dbname: testdb
chains:
  1:
    rpc_url: https://eth.example.com
`
	path := writeTestConfig(t, config)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for missing host")
	}
}

func TestLoadValidationNoChains(t *testing.T) {
	config := `
database:
  host: localhost
  dbname: testdb
chains: {}
`
	path := writeTestConfig(t, config)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for no chains")
	}
}

func TestLoadValidationMissingRPCURL(t *testing.T) {
	config := `
database:
  host: localhost
  dbname: testdb
chains:
  1:
    name: ethereum
    rpc_url: ""
`
	path := writeTestConfig(t, config)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected validation error for missing rpc_url")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}
