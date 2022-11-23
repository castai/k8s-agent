FROM alpine:3.17.0
ARG TARGETARCH=amd64
COPY bin/castai-agent-$TARGETARCH /usr/local/bin/castai-agent
CMD ["castai-agent"]
