package containers

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type MySQL struct {
	DSN string

	c testcontainers.Container
}

func StartMySQL(ctx context.Context) (*MySQL, error) {
	user := "juno"
	pass := "juno"
	db := "juno_pay"

	req := testcontainers.ContainerRequest{
		Image:         "mysql:8.4.3",
		ImagePlatform: "linux/amd64",
		Env: map[string]string{
			"MYSQL_ROOT_PASSWORD": "root",
			"MYSQL_DATABASE":      db,
			"MYSQL_USER":          user,
			"MYSQL_PASSWORD":      pass,
		},
		ExposedPorts: []string{"3306/tcp"},
		WaitingFor: wait.ForAll(
			wait.ForListeningPort(nat.Port("3306/tcp")),
			wait.ForLog("ready for connections").WithOccurrence(1),
		).WithStartupTimeout(2 * time.Minute),
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
	port, err := c.MappedPort(ctx, nat.Port("3306/tcp"))
	if err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}

	// mysql driver DSN: user:pass@tcp(host:port)/db
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci", user, pass, host, port.Port(), db)
	return &MySQL{DSN: dsn, c: c}, nil
}

func (m *MySQL) Terminate(ctx context.Context) error {
	if m == nil || m.c == nil {
		return nil
	}
	return m.c.Terminate(ctx)
}
