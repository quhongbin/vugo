BIN := bin/vugo
PKG := ./...

.PHONY: help build test cover fmt vet lint tidy run clean

help: ## 显示可用命令
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2}'

build: ## 编译 vugo 命令行工具
	go build -o $(BIN) ./cmd/vugo

test: ## 运行全部测试
	go test $(PKG)

cover: ## 运行测试并输出覆盖率
	go test -coverprofile=coverage.out $(PKG) && go tool cover -func=coverage.out

fmt: ## 格式化代码
	gofmt -l -w .

vet: ## 静态检查
	go vet $(PKG)

lint: fmt vet ## 格式化 + 静态检查

tidy: ## 整理依赖
	go mod tidy

run: build ## 编译后转译 testdata 示例到 /tmp/vugo-out
	$(BIN) -src ./testdata -dst /tmp/vugo-out

clean: ## 清理构建产物
	rm -rf bin coverage.out
