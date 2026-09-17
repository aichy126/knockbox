VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: build test cover lint run docker
build:
	go build -trimpath -ldflags "-s -w -X main.Version=$(VERSION)" -o knockbox .

test:
	go test ./... -race -cover

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
