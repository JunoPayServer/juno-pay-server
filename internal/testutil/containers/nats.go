package containers

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type NATS struct {
	URL string

	c testcontainers.Container
}

func StartNATS(ctx context.Context) (*NATS, error) {
	req := testcontainers.ContainerRequest{
		Image:        "nats:2.10.22",
		ExposedPorts: []string{"4222/tcp"},
		WaitingFor:   wait.ForListeningPort(nat.Port("4222/tcp")).WithStartupTimeout(60 * time.Second),
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
	p, err := c.MappedPort(ctx, nat.Port("4222/tcp"))
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}

	return &NATS{
		URL: fmt.Sprintf("nats://%s:%s", host, p.Port()),
		c:   c,
	}, nil
}

func (n *NATS) Terminate(ctx context.Context) error {
	if n == nil || n.c == nil {
		return nil
	}
	return n.c.Terminate(ctx)
}

