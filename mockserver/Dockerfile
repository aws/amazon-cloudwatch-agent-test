FROM golang:1.19 AS build
WORKDIR $GOPATH/main
COPY . .
RUN go env -w GOPROXY=direct
RUN GO111MODULE=on go mod download
RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -o=/bin/main .

# Generate TLS certificates at build time instead of committing them to the repo
RUN mkdir -p certificates/ssl && \
    openssl req -config openssl.conf -new -x509 -nodes -days 365 \
      -out certificates/ssl/certificate.crt \
      -keyout certificates/private.key && \
    printf 'mockserver\n' > certificates/ssl/ca-bundle.crt && \
    cat certificates/ssl/certificate.crt >> certificates/ssl/ca-bundle.crt

EXPOSE 80 
EXPOSE 443
FROM scratch
ENV AWS_REGION="us-west-2"
COPY --from=build /bin/main /bin/main
COPY --from=build /go/main/certificates certificates
ENTRYPOINT ["/bin/main"]
