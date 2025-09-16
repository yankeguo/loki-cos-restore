#!/bin/bash

set -eux

source .env

IMAGE_TAG="$(git rev-parse --abbrev-ref HEAD)-$(git rev-parse --short HEAD)"

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o loki-cos-restore .

docker build --platform linux/amd64 --push -t "${PRIVATE_IMAGE}:${IMAGE_TAG}" .
