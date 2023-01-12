FROM redhat/ubi8-minimal:8.7-1031 as rh-ubi
ARG TARGETARCH=amd64
COPY bin/castai-agent-$TARGETARCH /usr/local/bin/castai-agent
COPY LICENCE /licenses/LICENCE
USER 1002
CMD ["castai-agent"]