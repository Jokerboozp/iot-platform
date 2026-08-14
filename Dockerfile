FROM node:22-alpine AS web-build
WORKDIR /src
COPY web/package.json web/package-lock.json ./web/
RUN npm ci --prefix web
COPY web ./web
COPY internal/httpapi ./internal/httpapi
RUN npm --prefix web run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /src/internal/httpapi/static ./internal/httpapi/static
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/iot-platform ./cmd/iot-platform

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/iot-platform /app/iot-platform
VOLUME ["/app/data"]
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/iot-platform"]
