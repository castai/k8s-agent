build:
	GOOS=linux -ldflags="-s -w" go build -o bin/castai-agent .
	docker build -t castai/agent:$(VERSION) .

push:
	docker push castai/agent:$(VERSION)

deploy:
	cat deployment.yaml | envsubst | kubectl apply -f -

release: build push

