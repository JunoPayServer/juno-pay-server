FROM --platform=linux/amd64 rust:1.86-bookworm AS rust-build

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    pkg-config \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY rust/keys/Cargo.toml rust/keys/Cargo.toml
COPY rust/keys/src rust/keys/src
COPY rust/keys/include rust/keys/include
RUN cargo build --release --manifest-path rust/keys/Cargo.toml

FROM --platform=linux/amd64 node:20-bookworm AS admin-build

WORKDIR /src/admin-dashboard
COPY admin-dashboard/package.json admin-dashboard/package-lock.json ./
RUN npm ci
COPY admin-dashboard/ ./
RUN npm run build

FROM --platform=linux/amd64 golang:1.24-bookworm AS go-build

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    build-essential \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

COPY --from=rust-build /src/rust/keys/target/release /src/rust/keys/target/release
COPY --from=rust-build /src/rust/keys/include /src/rust/keys/include

COPY --from=admin-build /src/admin-dashboard/out /src/internal/api/adminui_dist

RUN go build -trimpath -tags=adminui -o /out/juno-pay-server ./cmd/juno-pay-server

FROM --platform=linux/amd64 debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
  && rm -rf /var/lib/apt/lists/*

RUN useradd -r -u 10001 -g nogroup juno

COPY --from=go-build /out/juno-pay-server /usr/local/bin/juno-pay-server
COPY --from=rust-build /src/rust/keys/target/release/libjuno_keys.so /usr/local/lib/libjuno_keys.so

RUN mkdir -p /data \
  && chown -R 10001:nogroup /data

ENV LD_LIBRARY_PATH=/usr/local/lib
ENV JUNO_PAY_ADDR=0.0.0.0:8080

EXPOSE 8080

USER 10001
ENTRYPOINT ["juno-pay-server"]
