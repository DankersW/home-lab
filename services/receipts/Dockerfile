# syntax=docker/dockerfile:1

############################
# Build stage
############################
FROM golang:1.25-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

# Build both static binaries. modernc.org/sqlite is pure Go, so CGO is off and
# the binaries run on the distroless static base. Templates and assets are
# embedded via embed.FS, so the final image needs only the binaries.
COPY . .
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/receipts ./cmd/receipts && \
    go build -trimpath -ldflags="-s -w" -o /out/backup   ./cmd/backup

############################
# Final stage (static, non-root)
############################
FROM gcr.io/distroless/static:nonroot

COPY --from=build /out/receipts /usr/local/bin/receipts
COPY --from=build /out/backup   /usr/local/bin/backup

# SQLite db + multipart scratch live here; mounted from ./services/receipts/data.
VOLUME ["/data"]

EXPOSE 8080
CMD ["/usr/local/bin/receipts"]
