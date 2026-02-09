FROM ghcr.io/x402-rs/x402-facilitator:dev

COPY config.json /app/config.json
COPY entrypoint.sh /app/entrypoint.sh

EXPOSE 8402

ENTRYPOINT ["sh", "/app/entrypoint.sh"]
