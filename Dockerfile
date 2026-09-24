FROM golang:1.25.5-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /hub ./cmd/hub

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /hub /hub
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/hub"]
