FROM golang:1.23-alpine AS build

WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/docs-svc ./cmd/docs-svc

FROM alpine:3.20
RUN apk add --no-cache ca-certificates git openssh-client \
    && adduser -D -u 10001 appuser
WORKDIR /app
COPY --from=build /out/docs-svc /app/docs-svc
USER appuser
EXPOSE 8084
ENTRYPOINT ["/app/docs-svc"]
