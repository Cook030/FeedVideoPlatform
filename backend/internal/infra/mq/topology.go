package inframq

import (
	"errors"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

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
