FROM node:22-alpine AS web
WORKDIR /web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build
FROM golang:1.22-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api && CGO_ENABLED=0 go build -o /out/worker ./cmd/worker && CGO_ENABLED=0 go build -o /out/simulator ./cmd/simulator
FROM alpine:3.21
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 urbanops
WORKDIR /app
COPY --from=backend /out/ /app/
COPY --from=web /web/dist /app/web/dist
COPY docs /app/docs
USER urbanops
EXPOSE 8080
CMD ["/app/api"]
