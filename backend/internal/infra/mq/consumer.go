package inframq

import (
	"context"
	"encoding/json"
	"log"
	"time"

	inframetrics "GCFeed/internal/infra/metrics"
	contract "GCFeed/internal/shared/contract"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ConsumeActionChanged 消费点赞收藏变更事件。
func (r *RabbitMQ) ConsumeActionChanged(ctx context.Context, handler func(context.Context, *contract.ActionChangedEvent) error) error {
	return consumeJSON(ctx, r, r.config.ActionChangedQueue, "mq_action_changed", handler)
}

// ConsumeVideoPublished 消费视频发布事件（关注流扇出）。
func (r *RabbitMQ) ConsumeVideoPublished(ctx context.Context, handler func(context.Context, *contract.PublishedEvent) error) error {
	return consumeJSON(ctx, r, r.config.VideoPublishedQueue, "mq_video_published", handler)
}

// ConsumeVideoPublishedForEmbedding 消费视频发布事件（向量化任务）。
func (r *RabbitMQ) ConsumeVideoPublishedForEmbedding(ctx context.Context, handler func(context.Context, *contract.PublishedEvent) error) error {
	return consumeJSON(ctx, r, r.config.VideoEmbeddingQueue, "mq_video_published", handler)
}

// ConsumeViewEventRecorded 消费观看行为已落库事件。
func (r *RabbitMQ) ConsumeViewEventRecorded(ctx context.Context, handler func(context.Context, *contract.ViewEventRecordedEvent) error) error {
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
