# syntax=docker/dockerfile:1

# ---- build stage ----
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/showip .

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot AS showip-image
COPY --from=build /out/showip /showip
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/showip"]
