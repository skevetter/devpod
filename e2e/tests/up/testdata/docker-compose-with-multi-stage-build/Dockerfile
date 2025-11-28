FROM golang:1.21-alpine AS base

RUN apk add --no-cache vim

FROM base

RUN mkdir -p /app
WORKDIR /app