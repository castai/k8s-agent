#!/usr/bin/env bash

ctx="$1"

kubectl config view --minify --flatten --context "$ctx"

