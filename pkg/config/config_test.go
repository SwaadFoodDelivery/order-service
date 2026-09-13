package config

import (
	"strings"
	"testing"
	"time"
)

func validConfig() *Config {
	c := &Config{}
	c.App.Env = "test"
	c.GRPC.OrderAddr = "127.0.0.1:50051"
	c.GRPC.ServiceKey = strings.Repeat("k", 32)
	c.GRPC.RequestTimeout = 2 * time.Second
	return c
}

func TestPlaintextFailsClosed(t *testing.T) {
	for _, env := range []string{"", "production", "staging", "Development"} {
		c := validConfig()
		c.App.Env = env
		if c.ValidateRPC() == nil {
			t.Errorf("accepted environment %q", env)
		}
	}
	for _, address := range []string{"", ":50051", "0.0.0.0:50051", "[::]:50051", "10.0.0.1:50051", "localhost:50051", "127.0.0.1:0", "127.0.0.1:65536"} {
		c := validConfig()
		c.GRPC.OrderAddr = address
		if c.ValidateRPC() == nil {
			t.Errorf("accepted bind address %q", address)
		}
	}
	for _, address := range []string{"127.0.0.1:50051", "[::1]:50051"} {
		c := validConfig()
		c.GRPC.OrderAddr = address
		if err := c.ValidateRPC(); err != nil {
			t.Errorf("rejected loopback: %v", err)
		}
	}
}

func TestCredentialAndTimeoutBounds(t *testing.T) {
	for _, key := range []string{"", strings.Repeat("k", 31), strings.Repeat("k", 32) + "\n", strings.Repeat("k", 32) + " other"} {
		c := validConfig()
		c.GRPC.ServiceKey = key
		if c.ValidateRPC() == nil {
			t.Error("accepted invalid service key")
		}
	}
	for _, timeout := range []time.Duration{0, -time.Second, MaxRPCTimeout + time.Millisecond} {
		c := validConfig()
		c.GRPC.RequestTimeout = timeout
		if c.ValidateRPC() == nil {
			t.Errorf("accepted timeout %v", timeout)
		}
	}
}

func TestLoadRequiresExplicitEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("ORDER_GRPC_ADDR", "127.0.0.1:50051")
	t.Setenv("ORDER_GRPC_SERVICE_KEY", strings.Repeat("k", 32))
	if _, err := Load(); err == nil {
		t.Fatal("implicit development environment was accepted")
	}
	t.Setenv("APP_ENV", "development")
	t.Setenv("POSTGRES_HOST", "127.0.0.1")
	t.Setenv("POSTGRES_USER", "reader")
	t.Setenv("POSTGRES_DB", "swaad_unit_test")
	t.Setenv("ORDER_GRPC_TIMEOUT_MS", "2000")
	c, err := Load()
	if err != nil || c.GRPC.RequestTimeout != 2*time.Second {
		t.Fatalf("load = %v, %v", c != nil, err)
	}
	for _, timeout := range []string{"0", "-1", "2001", "nonsense", "9223372036854775807"} {
		t.Setenv("ORDER_GRPC_TIMEOUT_MS", timeout)
		if _, err := Load(); err == nil {
			t.Errorf("accepted timeout %s", timeout)
		}
	}
}
