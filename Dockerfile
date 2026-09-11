FROM golang:1.26.4-bookworm AS go-toolchain
FROM rust:1.89-bookworm AS build
COPY --from=go-toolchain /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}" CGO_ENABLED=1
WORKDIR /src
COPY . .
RUN bash build.sh

FROM build AS test
RUN cargo test --locked --manifest-path native/rust/Cargo.toml && go vet ./... && \
    CALCULATOR_NATIVE_DIR=/src/bin go test -race -count=1 -coverprofile=/src/coverage.out ./... && \
    CALCULATOR_NATIVE_DIR=/src/bin go test -tags=integration -race -count=1 ./... && \
    go test -run '^$' -fuzz=FuzzQuery -fuzztime=3s -parallel=4 ./internal/httpapi

FROM debian:bookworm-slim AS runtime
WORKDIR /app
COPY --from=build /src/bin/ ./
USER 65532:65532
EXPOSE 8080
STOPSIGNAL SIGTERM
ENTRYPOINT ["/app/calculator_server"]
