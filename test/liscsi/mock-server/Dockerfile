FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY main.go .
RUN go mod init mock-lis-csi && go build -o mock-server main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/mock-server .
EXPOSE 8101
CMD ["./mock-server"]
