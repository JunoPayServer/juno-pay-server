package containers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type Mongo struct {
	URI      string
	Database string

	c testcontainers.Container
}

func StartMongoReplicaSet(ctx context.Context) (*Mongo, error) {
	hostPort, err := freePort()
	if err != nil {
		return nil, err
	}

	portStr := strconv.Itoa(hostPort)
	port := nat.Port(portStr + "/tcp")

	req := testcontainers.ContainerRequest{
		Image:         "mongo:7.0.15",
		ImagePlatform: "linux/amd64",
		ExposedPorts:  []string{string(port)},
		Cmd: []string{
			"mongod",
			"--replSet", "rs0",
			"--bind_ip_all",
			"--port", portStr,
		},
		HostConfigModifier: func(hc *dockercontainer.HostConfig) {
			hc.PortBindings = nat.PortMap{
				port: []nat.PortBinding{{
					HostIP:   "127.0.0.1",
					HostPort: portStr,
				}},
			}
		},
		WaitingFor: wait.ForListeningPort(port).WithStartupTimeout(90 * time.Second),
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

	// Initiate replica set (single node).
	initJS := fmt.Sprintf(`rs.initiate({_id:"rs0",members:[{_id:0,host:"127.0.0.1:%s"}]})`, portStr)
	if err := execMongosh(ctx, c, portStr, initJS); err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}

	if err := waitForMongoPrimary(ctx, c, portStr); err != nil {
		_ = c.Terminate(ctx)
		return nil, err
	}

	uri := fmt.Sprintf("mongodb://%s:%s/?replicaSet=rs0", host, portStr)
	return &Mongo{
		URI:      uri,
		Database: "juno_pay",
		c:        c,
	}, nil
}

func (m *Mongo) Terminate(ctx context.Context) error {
	if m == nil || m.c == nil {
		return nil
	}
	return m.c.Terminate(ctx)
}

func waitForMongoPrimary(ctx context.Context, c testcontainers.Container, port string) error {
	deadline := time.Now().Add(30 * time.Second)
	if dl, ok := ctx.Deadline(); ok {
		if d := time.Until(dl); d > 0 && d < 30*time.Second {
			deadline = time.Now().Add(d)
		}
	}

	for time.Now().Before(deadline) {
		out, err := mongoshEval(ctx, c, port, "db.hello().isWritablePrimary")
		if err == nil {
			if strings.TrimSpace(out) == "true" {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return fmt.Errorf("mongo: primary not ready")
}

func execMongosh(ctx context.Context, c testcontainers.Container, port, js string) error {
	_, err := mongoshEval(ctx, c, port, js)
	return err
}

func mongoshEval(ctx context.Context, c testcontainers.Container, port, js string) (string, error) {
	exitCode, reader, err := c.Exec(ctx, []string{"mongosh", "--quiet", "--port", port, "--eval", js})
	if err != nil {
		return "", err
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, bytes.NewReader(raw)); err != nil {
		stdout.Write(raw)
	}
	if exitCode != 0 {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return "", fmt.Errorf("mongosh exit %d: %s", exitCode, msg)
	}

	return strings.TrimSpace(stdout.String()), nil
}
