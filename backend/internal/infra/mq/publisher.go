package inframq

import (
	"context"
	"encoding/json"
	"time"

	contract "GCFeed/internal/shared/contract"

	amqp "github.com/rabbitmq/amqp091-go"
)

// publishEvent 统一序列化与消息属性，避免多个发布方法重复同样的代码。
// 消息属性（ContentType / DeliveryMode / MessageId / Timestamp）属于对外契约。
func publishEvent[T any](ctx context.Context, channel *amqp.Channel, exchange string, routingKey string, eventID string, body *T) error {
	content, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return channel.PublishWithContext(
		ctx,
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    eventID,
			Timestamp:    time.Now(),
			Body:         content,
		},
	)
}

// PublishActionChanged 投递点赞收藏变更事件。
func (r *RabbitMQ) PublishActionChanged(ctx context.Context, event *contract.ActionChangedEvent) error {
	if event == nil {
		return nil
	}
	return publishEvent(ctx, r.publishChannel, r.config.InteractionExchange, r.config.ActionChangedRouting, event.EventID, event)
}

// PublishVideoPublished 投递视频发布事件。
func (r *RabbitMQ) PublishVideoPublished(ctx context.Context, event *contract.PublishedEvent) error {
	if event == nil {
		return nil
	}
	return publishEvent(ctx, r.publishChannel, r.config.VideoExchange, r.config.VideoPublishedRouting, event.EventID, event)
}

// PublishViewEventRecorded 投递观看行为已落库事件。
func (r *RabbitMQ) PublishViewEventRecorded(ctx context.Context, event *contract.ViewEventRecordedEvent) error {
	if event == nil {
		return nil
	}
	return publishEvent(ctx, r.publishChannel, r.config.ExposureExchange, r.config.ViewEventRecordedRouting, event.EventID, event)
}
