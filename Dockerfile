FROM golang:1.25.0 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/invoicers ./cmd/invoicers

FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app
COPY --from=build /out/invoicers /app/invoicers

ENV INVOICERS_ADDR=:8080
ENV INVOICERS_DATA_DIR=/data
ENV INVOICERS_DB_PATH=/data/invoicers.db

EXPOSE 8080
VOLUME ["/data"]

ENTRYPOINT ["/app/invoicers", "serve"]
