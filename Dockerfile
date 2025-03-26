FROM scratch
COPY git-sync /
ENTRYPOINT ["/git-sync"]
