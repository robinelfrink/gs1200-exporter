FROM --platform=$BUILDPLATFORM golang:1.22.4 AS BUILD
WORKDIR /app
COPY . .
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=development
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-w -s -X 'main.Version=${VERSION}'" .

FROM scratch
WORKDIR /app
COPY --from=BUILD /app/gs1200-exporter /app

ENV GS1200_ADDRESS 192.168.1.3
ENV GS1200_PASSWORD 1234
ENV GS1200_PORT 9934

EXPOSE $GS1200_PORT
CMD [ "/app/gs1200-exporter" ]
