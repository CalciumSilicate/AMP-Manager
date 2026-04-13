FROM golang:1.24-alpine AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/perf-mock-upstream ./cmd/perf-mock-upstream
COPY internal/perf ./internal/perf

RUN CGO_ENABLED=0 go build -o /out/perf-mock-upstream ./cmd/perf-mock-upstream

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates

COPY --from=build /out/perf-mock-upstream ./perf-mock-upstream

EXPOSE 18080

CMD ["./perf-mock-upstream"]
