FROM ubuntu AS base

RUN apt update
RUN apt install vim -y

FROM base

RUN mkdir -p /app
WORKDIR /app
