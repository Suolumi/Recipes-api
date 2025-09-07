# syntax=docker/dockerfile:1

FROM golang:1.23 as builder

# Set destination for COPY
WORKDIR /app

# Copy the source code. Note the slash at the end, as explained in
# https://docs.docker.com/reference/dockerfile/#copy
COPY . ./

RUN go mod download

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/api ./cmd/api

RUN chmod +x ./api

# Optional:
# To bind to a TCP port, runtime parameters must be supplied to the docker command.
# But we can document in the Dockerfile what ports
# the application is going to listen on by default.
# https://docs.docker.com/reference/dockerfile/#expose
EXPOSE 8080

FROM gcr.io/distroless/static-debian12

WORKDIR /app
COPY --from=builder /app/api /app/api
COPY --from=builder /app/assets /app/assets

ENTRYPOINT ["/app/api"]