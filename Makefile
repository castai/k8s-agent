build:
	GOOS=linux go build -ldflags="-s -w" -o bin/castai-agent .
	docker build -t castai/agent:$(VERSION) .

push:
	docker push castai/agent:$(VERSION)

deploy:
	cat deployment.yaml | envsubst | kubectl apply -f -

release: build push

