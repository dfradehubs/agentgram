FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY main.go .

RUN CGO_ENABLED=0 GOOS=linux go build -o /mock-oauth2-mcp main.go

FROM alpine:3.19

RUN apk --no-cache add ca-certificates

COPY --from=builder /mock-oauth2-mcp /mock-oauth2-mcp

EXPOSE 9999

CMD ["/mock-oauth2-mcp"]
