build:
	GOOS=linux go build -o bin/castai-agent ./cmd/server/main.go
	docker build -t castai/agent:0.0.1 .

push:
	docker push castai/agent:0.0.1

deploy:
	cat deployment.yaml | envsubst | kubectl apply -f -

release: build push

