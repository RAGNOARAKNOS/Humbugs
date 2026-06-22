# syntax=docker/dockerfile:1

# --- Build stage -----------------------------------------------------------
FROM golang:1.26 AS build

WORKDIR /src

# Cache module downloads separately from the source for faster rebuilds.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Pure-Go build (modernc.org/sqlite needs no C toolchain); produce a static,
# stripped binary so it runs in a minimal scratch/distroless image.
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/humbugs ./cmd/humbugs

# --- Runtime stage ---------------------------------------------------------
# distroless/static ships ca-certificates (needed for HTTPS scraping) and a
# nonroot user, with nothing else to attack.
FROM gcr.io/distroless/static:nonroot

WORKDIR /data
COPY --from=build /out/humbugs /humbugs

# coins.yaml (config) and humbugs.db (state) live here; mount a volume to persist.
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/humbugs"]
CMD ["serve", "--addr", ":8080"]
