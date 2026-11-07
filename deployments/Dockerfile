FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG CMD
RUN CGO_ENABLED=0 go build -trimpath -o /out/app ${CMD}

FROM alpine:3.20
COPY --from=build /out/app /app
USER nobody
ENTRYPOINT ["/app"]
