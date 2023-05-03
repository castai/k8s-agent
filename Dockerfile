FROM gcr.io/distroless/static-debian11
ARG TARGETARCH
COPY bin/castai-agent-$TARGETARCH /usr/local/bin/castai-agent
CMD ["/usr/local/bin/castai-agent"]
