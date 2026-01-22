ARG BASE_IMAGE=mcr.microsoft.com/devcontainers/go
ARG BASE_VERSION=dev-1.24

FROM ${BASE_IMAGE}:${BASE_VERSION}

ARG BASE_IMAGE
ARG BASE_VERSION
RUN echo "${BASE_IMAGE}:${BASE_VERSION}" > /build-args.txt

RUN mkdir -p /app
WORKDIR /app
