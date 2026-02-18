FROM gcr.io/distroless/static-debian13:nonroot
WORKDIR /
COPY bin/duros-controller .
ENTRYPOINT ["/duros-controller"]
