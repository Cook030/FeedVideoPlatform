# GCFeed 命令入口。

.PHONY: help up down logs ps test fmt build-api build-web rebuild-embeddings clean

help:
	@echo 可用命令：
	@echo   make up         一键启动全栈（后台）
	@echo   make logs       跟踪 api / web / worker 日志
	@echo   make down       停止并移除容器
	@echo   make ps         查看服务状态
	@echo   make test       运行后端测试
	@echo   make fmt        格式化后端代码
	@echo   make build-api  本地编译后端
	@echo   make build-web  前端生产构建
	@echo   make rebuild-embeddings  重算当前模型的视频向量
	@echo   make clean      停止并清理数据卷

up:
	docker compose up -d --build

down:
	docker compose down

logs:
	docker compose logs -f api web worker

ps:
	docker compose ps

test:
	cd backend && go test ./...

fmt:
	cd backend && go fmt ./...

build-api:
	cd backend && go build ./...

build-web:
	npm --prefix frontend run build

rebuild-embeddings:
	cd backend && go run ./cmd/rebuild-embeddings

clean:
	docker compose down -v
