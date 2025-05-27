FROM golang:1.23-alpine AS go-builder
LABEL stage=go-builder
WORKDIR /app/calculation_engine

COPY calculation_engine/go.mod calculation_engine/go.sum ./
RUN go mod download && go mod verify
COPY calculation_engine/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o /app/calculation_service .

FROM rust:1.87-slim-bookworm AS rust-cpp-builder
LABEL stage=rust-cpp-builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    g++ make curl ca-certificates pkg-config libssl-dev \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /build_output/cpp

WORKDIR /app/metrics_generator

COPY server9/Cargo.toml server9/Cargo.lock* ./
RUN mkdir -p src && echo "fn main(){println!(\"Dummy main for cargo fetch\");}" > src/main.rs
RUN cargo fetch
COPY server9/src ./src

RUN cargo build --release

WORKDIR /app
COPY server9/export.cpp .
RUN g++ -std=c++11 -O2 -Wall -o /build_output/cpp/export_engine export.cpp

RUN curl -fsSL https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb -o /tmp/megacmd.deb \
    && dpkg -i /tmp/megacmd.deb || (apt-get update && apt-get install --fix-broken -y --no-install-recommends) \
    && rm /tmp/megacmd.deb \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

FROM debian:bookworm-slim AS final

RUN apt-get update && apt-get install -y --no-install-recommends \
    supervisor ca-certificates libfuse2 libssl3 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd --system appgroup && useradd --system --gid appgroup --no-log-init appuser
RUN mkdir -p /app /data/metrics_archive /var/log/supervisor \
    && chown -R appuser:appgroup /app /data /var/log/supervisor

WORKDIR /app

COPY --from=go-builder --chown=appuser:appgroup /app/calculation_service /app/calculation_service
COPY --chown=appuser:appgroup calculation_engine/dependencies /app/dependencies

COPY --from=rust-cpp-builder --chown=appuser:appgroup /app/metrics_generator/target/release/timestamp_generator /app/metrics_generator
COPY --from=rust-cpp-builder --chown=appuser:appgroup /build_output/cpp/export_engine /app/export_engine

COPY --from=rust-cpp-builder --chown=appuser:appgroup /usr/bin/mega-* /usr/local/bin/

COPY supervisor.conf /etc/supervisor/supervisord.conf

EXPOSE 8081
USER appuser
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/supervisord.conf", "-n"]