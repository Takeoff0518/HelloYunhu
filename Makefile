# HelloYunhu 构建脚本。需要 GNU make。
#
# 常用目标：
#   make build   编译可执行文件到 bin/
#   make dist    构建三平台发行包到 dist/，需要 Linux 或 macOS 环境
#   make check   格式检查、静态检查与测试
#   make test    运行测试
#   make lint    运行 golangci-lint

GO ?= go
BIN_DIR ?= bin
DIST_DIR ?= dist

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(BUILD_DATE)

ifeq ($(OS),Windows_NT)
BINARY := $(BIN_DIR)/helloyunhu.exe
else
BINARY := $(BIN_DIR)/helloyunhu
endif

.PHONY: all build dist run test race vet fmt lint check tidy clean

all: check build

## build: 编译可执行文件
build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/helloyunhu

## dist: 构建三平台发行包，需要 Linux 或 macOS 环境
dist:
	@rm -rf $(DIST_DIR)
	@mkdir -p $(DIST_DIR)
	@for target in linux/amd64 windows/amd64 darwin/arm64; do \
		goos="$${target%/*}"; goarch="$${target#*/}"; name="helloyunhu-$$goos-$$goarch"; \
		work="$(DIST_DIR)/$$name"; mkdir -p "$$work"; \
		binary=helloyunhu; if [ "$$goos" = windows ]; then binary=helloyunhu.exe; fi; \
		CGO_ENABLED=0 GOOS="$$goos" GOARCH="$$goarch" \
			$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o "$$work/$$binary" ./cmd/helloyunhu || exit 1; \
		cp LICENSE README.md "$$work/"; mkdir -p "$$work/examples"; cp examples/config.yaml "$$work/examples/"; \
	done
	@cd $(DIST_DIR) && for dir in */; do \
		name="$${dir%/}"; \
		case "$$name" in \
			*windows*) (cd "$$name" && zip -qr "../$$name.zip" .) ;; \
			*) tar -czf "$$name.tar.gz" -C "$$name" . ;; \
		esac; \
	done
	@cd $(DIST_DIR) && sha256sum ./*.tar.gz ./*.zip > sha256sums.txt && cat sha256sums.txt

## run: 使用根目录的 config.yaml 启动
run: build
	$(BINARY) run -config config.yaml

## test: 运行单元测试
test:
	$(GO) test ./...

## race: 运行竞态检测。需要 cgo 与可用的 C 编译器
race:
	$(GO) test -race ./...

## vet: 运行 go vet
vet:
	$(GO) vet ./...

## fmt: 格式化代码
fmt:
	$(GO) fmt ./...

## lint: 运行 golangci-lint，需要先安装该工具
lint:
	golangci-lint run

## check: 提交前必须通过的检查
check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "以下文件需要 gofmt："; echo "$$unformatted"; exit 1; \
	fi
	$(GO) vet ./...
	$(GO) test ./...

## tidy: 整理依赖
tidy:
	$(GO) mod tidy

## clean: 删除构建产物
clean:
	rm -rf $(BIN_DIR)
