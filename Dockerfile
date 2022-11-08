FROM alpine:3.13
ARG TARGETARCH amd64
COPY bin/castai-agent-$TARGETARCH /usr/local/bin/castai-agent
CMD ["castai-agent"]
