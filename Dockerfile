FROM alpine:3.13
COPY bin/castai-agent /usr/local/bin/castai-agent
CMD ["castai-agent"]
