# STAGE 1: Go Builder for Calculation Engine
FROM golang:1.23-alpine AS go-builder
LABEL stage=go-builder
WORKDIR /app

COPY calculation_engine/go.mod calculation_engine/go.sum ./
RUN go mod download
RUN go mod verify

COPY calculation_engine/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o /go_app_binary .


# STAGE 2: Rust/C++ Builder for Metrics Generator and Export Engine
FROM rust:1.87.0-slim-bookworm AS rust-cpp-builder
LABEL stage=rust-cpp-builder

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        g++ \
        make \
        curl \
        ca-certificates \
        pkg-config \
        libssl-dev && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app/metrics_generator
COPY server9/Cargo.toml server9/Cargo.lock* ./
RUN mkdir -p src && echo "fn main(){println!(\"Dummy main for cargo fetch\");}" > src/main.rs
RUN cargo fetch
COPY server9/src ./src
RUN cargo build --release

WORKDIR /app/export_engine_build
COPY server9/export.cpp .
RUN g++ -std=c++11 -O2 -Wall -o /app_export_engine export.cpp
# No need to install megacmd in this builder stage if it's only for the final image

# STAGE 3: Final Runtime Image
FROM debian:bookworm-slim AS final

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        supervisor \
        ca-certificates \
        libfuse2 \
        libssl3 \
        dbus \
        curl && \ 
    # Install megacmd directly in the final stage
    echo "Installing MEGAcmd in final stage..." && \
    curl -fsSL https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb -o /tmp/megacmd.deb && \
    apt-get install -y --no-install-recommends /tmp/megacmd.deb && \
    rm /tmp/megacmd.deb && \
    # apt-get autoremove -y curl # Keep curl if other parts of your app might need it, or remove if only for this
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* && \
    echo "--- [FINAL STAGE] Which megals: ---" && \
    (which megals || echo "megals not found in PATH via 'which' in final stage") && \
    echo "--- [FINAL STAGE] ls -l /usr/bin/megals: ---" && \
    (ls -l /usr/bin/megals || echo "/usr/bin/megals not found in final stage") && \
    echo "--- [FINAL STAGE] Which mega-cmd-server: ---" && \
    (which mega-cmd-server || echo "mega-cmd-server not found in PATH via 'which' in final stage")


RUN groupadd --gid 999 appgroup && useradd --uid 999 --gid appgroup --shell /bin/bash --create-home appuser

RUN mkdir -p /app /data/metrics_archive /var/log/supervisor \
    && chown -R appuser:appgroup /app /data \
    && chown -R root:root /var/log/supervisor

WORKDIR /app

COPY --from=go-builder --chown=appuser:appgroup /go_app_binary /app/calculation_service
RUN chmod +x /app/calculation_service

COPY --chown=appuser:appgroup calculation_engine/dependencies /app/dependencies

COPY --from=rust-cpp-builder --chown=appuser:appgroup /app/metrics_generator/target/release/timestamp_generator /app/metrics_generator
RUN chmod +x /app/metrics_generator

COPY --from=rust-cpp-builder --chown=appuser:appgroup /app_export_engine /app/export_engine
RUN chmod +x /app/export_engine

# mega-* tools are now installed system-wide in this stage, so no COPY needed for them here.

COPY supervisor.conf /etc/supervisor/supervisord.conf

EXPOSE 9000

CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/supervisord.conf", "-n"]