FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
COPY pkg ./pkg
COPY api/go.mod api/go.sum* ./api/
COPY api ./api

WORKDIR /src/api
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/transcodex-api .

FROM alpine:3.22

COPY --from=build /bin/transcodex-api /bin/transcodex-api

EXPOSE 8080
ENTRYPOINT ["/bin/transcodex-api"]
