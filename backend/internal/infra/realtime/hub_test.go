package inforealtime

import (
	"testing"
	"time"
)

// 以下用例迁移自 interfaces/http/message/stream_test.go，断言保持不变。

func TestHubDispatchToLocalClient(t *testing.T) {
	hub := NewHub()
	client := hub.register(7)
	defer hub.unregister(client)

	hub.OnEvent(7, "message", []byte(`{"id":1}`))

	select {
	case event := <-client.ch:
		if event.Type != "message" {
			t.Fatalf("expected message type, got %q", event.Type)
		}
		if string(event.Data) != `{"id":1}` {
			t.Fatalf("unexpected data: %s", event.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("expected event to be delivered")
	}

	// 其它用户的连接不应收到事件。
	other := hub.register(8)
	defer hub.unregister(other)
	hub.OnEvent(7, "message", []byte(`{"id":2}`))
	select {
	case <-other.ch:
		t.Fatal("unexpected event delivered to another user")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestHubDropsSlowClient(t *testing.T) {
	hub := NewHub()
	client := hub.register(9)
	defer hub.unregister(client)

	// 填满该连接的缓冲，模拟消费过慢的客户端。
	for i := 0; i < sseClientBuffer; i++ {
		hub.OnEvent(9, "message", []byte(`{"id":1}`))
	}
	// 再投递一次应触发断开，channel 被关闭。
	hub.OnEvent(9, "message", []byte(`{"id":2}`))

	deadline := time.After(time.Second)
	for {
		select {
		case _, open := <-client.ch:
			if !open {
				return
			}
		case <-deadline:
			t.Fatal("expected slow client to be dropped")
		}
	}
}

func TestHubCloseClosesAllClients(t *testing.T) {
	hub := NewHub()
	first := hub.register(1)
	second := hub.register(2)

	hub.Close()

	requireChannelClosed(t, first)
	requireChannelClosed(t, second)

	// 重复 Close 应安全无副作用。
	hub.Close()
}

// TestSubscriptionExposesEventsAndUnregisters 覆盖给 HTTP 层使用的导出抽象。
func TestSubscriptionExposesEventsAndUnregisters(t *testing.T) {
	hub := NewHub()
	subscription := hub.Subscribe(11)
	defer subscription.Close()

	hub.OnEvent(11, "message", []byte(`{"id":3}`))
	select {
	case event := <-subscription.Events():
		if event.Type != "message" || string(event.Data) != `{"id":3}` {
			t.Fatalf("unexpected event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("expected event to be delivered")
	}

	subscription.Close()
	select {
	case _, open := <-subscription.Events():
		if open {
			t.Fatal("expected subscription channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("expected subscription channel to be closed")
	}
}

func requireChannelClosed(t *testing.T, client *streamClient) {
	t.Helper()
	select {
	case _, open := <-client.ch:
		if open {
			t.Fatal("expected client channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("expected client channel to be closed")
	}
}
