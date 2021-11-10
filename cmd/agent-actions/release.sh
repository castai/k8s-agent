#!/bin/bash

set -e

GITHUB_SHA=local
GITHUB_REF=local
RELEASE_TAG=v0.0.0-alpha1
GITHUB_SHA=local

GOOS=linux GOARCH=amd64 go build -ldflags "-X main.GitCommit=$GITHUB_SHA -X main.GitRef=$GITHUB_REF -X main.Version=${RELEASE_TAG:-commit-$GITHUB_SHA}" -o bin/castai-agent-actions .

docker build -t castai/agent-actions:$RELEASE_TAG .
docker tag castai/agent-actions:$RELEASE_TAG castai/agent-actions:latest

docker push castai/agent-actions:$RELEASE_TAG
docker push castai/agent-actions:latest
