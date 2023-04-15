################################
#	Build Stage            #
#			       #
################################
FROM golang:1.20 as build

ARG TARGETARCH=amd64

ENV GOPROXY=direct

# download go modules ahead to speed up the building
WORKDIR /workspace
COPY go.mod .
COPY go.sum .
RUN go mod download -x

# copy source
COPY build/validator/linux/${TARGETARCH}/validator /workspace/validator

ENTRYPOINT ["/workspace/validator"]