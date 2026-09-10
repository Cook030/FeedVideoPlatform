package inframq

import (
	applicationexposure "GCFeed/internal/application/exposure"
	applicationinteraction "GCFeed/internal/application/interaction"
	applicationvideo "GCFeed/internal/application/video"
	infraconfig "GCFeed/internal/infra/config"
	inframetrics "GCFeed/internal/infra/metrics"
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const defaultInteractionExchange = "gcfeed.interaction"
const defaultActionChangedQueue = "gcfeed.interaction.action_changed"
const defaultActionChangedRouting = "interaction.action_changed"
const defaultVideoExchange = "gcfeed.video"
const defaultVideoPublishedQueue = "gcfeed.video.published"
const defaultVideoEmbeddingQueue = "gcfeed.video.embedding"
const defaultVideoPublishedRouting = "video.published"
const defaultExposureExchange = "gcfeed.exposure"
const defaultViewEventRecordedQueue = "gcfeed.exposure.view_event_recorded"
const defaultViewEventRecordedRouting = "exposure.view_event_recorded"

// defaultMaxRetries 是单条消息的最大重投次数，超过后消息进入死信队列。
const defaultMaxRetries = 3

// retryCountHeader 记录消息已被重投的次数，避免毒消息被无限 requeue。
const retryCountHeader = "x-retry-count"

const deadLetterExchangeSuffix = ".dlx"
const deadLetterQueueSuffix = ".dlq"

var ErrEmptyRabbitMQURL = errors.New("rabbitmq url is empty")

type RabbitMQ struct {
	conn            *amqp.Connection
	publishChannel  *amqp.Channel
	consumerChannel *amqp.Channel
	config          infraconfig.RabbitMQConfig
}

func NewRabbitMQ(cfg infraconfig.RabbitMQConfig) (*RabbitMQ, error) {
	cfg = normalizeRabbitMQConfig(cfg)
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, ErrEmptyRabbitMQURL
	}

	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, err
	}
	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	consumerChannel, err := conn.Channel()
	if err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, err
	}

	client := &RabbitMQ{
		conn:            conn,
		publishChannel:  channel,
		consumerChannel: consumerChannel,
		config:          cfg,
	}
	if err := client.ensureTopology(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func (r *RabbitMQ) Close() error {
	if r == nil {
		return nil
	}
	if r.publishChannel != nil {
		_ = r.publishChannel.Close()
	}
	if r.consumerChannel != nil {
		_ = r.consumerChannel.Close()
	}
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}

func (r *RabbitMQ) PublishActionChanged(ctx context.Context, event *applicationinteraction.ActionChangedEvent) error {
	if event == nil {
		return nil
	}
	content, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.publishChannel.PublishWithContext(
		ctx,
		r.config.InteractionExchange,
		r.config.ActionChangedRouting,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    event.EventID,
			Timestamp:    time.Now(),
			Body:         content,
		},
	)
}

func (r *RabbitMQ) PublishVideoPublished(ctx context.Context, event *applicationvideo.PublishedEvent) error {
	if event == nil {
		return nil
	}
	content, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.publishChannel.PublishWithContext(
		ctx,
		r.config.VideoExchange,
		r.config.VideoPublishedRouting,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    event.EventID,
			Timestamp:    time.Now(),
			Body:         content,
		},
	)
}

func (r *RabbitMQ) PublishViewEventRecorded(ctx context.Context, event *applicationexposure.ViewEventRecordedEvent) error {
	if event == nil {
		return nil
	}
	content, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return r.publishChannel.PublishWithContext(
		ctx,
		r.config.ExposureExchange,
		r.config.ViewEventRecordedRouting,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    event.EventID,
			Timestamp:    time.Now(),
			Body:         content,
		},
	)
}

func (r *RabbitMQ) ConsumeActionChanged(ctx context.Context, handler func(context.Context, *applicationinteraction.ActionChangedEvent) error) error {
	return consumeJSON(ctx, r, r.config.ActionChangedQueue, "mq_action_changed", handler)
}

func (r *RabbitMQ) ConsumeVideoPublished(ctx context.Context, handler func(context.Context, *applicationvideo.PublishedEvent) error) error {
	return consumeJSON(ctx, r, r.config.VideoPublishedQueue, "mq_video_published", handler)
}

func (r *RabbitMQ) ConsumeVideoPublishedForEmbedding(ctx context.Context, handler func(context.Context, *applicationvideo.PublishedEvent) error) error {
	return consumeJSON(ctx, r, r.config.VideoEmbeddingQueue, "mq_video_published", handler)
}

func (r *RabbitMQ) ConsumeViewEventRecorded(ctx context.Context, handler func(context.Context, *applicationexposure.ViewEventRecordedEvent) error) error {
	return consumeJSON(ctx, r, r.config.ViewEventRecordedQueue, "mq_view_event", handler)
}

// consumeJSON 统一消费循环：反序列化、业务处理、失败后的重试与死信判定。
func consumeJSON[T any](ctx context.Context, r *RabbitMQ, queue string, jobName string, handler func(context.Context, *T) error) error {
	deliveries, err := r.consumerChannel.ConsumeWithContext(
		ctx,
		queue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		for delivery := range deliveries {
			start := time.Now()
			var event T
			if err := json.Unmarshal(delivery.Body, &event); err != nil {
				// 反序列化失败无论重试多少次都不会成功，直接丢弃。
				inframetrics.ObserveWorkerJob(jobName+"_decode", time.Since(start), err)
				_ = delivery.Nack(false, false)
				continue
			}
			if err := handler(ctx, &event); err != nil {
				inframetrics.ObserveWorkerJob(jobName+"_consume", time.Since(start), err)
				r.retryOrDeadLetter(ctx, delivery, jobName, time.Since(start), err)
				continue
			}
			inframetrics.ObserveWorkerJob(jobName+"_consume", time.Since(start), nil)
			_ = delivery.Ack(false)
		}
	}()
	return nil
}

// retryOrDeadLetter 决定失败消息是重投还是进入死信队列。
//
// 早期实现统一使用 Nack(requeue=true)，一条业务上永远失败的消息会被无限
// 重新投递，占满消费者并让整条队列堵死。这里改为重新发布同一条消息并递增
// x-retry-count，达到上限后 Nack(requeue=false)，由队列的死信交换机接管。
func (r *RabbitMQ) retryOrDeadLetter(ctx context.Context, delivery amqp.Delivery, jobName string, elapsed time.Duration, cause error) {
	attempt := retryCountOf(delivery)
	if attempt >= r.maxRetries() {
		inframetrics.ObserveWorkerJob(jobName+"_dead_letter", elapsed, cause)
		log.Printf("rabbitmq drop message id=%s routing=%s after %d retries: %v", delivery.MessageId, delivery.RoutingKey, attempt, cause)
		_ = delivery.Nack(false, false)
		return
	}
	if delivery.RoutingKey == "" {
		// 缺少路由信息无法重新发布，退回 requeue 避免丢消息。
		_ = delivery.Nack(false, true)
		return
	}
	if err := r.republish(ctx, delivery, attempt+1); err != nil {
		// 重投失败时退回 requeue，宁可重复消费也不静默丢消息。
		log.Printf("rabbitmq republish message id=%s failed: %v", delivery.MessageId, err)
		_ = delivery.Nack(false, true)
		return
	}
	_ = delivery.Ack(false)
}

// republish 把消息按原始 exchange 和 routing key 重新发布，并写入重试次数。
func (r *RabbitMQ) republish(ctx context.Context, delivery amqp.Delivery, attempt int) error {
	headers := make(amqp.Table, len(delivery.Headers)+1)
	for key, value := range delivery.Headers {
		headers[key] = value
	}
	headers[retryCountHeader] = int64(attempt)
	return r.publishChannel.PublishWithContext(
		ctx,
		delivery.Exchange,
		delivery.RoutingKey,
		false,
		false,
		amqp.Publishing{
			Headers:      headers,
			ContentType:  delivery.ContentType,
			DeliveryMode: amqp.Persistent,
			MessageId:    delivery.MessageId,
			Timestamp:    time.Now(),
			Body:         delivery.Body,
		},
	)
}

func (r *RabbitMQ) maxRetries() int {
	if r.config.MaxRetries > 0 {
		return r.config.MaxRetries
	}
	return defaultMaxRetries
}

// retryCountOf 读取已重投次数，AMQP 数字可能被反序列化成多种整型。
func retryCountOf(delivery amqp.Delivery) int {
	switch value := delivery.Headers[retryCountHeader].(type) {
	case int64:
		if value > 0 {
			return int(value)
		}
	case int:
		if value > 0 {
			return value
		}
	case int32:
		if value > 0 {
			return int(value)
		}
	case float64:
		if value > 0 {
			return int(value)
		}
	}
	return 0
}

func (r *RabbitMQ) ensureTopology() error {
	if err := r.declareExchange(r.config.InteractionExchange); err != nil {
		return err
	}
	if err := r.declareQueue(r.config.ActionChangedQueue, r.config.InteractionExchange, r.config.ActionChangedRouting); err != nil {
		return err
	}

	if err := r.declareExchange(r.config.VideoExchange); err != nil {
		return err
	}
	if err := r.declareQueue(r.config.VideoPublishedQueue, r.config.VideoExchange, r.config.VideoPublishedRouting); err != nil {
		return err
	}
	if err := r.declareQueue(r.config.VideoEmbeddingQueue, r.config.VideoExchange, r.config.VideoPublishedRouting); err != nil {
		return err
	}

	if err := r.declareExchange(r.config.ExposureExchange); err != nil {
		return err
	}
	return r.declareQueue(r.config.ViewEventRecordedQueue, r.config.ExposureExchange, r.config.ViewEventRecordedRouting)
}

func (r *RabbitMQ) declareExchange(name string) error {
	return r.publishChannel.ExchangeDeclare(name, "topic", true, false, false, false, nil)
}

// declareQueue 声明业务队列、绑定路由，并为它准备专属的死信交换机和死信队列，
// 让重试耗尽的消息可以被留存和排查，而不是被静默丢弃。
func (r *RabbitMQ) declareQueue(name string, exchange string, routingKey string) error {
	deadLetterExchange, err := r.declareDeadLetter(name)
	if err != nil {
		return err
	}
	if err := r.ensureBusinessQueue(name, deadLetterExchange); err != nil {
		return err
	}
	return r.publishChannel.QueueBind(name, routingKey, exchange, false, nil)
}

// declareDeadLetter 声明队列专属的死信交换机与死信队列，返回死信交换机名。
func (r *RabbitMQ) declareDeadLetter(queue string) (string, error) {
	exchange := queue + deadLetterExchangeSuffix
	deadLetterQueue := queue + deadLetterQueueSuffix
	if err := r.publishChannel.ExchangeDeclare(exchange, "fanout", true, false, false, false, nil); err != nil {
		return "", err
	}
	if _, err := r.publishChannel.QueueDeclare(deadLetterQueue, true, false, false, false, nil); err != nil {
		return "", err
	}
	if err := r.publishChannel.QueueBind(deadLetterQueue, "", exchange, false, nil); err != nil {
		return "", err
	}
	return exchange, nil
}

// ensureBusinessQueue 声明业务队列，并在发现它缺少死信参数时就地迁移。
//
// 队列参数在 RabbitMQ 里不可变：已存在的队列如果参数不同，重新声明会得到 406。
// 迁移只在队列空闲且为空时执行（ifUnused + ifEmpty），因此不会丢消息，
// 也不会打断正在消费的实例；条件不满足时保留旧队列并告警，死信能力暂不生效。
func (r *RabbitMQ) ensureBusinessQueue(name string, deadLetterExchange string) error {
	args := amqp.Table{"x-dead-letter-exchange": deadLetterExchange}
	if _, err := r.publishChannel.QueueDeclare(name, true, false, false, false, args); err == nil {
		return nil
	} else if !isPreconditionFailed(err) {
		return err
	}

	// 406 属于 channel 级错误，broker 已经关掉了这条 channel，后续操作必须换新的。
	if err := r.reopenPublishChannel(); err != nil {
		return err
	}
	_, deleteErr := r.publishChannel.QueueDelete(name, true, true, false)
	if deleteErr != nil {
		// 删除失败同样是 channel 级错误（例如队列非空），先重建再尝试声明。
		if err := r.reopenPublishChannel(); err != nil {
			return err
		}
	}
	_, declareErr := r.publishChannel.QueueDeclare(name, true, false, false, false, args)
	if declareErr == nil {
		log.Printf("rabbitmq queue %s migrated to dead-letter exchange %s", name, deadLetterExchange)
		return nil
	}
	log.Printf("rabbitmq queue %s keeps old parameters, dead-letter is inactive (delete=%v declare=%v)", name, deleteErr, declareErr)
	// 这次失败同样会关闭 channel，重建后返回，避免影响后续拓扑声明。
	return r.reopenPublishChannel()
}

// reopenPublishChannel 在 channel 被 broker 因协议错误关闭后重建它。
func (r *RabbitMQ) reopenPublishChannel() error {
	if r.publishChannel != nil {
		_ = r.publishChannel.Close()
	}
	channel, err := r.conn.Channel()
	if err != nil {
		return err
	}
	r.publishChannel = channel
	return nil
}

func isPreconditionFailed(err error) bool {
	var amqpErr *amqp.Error
	if !errors.As(err, &amqpErr) {
		return false
	}
	return amqpErr.Code == amqp.PreconditionFailed
}

func normalizeRabbitMQConfig(cfg infraconfig.RabbitMQConfig) infraconfig.RabbitMQConfig {
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.InteractionExchange = strings.TrimSpace(cfg.InteractionExchange)
	cfg.ActionChangedQueue = strings.TrimSpace(cfg.ActionChangedQueue)
	cfg.ActionChangedRouting = strings.TrimSpace(cfg.ActionChangedRouting)
	if cfg.InteractionExchange == "" {
		cfg.InteractionExchange = defaultInteractionExchange
	}
	if cfg.ActionChangedQueue == "" {
		cfg.ActionChangedQueue = defaultActionChangedQueue
	}
	if cfg.ActionChangedRouting == "" {
		cfg.ActionChangedRouting = defaultActionChangedRouting
	}
	if cfg.VideoExchange == "" {
		cfg.VideoExchange = defaultVideoExchange
	}
	if cfg.VideoPublishedQueue == "" {
		cfg.VideoPublishedQueue = defaultVideoPublishedQueue
	}
	cfg.VideoEmbeddingQueue = strings.TrimSpace(cfg.VideoEmbeddingQueue)
	if cfg.VideoEmbeddingQueue == "" {
		cfg.VideoEmbeddingQueue = defaultVideoEmbeddingQueue
	}
	if cfg.VideoPublishedRouting == "" {
		cfg.VideoPublishedRouting = defaultVideoPublishedRouting
	}
	cfg.ExposureExchange = strings.TrimSpace(cfg.ExposureExchange)
	cfg.ViewEventRecordedQueue = strings.TrimSpace(cfg.ViewEventRecordedQueue)
	cfg.ViewEventRecordedRouting = strings.TrimSpace(cfg.ViewEventRecordedRouting)
	if cfg.ExposureExchange == "" {
		cfg.ExposureExchange = defaultExposureExchange
	}
	if cfg.ViewEventRecordedQueue == "" {
		cfg.ViewEventRecordedQueue = defaultViewEventRecordedQueue
	}
	if cfg.ViewEventRecordedRouting == "" {
		cfg.ViewEventRecordedRouting = defaultViewEventRecordedRouting
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = defaultMaxRetries
	}
	return cfg
}
