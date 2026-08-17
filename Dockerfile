# ── Build stage ──────────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.26.6-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w" -o /shipit ./cmd/shipit

# ── Runtime stage (distroless) ────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian13:nonroot

COPY --from=builder /shipit /shipit

USER nonroot:nonroot

ENTRYPOINT ["/shipit"]
