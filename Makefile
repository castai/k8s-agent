build:
	go build -o bin/castai-agent
	docker build -t castai/agent:0.0.1 .

push:
	docker push castai/agent:0.0.1

release: build push

