FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
ARG SERVICE
RUN go build -o /out/service ./cmd/${SERVICE}

FROM alpine:3.22
WORKDIR /app
COPY --from=build /out/service /app/service
EXPOSE 8080
ENTRYPOINT ["/app/service"]
