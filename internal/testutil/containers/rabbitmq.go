package containers

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type RabbitMQ struct {
	URL string

	c testcontainers.Container
}

func StartRabbitMQ(ctx context.Context) (*RabbitMQ, error) {
	req := testcontainers.ContainerRequest{
		Image:        "rabbitmq:3.13.7-management",
		ExposedPorts: []string{"5672/tcp"},
		Env: map[string]string{
			"RABBITMQ_DEFAULT_USER": "guest",
			"RABBITMQ_DEFAULT_PASS": "guest",
		},
		WaitingFor: wait.ForListeningPort(nat.Port("5672/tcp")).WithStartupTimeout(90 * time.Second),
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
	p, err := c.MappedPort(ctx, nat.Port("5672/tcp"))
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}

	return &RabbitMQ{
		URL: fmt.Sprintf("amqp://guest:guest@%s:%s/", host, p.Port()),
		c:   c,
	}, nil
}

func (r *RabbitMQ) Terminate(ctx context.Context) error {
	if r == nil || r.c == nil {
		return nil
	}
	return r.c.Terminate(ctx)
}

