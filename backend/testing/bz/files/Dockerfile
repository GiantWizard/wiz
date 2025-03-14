FROM golang:1.20 as builder
WORKDIR /app
COPY . .
RUN go build -o bazaar-tracker main.go

FROM ubuntu:jammy
WORKDIR /app

# Install dependencies
RUN apt-get update && apt-get install -y \
    curl \
    unzip \
    ca-certificates \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Copy binary and scripts
COPY --from=builder /app/bazaar-tracker .
COPY start.sh .
RUN chmod +x start.sh

# Run start script
CMD ["./start.sh"]