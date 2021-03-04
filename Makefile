build:
	GOOS=linux go build -o bin/castai-agent
	docker build -t castai/agent:0.0.1.zilvinas .

push:
	docker push castai/agent:0.0.1.zilvinas

deploy:
	cat deployment.yaml | envsubst | kubectl apply -f -

release: build push

