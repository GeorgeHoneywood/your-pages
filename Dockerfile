FROM golang:latest AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
RUN CGO_ENABLED=0 go build -ldflags "-s -w"

FROM gcr.io/distroless/base:latest
WORKDIR /app
COPY --from=builder /app/your-pages .
EXPOSE 4444
CMD ["./app"]