# The binary is built outside the image (CGO_ENABLED=0, static), so the image
# is only the binary on a distroless base: no shell, no package manager.
# The base carries CA certificates (needed for HTTPS to stores) and tzdata.
#
#   CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/cupproof-linux ./cmd/cupproof
#   docker build -t cupproof .
FROM gcr.io/distroless/static-debian12:nonroot
COPY bin/cupproof-linux /cupproof
USER nonroot
EXPOSE 8080
ENTRYPOINT ["/cupproof"]
CMD ["serve"]
