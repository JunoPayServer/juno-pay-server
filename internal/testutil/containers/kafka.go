package containers

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type Kafka struct {
	Brokers string

	net   testcontainers.Network
	zk    testcontainers.Container
	kafka testcontainers.Container
}

func StartKafka(ctx context.Context) (*Kafka, error) {
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		k, err := startKafkaOnce(ctx)
		if err == nil {
			return k, nil
		}
		lastErr = err
		if !isPortAllocatedErr(err) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil, lastErr
}

func startKafkaOnce(ctx context.Context) (*Kafka, error) {
	hostPort, err := freePort()
	if err != nil {
		return nil, err
	}

	netName := fmt.Sprintf("juno-pay-kafka-%d", time.Now().UnixNano())
	network, err := testcontainers.GenericNetwork(ctx, testcontainers.GenericNetworkRequest{
		NetworkRequest: testcontainers.NetworkRequest{
			Name: netName,
		},
	})
	if err != nil {
		return nil, err
	}

	zkReq := testcontainers.ContainerRequest{
		Image:         "confluentinc/cp-zookeeper:7.6.1",
		ImagePlatform: "linux/amd64",
		Env: map[string]string{
			"ZOOKEEPER_CLIENT_PORT": "2181",
			"ZOOKEEPER_TICK_TIME":   "2000",
		},
		ExposedPorts:   []string{"2181/tcp"},
		Networks:       []string{netName},
		NetworkAliases: map[string][]string{netName: {"zookeeper"}},
		WaitingFor:     wait.ForListeningPort(nat.Port("2181/tcp")).WithStartupTimeout(90 * time.Second),
	}
	zk, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: zkReq, Started: true})
	if err != nil {
		_ = network.Remove(ctx)
		return nil, err
	}

	kafkaReq := testcontainers.ContainerRequest{
		Image:         "confluentinc/cp-kafka:7.6.1",
		ImagePlatform: "linux/amd64",
		Env: map[string]string{
			"KAFKA_BROKER_ID":                                "1",
			"KAFKA_ZOOKEEPER_CONNECT":                        "zookeeper:2181",
			"KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR":         "1",
			"KAFKA_TRANSACTION_STATE_LOG_MIN_ISR":            "1",
			"KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR": "1",
			"KAFKA_GROUP_INITIAL_REBALANCE_DELAY_MS":         "0",
			"KAFKA_AUTO_CREATE_TOPICS_ENABLE":                "true",
			"KAFKA_LISTENER_SECURITY_PROTOCOL_MAP":           "PLAINTEXT:PLAINTEXT,PLAINTEXT_HOST:PLAINTEXT",
			"KAFKA_LISTENERS":                                "PLAINTEXT://0.0.0.0:9092,PLAINTEXT_HOST://0.0.0.0:29092",
			"KAFKA_ADVERTISED_LISTENERS":                     "PLAINTEXT://kafka:9092,PLAINTEXT_HOST://127.0.0.1:" + strconv.Itoa(hostPort),
			"KAFKA_INTER_BROKER_LISTENER_NAME":               "PLAINTEXT",
		},
		ExposedPorts:   []string{"29092/tcp"},
		Networks:       []string{netName},
		NetworkAliases: map[string][]string{netName: {"kafka"}},
		HostConfigModifier: func(hc *dockercontainer.HostConfig) {
			hc.PortBindings = nat.PortMap{
				nat.Port("29092/tcp"): []nat.PortBinding{{
					HostIP:   "127.0.0.1",
					HostPort: strconv.Itoa(hostPort),
				}},
			}
		},
		WaitingFor: wait.ForListeningPort(nat.Port("29092/tcp")).WithStartupTimeout(2 * time.Minute),
	}
	kc, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: kafkaReq, Started: true})
	if err != nil {
		_ = zk.Terminate(ctx)
		_ = network.Remove(ctx)
		return nil, err
	}

	host, err := kc.Host(ctx)
	if err != nil {
		_ = kc.Terminate(ctx)
		_ = zk.Terminate(ctx)
		_ = network.Remove(ctx)
		return nil, err
	}

	return &Kafka{
		Brokers: fmt.Sprintf("%s:%d", host, hostPort),
		net:     network,
		zk:      zk,
		kafka:   kc,
	}, nil
}

func isPortAllocatedErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "port is already allocated") ||
		strings.Contains(msg, "Bind for") ||
		strings.Contains(msg, "failed to set up container networking")
}

func (k *Kafka) Terminate(ctx context.Context) error {
	if k == nil {
		return nil
	}
	if k.kafka != nil {
		_ = k.kafka.Terminate(ctx)
	}
	if k.zk != nil {
		_ = k.zk.Terminate(ctx)
	}
	if k.net != nil {
		_ = k.net.Remove(ctx)
	}
	return nil
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}
