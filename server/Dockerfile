# zmp-server image: the Go room server alone, built from source inside Docker (no Go needed on the host).
#
#   docker build -t zmp-server:local server/        (from the repository root)
#   docker compose -f server/deploy/docker-compose.yml up -d
#
# The image is tiny (one static binary on an empty base), runs as a non-root user, and writes only to /data
# (the rooms' canonical saves) and, if ZMP_LOG_DIR names a mounted folder, to that folder (the players' logs).
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /zmp-server ./cmd/zmp-server

FROM scratch
COPY --from=build /zmp-server /zmp-server
USER 1000:1000
EXPOSE 47100/tcp
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD ["/zmp-server", "-healthcheck"]
ENTRYPOINT ["/zmp-server"]
# Add "-web", "/web" (and mount the browser build at /web) to serve the browser game on the same port.
CMD ["-listen", ":47100", "-data", "/data"]
