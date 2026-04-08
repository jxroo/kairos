# ---- Stage 1: Build Rust vector engine ----
FROM rust:1.86-bookworm AS rust-builder
WORKDIR /src/vecstore
COPY vecstore/ .
RUN cargo build --release

# ---- Stage 2: Build dashboard ----
FROM node:20-bookworm-slim AS dashboard-builder
WORKDIR /src/dashboard
COPY dashboard/package.json dashboard/package-lock.json ./
RUN npm ci
COPY dashboard/ .
RUN npm run build

# ---- Stage 3: Build Go binary ----
FROM golang:1.24-bookworm AS go-builder
RUN apt-get update && apt-get install -y --no-install-recommends \
    libsqlite3-dev gcc && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=rust-builder /src/vecstore/target/release/libvecstore.so vecstore/target/release/
COPY --from=dashboard-builder /src/dashboard/dist/ dashboard/dist/
ARG VERSION=dev
ENV CGO_LDFLAGS_ALLOW='-Wl,-rpath,.*'
RUN CGO_ENABLED=1 \
    CGO_LDFLAGS="-L/src/vecstore/target/release" \
    go build -ldflags "-X github.com/jxroo/kairos/internal/version.Version=${VERSION}" \
    -o /kairos ./cmd/kairos/

# ---- Stage 4: Minimal runtime ----
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates libsqlite3-0 && rm -rf /var/lib/apt/lists/*
RUN useradd -m -s /bin/bash kairos
COPY --from=go-builder /kairos /usr/local/bin/kairos
COPY --from=rust-builder /src/vecstore/target/release/libvecstore.so /usr/local/lib/
RUN ldconfig
USER kairos
WORKDIR /home/kairos
EXPOSE 7777
ENTRYPOINT ["kairos"]
CMD ["start"]
