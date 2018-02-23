FROM scratch
ENTRYPOINT ["/frontend-bin"]
ADD ca-certificates.crt /etc/ssl/certs/
ADD frontend-bin /
