FROM golang:1.19 AS build
WORKDIR $GOPATH/main
COPY . .
RUN go env -w GOPROXY=direct
RUN GO111MODULE=on go mod download
RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -o=/bin/main .
EXPOSE 80 
EXPOSE 443
FROM scratch
ENV AWS_REGION="us-west-2"
COPY --from=build /bin/main /bin/main
COPY certificates certificates
ENTRYPOINT ["/bin/main"]