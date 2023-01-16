build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -o bin/castai-agent-amd64 .
	docker build -t sauliuscast/agent:$(VERSION) .

generate:
	go generate ./...

push:
	docker push sauliuscast/agent:$(VERSION)

deploy:
	cat deployment.yaml | envsubst | kubectl apply -f -

SHELL := /bin/bash
run:
	source ./.env && go run .

test:
	go test ./... -race

release: build push
