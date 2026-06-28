FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
COPY pkg ./pkg
COPY reaper/go.mod reaper/go.sum* ./reaper/
COPY reaper ./reaper

WORKDIR /src/reaper
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/transcodex-reaper .

FROM alpine:3.22

COPY --from=build /bin/transcodex-reaper /bin/transcodex-reaper

ENTRYPOINT ["/bin/transcodex-reaper"]
