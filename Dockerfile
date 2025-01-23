FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETARCH
COPY bin/castai-agent-$TARGETARCH /usr/local/bin/castai-agent
CMD ["castai-agent"]
