VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build admin test cover lint run docker
# 管理界面先构建、再 embed 进二进制。没装 node 也能 `go build`——
# 那样 dist 里只有 .gitkeep，后台会说「还没构建」，其余功能一概照常。
admin:
	cd admin && npm ci && npm run build

build: admin
	go build -trimpath -ldflags "-s -w -X main.Version=$(VERSION)" -o knockbox .

test:
	go test ./... -race -cover
	cd admin && npm run test

cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1
	go tool cover -html=coverage.out

lint:
	go vet ./...
	go mod tidy -diff
	gofmt -l . | tee /dev/stderr | (! read)
	golangci-lint run ./...

run: build
	./knockbox serve -c config.toml

docker:
	docker build --build-arg VERSION=$(VERSION) -t knockbox:$(VERSION) .
