FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY main.go .

RUN CGO_ENABLED=0 GOOS=linux go build -o /mock-agent main.go

# Runtime image
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

COPY --from=builder /mock-agent /mock-agent

EXPOSE 9000

CMD ["/mock-agent"]
