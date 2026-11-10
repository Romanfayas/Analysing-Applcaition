FROM alpine:3.19
RUN apk add --no-cache openssh-client netcat-openbsd
COPY tunnel.sh /tunnel.sh
RUN chmod +x /tunnel.sh
CMD ["/tunnel.sh"]
