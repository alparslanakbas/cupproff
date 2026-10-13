# Two stages: the server builds the binary from the checkout (the templ and
# sqlc outputs are committed, so a plain go build is enough), and the image
# is only that static binary on a distroless base: no shell, no package
# manager. The base carries CA certificates (HTTPS to stores) and tzdata.
#
#   docker build -t cupproof .
FROM golang:1.27.2 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/cupproof ./cmd/cupproof

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/cupproof /cupproof
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/cupproof"]
CMD ["serve"]
