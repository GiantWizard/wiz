# STAGE 1: Go Builder for Calculation Engine
FROM golang:1.23-alpine AS go-builder
LABEL stage=go-builder
WORKDIR /app # Using a general /app first

# Copy go.mod and go.sum first to leverage Docker cache for dependencies
# Source: backend/calculation_engine/go.mod (relative to build context 'backend/')
# Destination: /app/go.mod (inside the container)
COPY calculation_engine/go.mod calculation_engine/go.sum ./
# After this, /app/go.mod and /app/go.sum exist in the container.

# Download dependencies based on the copied go.mod/go.sum
# This command runs in WORKDIR /app
RUN go mod download
RUN go mod verify

# Copy the rest of the Go application source code
# Source: backend/calculation_engine/ (all contents)
# Destination: /app/ (all contents copied into /app)
COPY calculation_engine/ ./
# Now, all .go files from calculation_engine are in /app along with go.mod & go.sum

# Build the Go application.
# This command runs in WORKDIR /app. Go will find go.mod in current dir.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o /go_app_binary .


# STAGE 2: Rust/C++ Builder for Metrics Generator and Export Engine
FROM rust:1.87.0-slim-bookworm AS rust-cpp-builder
LABEL stage=rust-cpp-builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    g++ make curl ca-certificates pkg-config libssl-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app/metrics_generator
# Source: backend/server9/Cargo.toml
# Destination: /app/metrics_generator/Cargo.toml
COPY server9/Cargo.toml server9/Cargo.lock* ./
RUN mkdir -p src && echo "fn main(){println!(\"Dummy main for cargo fetch\");}" > src/main.rs
RUN cargo fetch
# Source: backend/server9/src/
# Destination: /app/metrics_generator/src/
COPY server9/src ./src
RUN cargo build --release

WORKDIR /app/export_engine_build
# Source: backend/server9/export.cpp
# Destination: /app/export_engine_build/export.cpp
COPY server9/export.cpp .
RUN g++ -std=c++11 -O2 -Wall -o /app_export_engine export.cpp

RUN curl -fsSL https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb -o /tmp/megacmd.deb \
    && apt-get update \
    && apt-get install -y --no-install-recommends /tmp/megacmd.deb \
    && rm /tmp/megacmd.deb \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# STAGE 3: Final Runtime Image
FROM debian:bookworm-slim AS final

RUN apt-get update && apt-get install -y --no-install-recommends \
    supervisor ca-certificates libfuse2 libssl3 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd --gid 999 appgroup && useradd --uid 999 --gid appgroup --shell /bin/bash --create-home appuser

RUN mkdir -p /app /data/metrics_archive /var/log/supervisor \
    && chown -R appuser:appgroup /app /data \
    && chown -R root:root /var/log/supervisor

WORKDIR /app

# Copy the compiled Go binary from the builder stage
COPY --from=go-builder --chown=appuser:appgroup /go_app_binary /app/calculation_service
# Copy Go dependencies
# Source: backend/calculation_engine/dependencies/
# Destination: /app/dependencies/
COPY --chown=appuser:appgroup calculation_engine/dependencies /app/dependencies

COPY --from=rust-cpp-builder --chown=appuser:appgroup /app/metrics_generator/target/release/timestamp_generator /app/metrics_generator
COPY --from=rust-cpp-builder --chown=appuser:appgroup /app_export_engine /app/export_engine

COPY --from=rust-cpp-builder --chown=root:root /usr/bin/mega-* /usr/local/bin/
RUN chmod +x /usr/local/bin/mega-*

# Source: backend/supervisor.conf
# Destination: /etc/supervisor/supervisord.conf
COPY supervisor.conf /etc/supervisor/supervisord.conf

EXPOSE 8081

CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/supervisord.conf", "-n"]