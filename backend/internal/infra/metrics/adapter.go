package inframetrics

import (
	"time"

	contract "GCFeed/internal/shared/contract"
)

// Adapter 把本包的采集函数适配为应用层依赖的观测端口。
// 指标名、标签与采集点完全沿用既有实现，不改变监控口径。
type Adapter struct{}

// NewAdapter 创建观测适配器。
func NewAdapter() Adapter {
	return Adapter{}
}

// ObserveFeed 采集一次 Feed 请求。
func (Adapter) ObserveFeed(scene string, duration time.Duration, itemCount int, err error) {
	ObserveFeed(scene, duration, itemCount, err)
}

// ObserveWorkerJob 采集一次后台任务执行。
func (Adapter) ObserveWorkerJob(job string, duration time.Duration, err error) {
	ObserveWorkerJob(job, duration, err)
}

// 编译期断言：Adapter 同时满足两个观测端口。
var (
	_ contract.FeedObserver   = Adapter{}
	_ contract.WorkerObserver = Adapter{}
)
