FROM alpine:3.19
WORKDIR /
COPY bin/duros-controller .
USER 65534
ENTRYPOINT ["/duros-controller"]
