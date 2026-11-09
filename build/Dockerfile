# The portcheck image, built by GoReleaser from the release binaries
# (dockers_v2 in .goreleaser.yaml). Distroless: no shell, runs as non-root.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/portcheck /usr/bin/portcheck
ENTRYPOINT ["/usr/bin/portcheck"]
CMD ["--help"]
