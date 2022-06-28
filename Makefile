build:
	GOOS=linux go build -ldflags="-s -w" -o bin/castai-agent .
	docker build -t us-docker.pkg.dev/castai-hub/library/agent:$(VERSION) .

generate:
	go generate ./...

push:
	docker push us-docker.pkg.dev/castai-hub/library/agent:$(VERSION)

deploy:
	cat deployment.yaml | envsubst | kubectl apply -f -

SHELL := /bin/bash
run:
	source ./.env && go run .

release: build push
