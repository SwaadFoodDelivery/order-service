package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const MaxRPCTimeout = 2 * time.Second

type Config struct {
	App      struct{ Name, Env, Port, LogLevel string }
	Postgres struct {
		Host, Port, User, Password, DBName, SSLMode    string
		MaxOpenConns, MaxIdleConns, ConnMaxLifetimeMin int
	}
	// Retained for the unused legacy infrastructure packages, not startup dependencies.
	Redis struct {
		Addr, Password              string
		DB, PoolSize, ReadTimeoutMS int
	}
	Kafka struct {
		Brokers                 []string
		ClientID                string
		RequiredAcks, BatchSize int
		OrderEventsTopic        string
	}
	JWT       struct{ Secret string }
	RateLimit struct{ DefaultPerMin, WindowSec int }
	GRPC      struct {
		OrderAddr      string
		ServiceKey     string
		RequestTimeout time.Duration
	}
}

func (c Config) RateWindow() time.Duration { return time.Duration(c.RateLimit.WindowSec) * time.Second }

// Load deliberately does not auto-load .env.development: plaintext transport
// requires an explicit environment selection by the operator.
func Load() (*Config, error) {
	v := viper.New()
	v.AutomaticEnv()
	v.SetDefault("LOG_LEVEL", "info")
	v.SetDefault("POSTGRES_PORT", "5432")
	v.SetDefault("POSTGRES_SSLMODE", "require")
	v.SetDefault("POSTGRES_MAX_OPEN_CONNS", 10)
	v.SetDefault("POSTGRES_MAX_IDLE_CONNS", 5)
	v.SetDefault("POSTGRES_CONN_MAX_LIFETIME_MIN", 5)
	v.SetDefault("ORDER_GRPC_TIMEOUT_MS", 2000)
	c := &Config{}
	c.App.Name = v.GetString("APP_NAME")
	c.App.Env = v.GetString("APP_ENV")
	c.App.LogLevel = v.GetString("LOG_LEVEL")
	c.Postgres.Host = v.GetString("POSTGRES_HOST")
	c.Postgres.Port = v.GetString("POSTGRES_PORT")
	c.Postgres.User = v.GetString("POSTGRES_USER")
	c.Postgres.Password = v.GetString("POSTGRES_PASSWORD")
	c.Postgres.DBName = v.GetString("POSTGRES_DB")
	c.Postgres.SSLMode = v.GetString("POSTGRES_SSLMODE")
	c.Postgres.MaxOpenConns = v.GetInt("POSTGRES_MAX_OPEN_CONNS")
	c.Postgres.MaxIdleConns = v.GetInt("POSTGRES_MAX_IDLE_CONNS")
	c.Postgres.ConnMaxLifetimeMin = v.GetInt("POSTGRES_CONN_MAX_LIFETIME_MIN")
	c.GRPC.OrderAddr = v.GetString("ORDER_GRPC_ADDR")
	c.GRPC.ServiceKey = v.GetString("ORDER_GRPC_SERVICE_KEY")
	timeoutMS, err := strconv.ParseInt(v.GetString("ORDER_GRPC_TIMEOUT_MS"), 10, 64)
	if err != nil || timeoutMS < 1 || timeoutMS > MaxRPCTimeout.Milliseconds() {
		return nil, fmt.Errorf("ORDER_GRPC_TIMEOUT_MS must be between 1 and 2000")
	}
	c.GRPC.RequestTimeout = time.Duration(timeoutMS) * time.Millisecond
	if err := c.ValidateRPC(); err != nil {
		return nil, err
	}
	if c.Postgres.Host == "" || c.Postgres.User == "" || c.Postgres.DBName == "" {
		return nil, fmt.Errorf("POSTGRES_HOST, POSTGRES_USER, and POSTGRES_DB are required")
	}
	if c.Postgres.MaxOpenConns < 1 || c.Postgres.MaxIdleConns < 0 ||
		c.Postgres.MaxIdleConns > c.Postgres.MaxOpenConns || c.Postgres.ConnMaxLifetimeMin < 1 {
		return nil, fmt.Errorf("invalid Postgres pool limits")
	}
	return c, nil
}

func (c *Config) ValidateRPC() error {
	if c == nil {
		return fmt.Errorf("configuration is required")
	}
	if c.App.Env != "development" && c.App.Env != "test" {
		return fmt.Errorf("plaintext order RPC is restricted to explicit development/test; TLS is not implemented")
	}
	host, port, err := net.SplitHostPort(c.GRPC.OrderAddr)
	ip := net.ParseIP(host)
	p, portErr := strconv.Atoi(port)
	if err != nil || ip == nil || !ip.IsLoopback() || portErr != nil || p < 1 || p > 65535 {
		return fmt.Errorf("ORDER_GRPC_ADDR must be an explicit numeric loopback address and port")
	}
	if len(c.GRPC.ServiceKey) < 32 || strings.TrimSpace(c.GRPC.ServiceKey) != c.GRPC.ServiceKey {
		return fmt.Errorf("ORDER_GRPC_SERVICE_KEY must contain at least 32 bytes without surrounding whitespace")
	}
	for _, b := range []byte(c.GRPC.ServiceKey) {
		if b < 33 || b > 126 {
			return fmt.Errorf("ORDER_GRPC_SERVICE_KEY must contain printable ASCII without spaces")
		}
	}
	if c.GRPC.RequestTimeout <= 0 || c.GRPC.RequestTimeout > MaxRPCTimeout {
		return fmt.Errorf("order RPC timeout must be positive and at most two seconds")
	}
	return nil
}
