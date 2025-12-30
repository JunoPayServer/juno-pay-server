//go:build integration

package outbox

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
	"github.com/Abdullah1738/juno-pay-server/internal/store"
	"github.com/Abdullah1738/juno-pay-server/internal/testutil/containers"
	"github.com/nats-io/nats.go"
	"github.com/rabbitmq/amqp091-go"
	kafka "github.com/segmentio/kafka-go"
)

func TestWorker_Kafka_Delivery_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	k, err := containers.StartKafka(ctx)
	if err != nil {
		t.Fatalf("StartKafka: %v", err)
	}
	defer func() { _ = k.Terminate(context.Background()) }()

	topic := "juno.pay.test"

	conn, err := kafka.Dial("tcp", k.Brokers)
	if err != nil {
		t.Fatalf("kafka dial: %v", err)
	}
	if err := conn.CreateTopics(kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}); err != nil {
		_ = conn.Close()
		t.Fatalf("kafka create topic: %v", err)
	}
	_ = conn.Close()

	st := store.NewMem()
	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
		InvoiceTTLSeconds:     0,
		RequiredConfirmations: 1,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	})
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}
	if _, err := st.CreateEventSink(ctx, store.EventSinkCreate{
		MerchantID: m.MerchantID,
		Kind:       domain.EventSinkKafka,
		Config:     json.RawMessage(`{"brokers":"` + k.Brokers + `","topic":"` + topic + `"}`),
	}); err != nil {
		t.Fatalf("CreateEventSink: %v", err)
	}
	if _, _, err := st.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1addr0",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             1,
		RequiredConfirmations: 1,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	}); err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	w, err := New(st)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{k.Brokers},
		Topic:       topic,
		Partition:   0,
		MinBytes:    1,
		MaxBytes:    1e6,
		StartOffset: kafka.FirstOffset,
	})
	defer func() { _ = r.Close() }()

	readCtx, readCancel := context.WithTimeout(ctx, 30*time.Second)
	defer readCancel()
	msg, err := r.ReadMessage(readCtx)
	if err != nil {
		t.Fatalf("kafka read: %v", err)
	}
	var ce domain.CloudEvent
	if err := json.Unmarshal(msg.Value, &ce); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ce.Type != "invoice.created" {
		t.Fatalf("unexpected ce.type=%q", ce.Type)
	}
}

func TestWorker_NATS_Delivery_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	ns, err := containers.StartNATS(ctx)
	if err != nil {
		t.Fatalf("StartNATS: %v", err)
	}
	defer func() { _ = ns.Terminate(context.Background()) }()

	subject := "juno.pay.test"

	nc, err := nats.Connect(ns.URL, nats.Timeout(5*time.Second))
	if err != nil {
		t.Fatalf("nats connect: %v", err)
	}
	defer nc.Close()

	ch := make(chan *nats.Msg, 1)
	if _, err := nc.ChanSubscribe(subject, ch); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := nc.FlushTimeout(5 * time.Second); err != nil {
		t.Fatalf("flush: %v", err)
	}

	st := store.NewMem()
	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
		InvoiceTTLSeconds:     0,
		RequiredConfirmations: 1,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	})
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}
	if _, err := st.CreateEventSink(ctx, store.EventSinkCreate{
		MerchantID: m.MerchantID,
		Kind:       domain.EventSinkNATS,
		Config:     json.RawMessage(`{"url":"` + ns.URL + `","subject":"` + subject + `"}`),
	}); err != nil {
		t.Fatalf("CreateEventSink: %v", err)
	}
	if _, _, err := st.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1addr0",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             1,
		RequiredConfirmations: 1,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	}); err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	w, err := New(st)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	select {
	case msg := <-ch:
		var ce domain.CloudEvent
		if err := json.Unmarshal(msg.Data, &ce); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if ce.Type != "invoice.created" {
			t.Fatalf("unexpected ce.type=%q", ce.Type)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for nats message")
	}
}

func TestWorker_RabbitMQ_Delivery_Integration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	rb, err := containers.StartRabbitMQ(ctx)
	if err != nil {
		t.Fatalf("StartRabbitMQ: %v", err)
	}
	defer func() { _ = rb.Terminate(context.Background()) }()

	queue := "juno.pay.test"

	conn, err := amqp091.Dial(rb.URL)
	if err != nil {
		t.Fatalf("amqp dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("channel: %v", err)
	}
	defer func() { _ = ch.Close() }()

	if _, err := ch.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		t.Fatalf("queue declare: %v", err)
	}
	msgs, err := ch.Consume(queue, "", true, false, false, false, nil)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}

	st := store.NewMem()
	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
		InvoiceTTLSeconds:     0,
		RequiredConfirmations: 1,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	})
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}
	if _, err := st.CreateEventSink(ctx, store.EventSinkCreate{
		MerchantID: m.MerchantID,
		Kind:       domain.EventSinkRabbitMQ,
		Config:     json.RawMessage(`{"url":"` + rb.URL + `","queue":"` + queue + `"}`),
	}); err != nil {
		t.Fatalf("CreateEventSink: %v", err)
	}
	if _, _, err := st.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1addr0",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             1,
		RequiredConfirmations: 1,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	}); err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	w, err := New(st)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := w.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	select {
	case msg := <-msgs:
		var ce domain.CloudEvent
		if err := json.Unmarshal(msg.Body, &ce); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if ce.Type != "invoice.created" {
			t.Fatalf("unexpected ce.type=%q", ce.Type)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for rabbitmq message")
	}
}

