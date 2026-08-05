FROM nginxinc/nginx-unprivileged:1.31.3-alpine@sha256:a6c3ec0c0d249d68b0682df854d4a9e222b90fb607dc3fcf2f1d2fcbc85d347e

COPY reverse-proxy/nginx.conf /etc/nginx/nginx.conf
COPY reverse-proxy/conf.d/dev.conf /etc/nginx/conf.d/default.conf

USER 101:101
EXPOSE 8080 8443
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=5 CMD ["wget", "-q", "-O", "/dev/null", "http://127.0.0.1:8080/health/live"]
