FROM golang:alpine AS builder
WORKDIR /
COPY go.mod go.sum main.go ./
RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -o castai-agent

FROM alpine:3.13
COPY --from=builder /castai-agent /usr/local/bin/castai-agent
CMD ["castai-agent"]

