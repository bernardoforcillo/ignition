package database

import (
	"errors"
	"testing"
	"time"
)

func TestFromEnv_ReadsDSNAndPool(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x")
	t.Setenv("DATABASE_MAX_OPEN_CONNS", "20")
	t.Setenv("DATABASE_MAX_IDLE_CONNS", "7")
	t.Setenv("DATABASE_CONN_MAX_LIFETIME", "15m")
	got, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	want := Config{DSN: "postgres://x", MaxOpenConns: 20, MaxIdleConns: 7, ConnMaxLifetime: 15 * time.Minute}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestFromEnv_Errors(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		is   error
	}{
		{"missing DSN", nil, ErrNoDSN},
		{"bad open conns", map[string]string{"DATABASE_URL": "x", "DATABASE_MAX_OPEN_CONNS": "many"}, nil},
		{"negative idle conns", map[string]string{"DATABASE_URL": "x", "DATABASE_MAX_IDLE_CONNS": "-1"}, nil},
		{"bad lifetime", map[string]string{"DATABASE_URL": "x", "DATABASE_CONN_MAX_LIFETIME": "soon"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			_, err := FromEnv()
			if err == nil {
				t.Fatal("want error")
			}
			if tt.is != nil && !errors.Is(err, tt.is) {
				t.Errorf("got %v, want %v", err, tt.is)
			}
		})
	}
}

func TestWithDefaults(t *testing.T) {
	tests := []struct {
		name string
		in   Config
		want Config
	}{
		{"zero gets defaults", Config{}, Config{MaxOpenConns: 10, MaxIdleConns: 5, ConnMaxLifetime: 30 * time.Minute}},
		{"idle capped at open", Config{MaxOpenConns: 2, MaxIdleConns: 9, ConnMaxLifetime: time.Minute}, Config{MaxOpenConns: 2, MaxIdleConns: 2, ConnMaxLifetime: time.Minute}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.withDefaults(); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestOpen_EmptyDSNFails(t *testing.T) {
	if _, err := Open(t.Context(), Config{}); !errors.Is(err, ErrNoDSN) {
		t.Errorf("got %v, want ErrNoDSN", err)
	}
}
