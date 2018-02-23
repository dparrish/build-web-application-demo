FROM scratch
ENTRYPOINT ["/frontend"]
ADD ca-certificates.crt /etc/ssl/certs/
ADD frontend /
