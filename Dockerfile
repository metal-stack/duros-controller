FROM alpine:3.20
WORKDIR /
COPY bin/duros-controller .
USER 65534
ENTRYPOINT ["/duros-controller"]
