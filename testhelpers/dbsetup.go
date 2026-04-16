package testhelpers

import (
	"context"
	"fmt"
	"os/exec"
	"testing"

	"github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// DockerAvailable checks if Docker is available on the system
func DockerAvailable() bool {
	_, err := exec.LookPath("docker")
	if err != nil {
		return false
	}
	return true
}

// SkipIfNoDocker skips the test if Docker is not available
func SkipIfNoDocker(t *testing.T) {
	t.Helper()
	if !DockerAvailable() {
		t.Skip("Docker not available, skipping integration test")
	}
}

// SetupMySQLContainer starts a MySQL test container, returns DSN and cleanup function
func SetupMySQLContainer(ctx context.Context) (string, func(), error) {
	ctr, err := mysql.Run(ctx, "mysql:8.0",
		mysql.WithDatabase("testdb"),
		mysql.WithUsername("test"),
		mysql.WithPassword("test"),
	)
	if err != nil {
		return "", nil, fmt.Errorf("start mysql container: %w", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("get mysql host: %w", err)
	}
	port, err := ctr.MappedPort(ctx, "3306/tcp")
	if err != nil {
		return "", nil, fmt.Errorf("get mysql port: %w", err)
	}

	dsn := fmt.Sprintf("test:test@tcp(%s:%s)/testdb?parseTime=true", host, port.Port())

	cleanup := func() {
		_ = ctr.Terminate(ctx)
	}

	return dsn, cleanup, nil
}

// SetupPostgresContainer starts a PostgreSQL test container, returns DSN and cleanup function
func SetupPostgresContainer(ctx context.Context) (string, func(), error) {
	ctr, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
	)
	if err != nil {
		return "", nil, fmt.Errorf("start postgres container: %w", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("get postgres host: %w", err)
	}
	port, err := ctr.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return "", nil, fmt.Errorf("get postgres port: %w", err)
	}

	dsn := fmt.Sprintf("postgres://test:test@%s:%s/testdb?sslmode=disable", host, port.Port())

	cleanup := func() {
		_ = ctr.Terminate(ctx)
	}

	return dsn, cleanup, nil
}
