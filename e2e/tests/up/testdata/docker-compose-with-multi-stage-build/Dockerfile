FROM mcr.microsoft.com/devcontainers/go:1-1.23-bullseye AS base

RUN apt update
RUN apt install vim -y

FROM base

RUN mkdir -p /app
WORKDIR /app