package containers

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type Postgres struct {
	DSN string

	c testcontainers.Container
}

func StartPostgres(ctx context.Context) (*Postgres, error) {
	user := "postgres"
	pass := "postgres"
	db := "juno_pay"

	req := testcontainers.ContainerRequest{
		Image:         "postgres:16.4-alpine",
		ImagePlatform: "linux/amd64",
		Env: map[string]string{
			"POSTGRES_USER":     user,
			"POSTGRES_PASSWORD": pass,
			"POSTGRES_DB":       db,
		},
		ExposedPorts: []string{"5432/tcp"},
		WaitingFor:   wait.ForListeningPort(nat.Port("5432/tcp")).WithStartupTimeout(90 * time.Second),
	}

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	if err != nil {
		return nil, err
	}

	host, err := c.Host(ctx)
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}
	port, err := c.MappedPort(ctx, nat.Port("5432/tcp"))
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", user, pass, host, port.Port(), db)
	return &Postgres{DSN: dsn, c: c}, nil
}

func (p *Postgres) Terminate(ctx context.Context) error {
	if p == nil || p.c == nil {
		return nil
	}
	return p.c.Terminate(ctx)
}
