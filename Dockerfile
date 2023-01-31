FROM alpine:3.17.0
ARG TARGETARCH
COPY bin/castai-agent-$TARGETARCH /usr/local/bin/castai-agent
CMD ["castai-agent"]