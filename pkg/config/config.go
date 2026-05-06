package config

import (
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App struct {
		Name     string
		Env      string
		Port     string
		LogLevel string
	}
	Postgres struct {
		Host, Port, User, Password, DBName, SSLMode    string
		MaxOpenConns, MaxIdleConns, ConnMaxLifetimeMin int
	}
	Redis struct {
		Addr, Password              string
		DB, PoolSize, ReadTimeoutMS int
	}
	Kafka struct {
		Brokers          []string
		ClientID         string
		RequiredAcks     int
		BatchSize        int
		OrderEventsTopic string
	}
	JWT struct {
		Secret string
	}
	GRPC struct {
		OrderAddr string
	}
	RateLimit struct {
		DefaultPerMin int
		WindowSec     int
	}
}

func (c Config) RateWindow() time.Duration { return time.Duration(c.RateLimit.WindowSec) * time.Second }

func Load() (*Config, error) {
	viper.SetConfigFile(".env.development")
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	_ = viper.ReadInConfig()
	cfg := &Config{}
	cfg.App.Name = viper.GetString("APP_NAME")
	cfg.App.Env = viper.GetString("APP_ENV")
	cfg.App.Port = viper.GetString("APP_PORT")
	cfg.App.LogLevel = viper.GetString("LOG_LEVEL")
	cfg.Postgres.Host = viper.GetString("POSTGRES_HOST")
	cfg.Postgres.Port = viper.GetString("POSTGRES_PORT")
	cfg.Postgres.User = viper.GetString("POSTGRES_USER")
	cfg.Postgres.Password = viper.GetString("POSTGRES_PASSWORD")
	cfg.Postgres.DBName = viper.GetString("POSTGRES_DB")
	cfg.Postgres.SSLMode = viper.GetString("POSTGRES_SSLMODE")
	cfg.Postgres.MaxOpenConns = viper.GetInt("POSTGRES_MAX_OPEN_CONNS")
	cfg.Postgres.MaxIdleConns = viper.GetInt("POSTGRES_MAX_IDLE_CONNS")
	cfg.Postgres.ConnMaxLifetimeMin = viper.GetInt("POSTGRES_CONN_MAX_LIFETIME_MIN")
	cfg.Redis.Addr = viper.GetString("REDIS_ADDR")
	cfg.Redis.Password = viper.GetString("REDIS_PASSWORD")
	cfg.Redis.DB = viper.GetInt("REDIS_DB")
	cfg.Redis.PoolSize = viper.GetInt("REDIS_POOL_SIZE")
	cfg.Redis.ReadTimeoutMS = viper.GetInt("REDIS_READ_TIMEOUT_MS")
	cfg.Kafka.Brokers = strings.Split(viper.GetString("KAFKA_BROKERS"), ",")
	cfg.Kafka.ClientID = viper.GetString("KAFKA_CLIENT_ID")
	cfg.Kafka.RequiredAcks = viper.GetInt("KAFKA_REQUIRED_ACKS")
	cfg.Kafka.BatchSize = viper.GetInt("KAFKA_BATCH_SIZE")
	cfg.Kafka.OrderEventsTopic = viper.GetString("KAFKA_ORDER_EVENTS_TOPIC")
	cfg.JWT.Secret = viper.GetString("JWT_SECRET")
	cfg.GRPC.OrderAddr = viper.GetString("ORDER_GRPC_ADDR_UNUSED")
	cfg.RateLimit.DefaultPerMin = viper.GetInt("RATE_LIMIT_DEFAULT_PER_MIN")
	cfg.RateLimit.WindowSec = viper.GetInt("RATE_LIMIT_WINDOW_SEC")
	if cfg.App.Port == "" {
		cfg.App.Port = "8080"
	}
	if cfg.GRPC.OrderAddr == "" {
		cfg.GRPC.OrderAddr = "localhost:50051"
	}
	if cfg.RateLimit.DefaultPerMin == 0 {
		cfg.RateLimit.DefaultPerMin = 60
	}
	if cfg.RateLimit.WindowSec == 0 {
		cfg.RateLimit.WindowSec = 60
	}
	return cfg, nil
}
