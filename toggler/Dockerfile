FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go index.html ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /toggler .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /toggler /toggler
EXPOSE 8080
ENTRYPOINT ["/toggler"]
