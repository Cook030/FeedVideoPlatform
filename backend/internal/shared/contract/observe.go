package contract

import "time"

// FeedObserver 观测 Feed 场景请求：应用层只依赖该抽象，
// 具体指标实现由基础设施层提供，避免 application 反向依赖 infra。
type FeedObserver interface {
	ObserveFeed(scene string, duration time.Duration, itemCount int, err error)
}

// WorkerObserver 观测后台任务执行。
type WorkerObserver interface {
	ObserveWorkerJob(job string, duration time.Duration, err error)
}
