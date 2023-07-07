lint:
	golangci-lint run ./...

fix-lint:
	golangci-lint run ./... --fix

build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/castai-agent-amd64 .
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/castai-agent-arm64 .
	docker buildx build --push --platform=linux/amd64,linux/arm64 -t  $(TAG) .

generate:
	go generate ./...

SHELL := /bin/bash
run:
	source ./.env && go run .

test:
	go test ./... -race

release: build push
