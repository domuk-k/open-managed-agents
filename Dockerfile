FROM golang:1.22-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /oma ./cmd/oma

FROM alpine:3.19
RUN apk add --no-cache docker-cli ca-certificates
COPY --from=builder /oma /usr/local/bin/oma
EXPOSE 8080
ENTRYPOINT ["oma"]
CMD ["server", "start"]
