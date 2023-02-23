FROM redhat/ubi9-minimal:9.1
ARG TARGETARCH=amd64
COPY bin/castai-agent-$TARGETARCH /usr/local/bin/castai-agent
COPY LICENCE /licenses/LICENCE
USER 1001010000
CMD ["castai-agent"]
