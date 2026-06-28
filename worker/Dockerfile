FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
COPY pkg ./pkg
COPY worker/go.mod worker/go.sum* ./worker/
COPY worker ./worker

WORKDIR /src/worker
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/transcodex-worker .

FROM alpine:3.22

RUN apk add --no-cache ca-certificates ffmpeg

COPY --from=build /bin/transcodex-worker /bin/transcodex-worker

ENTRYPOINT ["/bin/transcodex-worker"]
