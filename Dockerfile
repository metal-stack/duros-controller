FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY bin/duros-controller .
ENTRYPOINT ["/duros-controller"]
