FROM redhat/ubi8:8.7-1037-source as rh-ubi
ARG TARGETARCH=amd64
COPY bin/castai-agent-$TARGETARCH /usr/local/bin/castai-agent
CMD ["castai-agent"]
