# Multi-stage build: a big toolchain image compiles the binary, then we copy ONLY the
# binary into a tiny runtime image. Result: a small, fast image with no Go toolchain.

# ---- build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# Copy go.mod/go.sum first and download deps — this layer is cached unless deps change,
# so rebuilds after a code change are fast.
COPY go.mod go.sum ./
RUN go mod download

# Now the source, and compile a STATIC binary (CGO off) so it runs on a minimal base.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /task-engine .

# ---- runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates   # for any TLS calls
COPY --from=build /task-engine /task-engine
EXPOSE 8080
ENTRYPOINT ["/task-engine"]
