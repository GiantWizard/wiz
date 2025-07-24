# STAGE 1: Go Builder for Calculation Engine
FROM golang:1.23-alpine AS go-builder
LABEL stage=go-builder
WORKDIR /app

# Download and verify modules
COPY calculation_engine/go.mod calculation_engine/go.sum ./
RUN go mod download && go mod verify

# Copy the whole calculation_engine dir (including dependencies/) and build
COPY calculation_engine/ ./
RUN CGO_ENABLED=0 GOOS=linux \
    go build -ldflags="-s -w" -trimpath -o /go_app_binary .

# STAGE 2: Rust/C++ Builder for Metrics Generator and Export Engine
FROM rust:1.87.0-slim-bookworm AS rust-cpp-builder
LABEL stage=rust-cpp-builder

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      g++ make curl ca-certificates pkg-config libssl-dev && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app/metrics_generator
COPY server9/Cargo.toml server9/Cargo.lock* ./
RUN mkdir -p src && echo "fn main(){}" > src/main.rs
RUN cargo fetch

COPY server9/src ./src
RUN cargo build --release

WORKDIR /app/export_engine_build
COPY server9/export.cpp .
RUN g++ -std=c++11 -O2 -Wall -o /app_export_engine export.cpp

# STAGE 3: Final Runtime Image
FROM debian:bookworm-slim AS final

# Runtime deps + cap tool + MEGAcmd
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
      supervisor \
      ca-certificates \
      libfuse2 \
      libssl3 \
      dbus \
      curl \
      libcap2-bin && \
    curl -fsSL https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb \
      -o /tmp/megacmd.deb && \
    apt-get install -y /tmp/megacmd.deb && \
    rm -f /tmp/megacmd.deb && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/megals && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/megaget && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/megalogin && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/megaput && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-ipc && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-whoami && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-logout && \
    ln -sf /usr/bin/mega-cmd /usr/local/bin/mega-mkdir && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Allow mega‑cmd‑server to bind privileged ports as non-root
RUN setcap 'cap_net_bind_service=+ep' /usr/bin/mega-cmd-server

# Create appuser/appgroup
RUN groupadd --gid 999 appgroup && \
    useradd --uid 999 --gid appgroup --shell /bin/bash --create-home appuser

# Prepare directories
RUN mkdir -p /app /data/metrics_archive /var/log/supervisor /var/run && \
    chown -R appuser:appgroup /app /data /home/appuser /var/run && \
    chown -R appuser:appgroup /var/log/supervisor

WORKDIR /app

# Copy the Go binary and its dependencies
COPY --from=go-builder --chown=appuser:appgroup /go_app_binary /app/calculation_service
RUN chmod +x /app/calculation_service

# **Fixed:** pull the dependencies tree from /app/dependencies in the go-builder
COPY --from=go-builder --chown=appuser:appgroup /app/dependencies /app/dependencies

# Copy Rust & C++ binaries
COPY --from=rust-cpp-builder --chown=appuser:appgroup /app/metrics_generator/target/release/timestamp_generator \
     /app/metrics_generator
RUN chmod +x /app/metrics_generator

COPY --from=rust-cpp-builder --chown=appuser:appgroup /app_export_engine /app/export_engine
RUN chmod +x /app/export_engine

# Copy session-keeper script and supervisor config
COPY session-keeper.sh /usr/local/bin/session-keeper.sh
COPY supervisor.conf /etc/supervisor/supervisor.conf
RUN chmod +x /usr/local/bin/session-keeper.sh

EXPOSE 9000

CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/supervisor.conf", "-n"]
