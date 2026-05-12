FROM golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/hello-system .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates curl
COPY --from=build /out/hello-system /usr/local/bin/hello-system
USER nobody
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/hello-system"]
