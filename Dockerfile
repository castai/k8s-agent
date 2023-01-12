FROM registry.redhat.io/ubi8/ubi:latest
ARG TARGETARCH=amd64
COPY bin/castai-agent-$TARGETARCH /usr/local/bin/castai-agent
CMD ["castai-agent"]
