FROM golang:1.20 as build

ARG TARGETARCH=amd64

ENV GOPROXY=direct


WORKDIR /workspace
COPY go.mod .
COPY go.sum .
RUN go mod download -x

COPY build/validator/linux/${TARGETARCH}/validator /workspace/validator

ENTRYPOINT ["/workspace/validator"]