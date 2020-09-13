FROM golang:1.15 AS build

RUN apt-get update && apt-get install -y upx-ucl
ENV CGO_ENABLED=0
WORKDIR /app
COPY . .
RUN go build -ldflags="-s -w" -o a2a ./client
RUN upx a2a

FROM gcr.io/distroless/static
COPY --from=build /app/a2a /usr/bin/actions2aws
ENTRYPOINT ["/usr/bin/actions2aws"]
